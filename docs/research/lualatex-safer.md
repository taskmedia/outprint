# `lualatex --safer` in the render Worker: usable or not?

- Date: 2026-10-31
- Scope: whether `lualatex --safer` can harden the Pandoc -> LuaLaTeX (KOMA-Script, fontspec) step, and what it adds on top of `shell_escape` / `openout_any`. Follow-up to `markdown-latex-fidelity.md` section 3.1 / 6, which recorded that `--safer` broke font loading.
- Environment: LuaHBTeX 1.24.0 / TeX Live 2026 (basic), macOS, `/Library/TeX/texbin`. TeX Live defaults: `shell_escape = p`, `openout_any = p`, `openin_any = a` (`texmf.cnf`).
- Legend: **[V]** verified by a local experiment, **[S]** read in primary source (cited), **[I]** inferred.
- Experiments were run in the session scratchpad (not committed). Note: `LANG=en-US.UTF-8` (hyphen) made the `lualatex` wrapper abort with `Unable to read locale data`; the experiments used `LC_ALL=en_US.UTF-8`. The Worker image should set a valid locale.

## 1. Verdict in one paragraph

**Reject `--safer` for the fontspec/luaotfload pipeline.** It cannot work: `luaotfload` refuses to start under `--safer` by design, so no OpenType font can be loaded, neither by name nor by file path. The hardening that matters is already provided by `shell_escape=f` (disables the process-spawning and native-library routes, and drops `io.popen`/`os.execute`) and `openout_any=p` (which also gates Lua `io.open` for writing via kpathsea). `--safer` would add a stricter write block and removal of `debug`, but it leaves several read/FS functions open, so it is not a closed sandbox either. The network-less container stays mandatory.

## 2. Why it broke (question 1)

`luaotfload.lua` aborts itself when the engine reports `--safer` [S, V]:

```lua
if status.safer_option ~= 0 then
 texio.write_nl("term and log","luaotfload can't run with option --safer. Aborting")
 error("safer_option used")
end
```

Source: `.../texmf-dist/tex/luatex/luaotfload/luaotfload.lua` lines 103-106 (installed TL 2026).

Reproduction [V] (fresh cache dir, `TEXMFVAR=$PWD/var1`; same result with a fully pre-built names database):

| Input | `lualatex` | `lualatex --safer` |
|---|---|---|
| `\setmainfont{Latin Modern Roman}` (by name) | PDF written | `luaotfload can't run with option --safer. Aborting` then `! Font \TU/lmr/m/n/10.95=[lmroman10-regular]:+tlig; ... not loadable: metric data not found or bad.` and `! Package fontspec Error: The font "Latin Modern Roman" cannot be found` |
| `\setmainfont{lmroman10-regular.otf}[Path=./,...]` (by file path) | PDF written | same abort, `The font "lmroman10-regular" cannot be found` |
| distro font by name (TeX Gyre; Latin Modern as fontspec default) | PDF written (LM) | same abort |
| no font package at all (`scrartcl` only) | PDF written | `! error: (file rm-lmr10) (type 3): font rm-lmr10 at 657 not found`, `Fatal error occurred, no output PDF file produced!` |

So it is neither a lookup-by-name problem nor a cache problem: the whole font loader is off, and the LaTeX format (which loads luaotfload) is broken with it. TFM fonts do still work in plain TeX under `--safer` (`\font\x=ec-lmr10` gave a DVI [V]), but `file:` OpenType loading does not work even without `--safer` unless luaotfload is active [V: `Font \x=file:lmroman10-regular.otf not loadable`].

Related finding: luaotfload also needs a **writable cache** even in normal mode. With `TEXMFVAR`/`TEXMFCACHE` on a read-only directory (pre-built or empty) it fails with `system : no writeable cache path, quiting` (`luaotfload-init.lua:301`) [V].

## 3. What `--safer` actually restricts (question 3)

Manual [S: LuaTeX manual source `texk/web2c/doc/luatex/luatex-lua.tex`, "Initialization"]: `--safer` nils `os.execute exec kpsepopen spawn setenv rename remove tmpdir`, `io.popen output tmpfile`, `lfs.rmdir mkdir mkdirp chdir lock touch`; disables loading of compiled Lua libraries; makes `io.open` fail for anything but reading; disables the `socket` library unconditionally (not `mime`); since 1.18 the `debug` library is off unless `--luadebug`.

Implementation [S]: `texk/web2c/luatexdir/lua/luatex-core.lua` (TeX-Live/texlive-source). It replaces the functions with a dummy that prints once `safer option set, function "io.open" is disabled`, sets `package.loadlib` to a no-op, removes `package.searchers[3,4]` (C loaders), removes `ffi`, sets `debug = nil`. That last block runs for `saferoption == 1 or shellescape ~= 1`, so loadlib/ffi/C loaders are **also removed in the default restricted mode** [S].

Empirical matrix (`\directlua`, same file, each cell what the call returned) [V]:

| Call | default (`shell_escape=p`) | `--safer` |
|---|---|---|
| `os.execute`, `io.popen`, `os.exec`, `os.spawn` | `nil, "Command execution disabled via shell_escape='p'"` / `"specific command execution disabled"` | dummy, returns nothing |
| `io.open(file,"r")` | works (any path, incl. `/etc/hosts`, `../x`) | **returns nil (even for reading)**: the dummy replaces the whole function, contrary to the manual's "reading still works" |
| `io.open(f,"w")` | `./f` ok; `/tmp/x` and `../x` blocked by `openout_any=p` | nil |
| `io.lines(path)`, `io.input(path)`, `loadfile`, `dofile`, `require` of local `.lua` | work for any path | **still work** (read `../outside.txt`, `/etc/hosts`, execute `../outside.lua`) |
| `lfs.attributes/dir/symlinkattributes` | work | work |
| `lfs.link` (symlink) | returned `0`, symlink created | returned `0`, **symlink created** (not in the manual's nil list) |
| `lfs.mkdir/rmdir/touch/chdir`, `os.rename/remove` | work in cwd / blocked outside (kpse checks; `chdir` -> `LuaTeX: operation not permitted`) | dummies, nothing happens |
| `os.getenv`, `os.tmpname`, `kpse.find_file("/etc/hosts","tex")` | work | work |
| `package.loadlib`, `require "ffi"` | disabled (returns nothing / `false`) | disabled |
| `debug` | table (traceback only per 1.18 text; full table observed in the `luahbtex --ini` run) | `nil` |
| `\openin`, `\input ../outside.txt`, `\openout` | `\input` and `\openin /etc/hosts` work; `\openout ../up.txt` -> `Not writing to ../up.txt (openout_any = p; no extended check)` | **identical**: `--safer` does not touch TeX-level I/O |
| `\write18` | `status.shell_escape = 2` (restricted), blocked | same |

## 4. File access controls (question 4)

- `\directlua` can read files under `--safer` via `io.lines`, `io.input`, `loadfile`, `dofile`, `lfs.*`, `kpse.find_file`, and via TeX `\input`/`\openin` [V, table above]. Only `io.open` is dead.
- **`openin_any` no longer exists as a control.** `texmf.cnf` (TL 2026): "as of 2026, openin_any no longer has any effect; the underlying kpse_in_name_ok and related functions always return true" [S, V: `\input ../outside.txt` and `\openin /etc/hosts` worked with `openin_any=p` in the environment, consistent with the leak recorded in `markdown-latex-fidelity.md` 3.1]. Reads must therefore be contained by the container filesystem (mount only the job dir and the Template package), not by TeX.
- `openout_any=p` is the real write control: blocks dotfiles, `..`, absolute paths outside `$TEXMFOUTPUT`, for `\openout` and (since LuaTeX 1.18 with kpathsea) for Lua `io.open(...,"w")` and the `lfs` mutators [S: manual text above; V: `/tmp/x` and `../up.txt` blocked without `--safer`]. A job writing inside its own cwd is still allowed (`./w1.txt` ok) [V].

## 5. Network-capable Lua libraries (question 5)

- This build contains a LuaSocket (`require "socket"` returns a table) [V]. It is **off by default and in restricted mode**; it is switched on by `--socket` or by plain `--shell-escape` [S: manual text "By default the socket library is not enabled: one can enable it with --socket or with --shell-escape (but without --shell-restricted) and disable it with --nosocket or unconditionally with --safer"; V].

| Flags | `require "socket"` [V] |
|---|---|
| none (restricted default) | `module 'socket' not found` |
| `--socket` | table |
| `--shell-escape` | table |
| `--safer`, `--safer --socket` | `module 'socket' not found` |

- Other network libs (`socket.http`, `curl`, `lcurl`, `ssl`, `posix`, `lanes`, `ffi`) are not found [V]. `mime`/`ltn12` load (encoding helpers only; `mime.core` is built in) [V]. Built-ins seen in `package.loaded` under `lualatex`: `lpeg lfs md5 sha2 zlib gzip zip pdfe pdf img mplib luaharfbuzz fontloader kpse lualibs` [V]. `os.socketgettime`/`socketsleep` exist but are timers.
- Consequence: **never pass `--shell-escape` or `--socket`**; keep the default `p` or set `shell_escape=f`.

## 6. Fonts under `--safer` and the setup the Worker actually needs (question 2)

Under `--safer` there is no workable font setup with fontspec/luaotfload (section 2). The only theoretical path is a custom format with TFM/Type1 fonts and no luaotfload (no `fontspec`, no OpenType Templates). That is not "LuaLaTeX with KOMA and shipped Template fonts" [I], so it is out of scope.

Setup that works **without** `--safer` and is deterministic [V, `shell_escape=f lualatex`]:

1. Template fonts shipped in the package, loaded by file path: `\setmainfont{Tpl}[Path=./fonts/,Extension=.otf,UprightFont=*-regular,BoldFont=*-bold,ItalicFont=*-italic,BoldItalicFont=*-bolditalic]`. PDF written, LM-based OTF embedded as `CID Type 0C`.
2. Distro fonts by name resolve through the names database (`\setmainfont{Latin Modern Roman}` ok). Fonts outside the TeX trees need `OSFONTDIR`; unset here.
3. luaotfload cache: build at image time (one unrestricted run populates `luatex-cache/generic/names` and `fonts/otl`), point `TEXMFVAR`+`TEXMFCACHE` at an **absolute, writable** path (tmpfs or per-job copy). A copy of the pre-built cache directory worked and did not rebuild the names db (mtime unchanged) [V]. A read-only cache dir does **not** work (`no writeable cache path`) [V]. Avoid `..` in `TEXMFVAR`: with `openout_any=p` the first build failed on a path containing `..` [V].
4. Root filesystem read-only, job dir and cache on tmpfs, no network, mount only the Template package and job dir (reads are not restricted by TeX).

## 7. What `--safer` would add over `shell_escape=f` + `openout_any=p` (question 6)

| Concern | Without `--safer` (`shell_escape=f`, `openout_any=p`) | `--safer` adds |
|---|---|---|
| Spawn processes | blocked [V] | nothing |
| Native libs / ffi / socket | blocked in restricted/f modes [S, V] | nothing |
| Writing outside cwd | blocked (kpse checks, `lfs` too) [V] | write inside cwd also blocked; but luaotfload dies |
| Reading arbitrary files | open | still open (`io.lines`, `dofile`, `\input`) [V] |
| `debug` library | traceback only (1.18+) | removed |
| `lfs.link` | open | open [V] (gap) |

Net: `--safer` adds little security for this pipeline and costs the font loader.

## Recommendation

- **Reject `--safer`.** Run `lualatex` with `shell_escape=f` (set via environment or a Worker-owned `texmf.cnf`, not the user-controlled document), `openout_any=p`, never `--shell-escape`/`--socket`.
- Fonts: Template-shipped fonts by `Path=` or distro fonts by name; pre-build the luaotfload cache at image build time into a writable tmpfs/per-job dir; valid `LC_ALL`.
- Treat TeX-level reads as unrestricted (`openin_any` is a no-op in TL 2026): confine with the container (read-only root, only job dir + Template mounted, no secrets, no network), plus the Pandoc-side measures in `markdown-latex-fidelity.md` 3.1 (`-raw_tex-raw_attribute`, neutralise `Math`).
- All of this is defence in depth; the network-less container sandbox is mandatory regardless.
- Re-evaluate only if luaotfload gains a `--safer` mode or the Template switches to a TFM-only fontless setup.

## Sources

- LuaTeX manual source, "Initialization" section: https://github.com/TeX-Live/texlive-source/blob/trunk/texk/web2c/doc/luatex/luatex-lua.tex (`--safer`, `--socket`, `--nosocket`, kpse-checked `io`/`lfs`).
- LuaTeX security overloads: https://github.com/TeX-Live/texlive-source/blob/trunk/texk/web2c/luatexdir/lua/luatex-core.lua (`saferoption`, `installdummy`, loadlib/ffi removal, kpse wrappers).
- luaotfload: `texmf-dist/tex/luatex/luaotfload/luaotfload.lua` lines 103-106 and `luaotfload-init.lua:301` (installed TL 2026).
- `texmf.cnf` (TL 2026): `shell_escape`, `openout_any`, and the `openin_any` no-op note (thread cited there: https://tug.org/pipermail/tex-live/2025-December/051965.html).

## Outcome

- `--safer` was rejected, as recommended; the Worker never passes it (see [../worker.md](../worker.md#engine-lockdown)).
- The section 4 finding that `openin_any` is "no longer a control" was **not** accepted as final. The Worker runs LuaLaTeX with `openin_any=p` (paranoid reads) and makes that work by linking the TeX trees into the job directory, so that reads are confined to the job and the TeX installation. See [../worker.md](../worker.md#reading-below-the-job-directory-only) for the mechanism and [../security.md](../security.md) for the tests that prove it. Note that the setting is only effective together with that layout; the container sandbox stays mandatory.
- Fonts are loaded by file path (`Path=./fonts/`) because the luaotfload name database cannot be built under paranoid reads, which confirms section 6. The cache is built at image build time (`renderer warm-cache`).
