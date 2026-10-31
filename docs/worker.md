# Worker pipeline

The Worker turns one job into one PDF. The same pipeline runs behind the
`renderer render` command and behind the Worker's HTTP endpoint
(`renderer worker`, see [architecture.md](architecture.md)). This page
describes what it does, how it is locked down, what it logs and where it is
limited. The reasons for the two-role split are in
[ADR 0001](adr/0001-split-renderer-into-gateway-and-worker.md).

## Phases

A job runs through these phases. A failure names the phase and a kind, plus
the job id; see [Errors](#errors).

| Phase | What happens |
|-------|--------------|
| `setup` | Create the job directory, find the TeX installation, write the engine configuration, copy the Template's shared files and fonts, write the bundled images and copy the pre-built font cache. |
| `svg` | Convert the Configuration's SVG logo to PDF with `rsvg-convert`. |
| `pandoc` | Read Markdown with the hardened reader, run the Lua filters, write LaTeX using the Document Type's template. |
| `filter` | Not a separate step: the failure phase when one of the filters rejects the document. |
| `tex` | Run LuaLaTeX, at most twice. |
| `timeout` | Not a separate step: the failure phase when the wall-clock limit ended the job. |

Document values reach TeX in one way only: as Pandoc metadata in a YAML file
the Worker writes (`--metadata-file`). They are never passed as `-V`, and
never concatenated into a command line or into TeX.

### Reader

```
pandoc --sandbox -f markdown-raw_tex-raw_attribute-tex_math_dollars-yaml_metadata_block \
  -t latex --template <Document Type> --metadata-file meta.yaml \
  --lua-filter sanitize.lua --lua-filter layout.lua
```

- `-raw_tex` and `-raw_attribute`: TeX in the Markdown, including
  ` ```{=latex} ` blocks, is ordinary text.
- `-tex_math_dollars`: a `$` is a dollar sign. Prices and `$x$` print as
  written. There is no math in v1.
- `-yaml_metadata_block`: the Metadata Block is handled before Pandoc, so a
  YAML block in the body is text.
- Raw HTML stays on because Outline writes `<br>` for line breaks in tables.
- `--sandbox` limits what the readers and writers may do with files. It does
  not limit the Lua filters or LuaLaTeX, which is why the rest of this page
  exists.

### Lua filters

The filters are embedded in the binary (`internal/worker/filters`) and written
into the job directory. They run in this order.

`sanitize.lua`:

- `<br>`, `<br/>` and `<br />` become line breaks.
- Any other raw HTML is dropped: block-level raw HTML (`<script>`, comments,
  `<iframe>`) disappears completely; inline tags such as `<b>` lose their tag
  and keep their text. `<div>` is not raw HTML for Pandoc (it becomes a
  division), so its text stays.
- Any raw node of format `latex` or `tex`, in the body or in metadata, fails
  the job with kind `raw_tex`. This cannot happen with the reader above; it
  is an assertion, an internal error, and the raw node is never silently
  removed. Any other raw format fails with `raw_format`.
- Any math node fails the job with kind `math`.
- An image must refer to `images/<name>`, a file the job bundle contains. A
  parent path, an absolute path, a URL with a scheme (`file:`, `https:`), or a
  missing file fails the job with kind `image_path`.
- A link keeps its text, and stays a link only for `http`, `https`, `mailto`
  and `#anchor` targets. Everything else (`javascript:`, `file:`, `data:`,
  site-relative paths) becomes plain text.

`layout.lua`:

- Tables get column widths. A table whose content fits one line keeps its
  natural width. Any other table is set with relative widths from the content,
  damped so that one long column does not starve the others and never
  narrower than the longest unbreakable word. The writer produces a
  `longtable`, so a long table breaks across pages and its header row repeats
  on every page.
- Headings deeper than level 5 become bold paragraphs.
- A paragraph that holds only line breaks is removed.

The Template decides how headings look. The `neutral` report uses KOMA-Script
sections; its letter has no sectioning commands, so it defines a heading as a
bold line.

A filter failure is raised as `renderer-filter:<kind>: <fixed text>`. The
Worker reads only the kind; the text never contains Document content.

## Engine lockdown

LuaLaTeX runs with a `texmf.cnf` the Worker writes into the job directory and
puts first in `TEXMFCNF`:

```
shell_escape = f      no shell, no process spawning, no native libraries
openin_any   = p      paranoid reads
openout_any  = p      paranoid writes
TEXMFOUTPUT  = <job directory>
```

It never gets `--shell-escape`, `--socket` or `--safer`. The command line has
`-no-shell-escape -halt-on-error -interaction=nonstopmode`. kpathsea is also
told not to run its `mktex*` helper scripts.

### Reading below the job directory only

Paranoid mode refuses absolute paths unless they lie below `TEXMFOUTPUT`.
That also covers the TeX installation itself: with `openin_any=p` and the
distribution's own paths, Lua code in LuaTeX (luaotfload reads Unicode data
files with `io.open`) fails to start. The fix is not to loosen the setting but
to bring the installation inside the boundary:

1. `TEXMFOUTPUT` is the job directory.
2. The Worker finds the TeX trees with `kpsewhich` and links each into the job
   directory (`trees/dist`, `trees/debian`, ...), then points `TEXMFDIST`,
   `TEXMFMAIN`, `TEXMFDEBIAN`, `TEXMFSYSVAR`, `TEXMFSYSCONFIG` and
   `TEXMFLOCAL` at the links in its `texmf.cnf`. A tree that does not exist
   gets a path inside the job directory anyway, so the real location is never
   searched.
3. `TEXMFVAR`, `TEXMFCACHE`, `TEXMFCONFIG` and `HOME` are in the job
   directory.

The result: `\input`, `\openin`, `io.open`, `io.lines` and `dofile` can read
the TeX installation and the job's own files, and nothing else. The trees are
symlinks to read-only directories; the container root file system is read-only
in production, so a write through a link fails.

The `neutral` fonts show the consequence. The luaotfload font name database
cannot be built under paranoid reads, so a Template must load its fonts by
file (`Path=./fonts/`, see [packages.md](packages.md)). The default Latin
Modern fonts that LaTeX loads for code and math are found by file name and
work.

### Environment

The tools get exactly this environment and nothing from the parent:

| Variable | Value |
|----------|-------|
| `PATH` | `/usr/local/bin:/usr/bin:/bin` |
| `LC_ALL` | `C.UTF-8` (exists in the image) |
| `HOME` | `<job>/home` |
| `TMPDIR` | `<job>` |
| `TEXMFVAR`, `TEXMFCACHE` | `<job>/texmf-var` |
| `TEXMFCONFIG` | `<job>/texmf-config` |
| `TEXMFCNF` | `<job>/texmf-cnf:` and the installation's own path |
| `SOURCE_DATE_EPOCH`, `FORCE_SOURCE_DATE` | passed through only when set |

No secret, token or cookie can reach a child process through the environment.

### Font cache

luaotfload caches processed font data. `renderer warm-cache` renders a small
document of every Document Type once when the image is built and keeps the
cache in `/opt/renderer/texcache` (`RENDERER_TEXCACHE_DIR`). A job copies it
into its own `texmf-var`, which is writable; the shared copy is never written.
If the directory does not exist, the job builds its cache itself and is
slower.

### Runs, time and process groups

LuaLaTeX runs once, and a second time only if the log asks for it
("Rerun to get", "Label(s) may have changed"). It never runs a third time.

The whole job has a wall-clock limit, 60 seconds by default
(`--timeout`, `RENDERER_TIMEOUT`). Every tool runs in its own process group;
when the limit ends, or after any tool exits, the group is killed with
`SIGKILL`, so no `pandoc`, `rsvg-convert` or `lualatex` child outlives the
job.

## Job directory

Each job gets a new directory `job-<id>` below the work root
(`RENDERER_WORK_DIR`, default the system temporary directory), mode `0700`,
removed when the job ends, whatever the outcome. The id is 16 random hex
digits. Production mounts the work root as a tmpfs.

```
job-<id>/
  home/ texmf-var/ texmf-config/ texfonts/   engine state, empty or pre-built
  texmf-cnf/texmf.cnf  trees/                engine configuration, TeX trees
  filters/             the Lua filters
  shared/ fonts/       copied from the Template
  images/              the job's images by bundle name
  meta.yaml input.md   what Pandoc reads
  logo.pdf             the converted logo
  doc.tex doc.pdf doc.log
```

## Errors

Every failure is a `worker.Error` with a phase, a kind and the job id. It
never carries a TeX log or other tool output, because a log can quote the
Document.

| Phase | Kind | Meaning |
|-------|------|---------|
| `setup` | `invalid_job`, `internal` | The job is malformed, or the Worker could not prepare the directory. |
| `svg` | `tool_failed`, `invalid_job` | The logo could not be converted. |
| `pandoc` | `tool_failed` | Pandoc failed for a reason other than a filter. |
| `filter` | `raw_tex`, `raw_format`, `math`, `image_path` | A filter rejected the document. `raw_tex`, `raw_format` and `math` are internal errors. |
| `tex` | `tex_error`, `no_output` | LuaLaTeX failed or produced no PDF. |
| `timeout` | `timeout` | The wall-clock limit ended the job. |

The CLI prints the phase, kind and job id, and a hint:

```
renderer: render failed in phase tex (tex_error), job 3f1c9a0b5d7e2468: lualatex exited with code 1
  LaTeX could not typeset the document; --debug-tex-log prints the full log (it may contain document content)
```

## Logging

The Worker logs with `log/slog` to stderr. Failures are logged at `WARN` with
only these attributes: `job`, `phase`, `duration_ms`, `exit_code`, and, for
TeX failures, `tex_error`, the first line of the log that starts with `!`.
Successful phases are logged at `DEBUG` (`RENDERER_LOG_LEVEL=debug`). Neither
Document content nor metadata values are logged.

The first TeX error line can still name a control sequence or file the
Document used, as in `! Undefined control sequence.` or ``! LaTeX Error: File
`x' not found.``. It is the one deliberate exception.

### Debug switch

`--debug-tex-log`, or `RENDERER_DEBUG_TEX_LOG=1`, prints the full TeX log of
every run to stderr, between lines that say it may contain Document content.
It is off by default and meant for a developer's workstation. Do not set it in
a deployment.

## Configuration

| Setting | Flag | Environment | Default |
|---------|------|-------------|---------|
| Time limit | `--timeout` | `RENDERER_TIMEOUT` | `60s` |
| Full TeX log | `--debug-tex-log` | `RENDERER_DEBUG_TEX_LOG=1` | off |
| Work root | | `RENDERER_WORK_DIR` | system temporary directory |
| Font cache | | `RENDERER_TEXCACHE_DIR` | `/opt/renderer/texcache` |
| Log level | | `RENDERER_LOG_LEVEL` | `warn` |

## Limitations

- Math is not rendered; `$` is text.
- The engine lockdown confines TeX to the job directory and the TeX
  installation. It does not replace the container sandbox of ADR 0001: the
  Worker must still run with no network, no secrets and a read-only root.
- Lua can still read the whole TeX installation and the environment above.
  Neither holds anything secret.
- SVG images in the Document are not converted yet; the job bundle accepts
  PNG, JPEG and PDF files.
- Text inside SVG logos uses the fonts of the image; a missing font is
  substituted silently. Export text as paths.
- Glyphs missing from a Template's fonts are dropped by LuaLaTeX with only a
  log warning, which the Worker does not turn into an error.

## Tests

- Tier 1 (`make test`): environment, engine configuration, process-group kill,
  error shape, CLI mapping.
- Tier 2 (`make test-golden`, in the image): PDF goldens for every fixture
  topic; the Worker tests (filters on hand-made ASTs, engine lockdown with
  hostile TeX, timeout, run count, debug log, cache); security layer 1.
- Security layer 1 (`make test-security`): a corpus of hostile Documents,
  metadata values included, rendered through the real binary. Each payload
  class has one outcome, `literal`, `dropped` or `rejected`. A random canary
  file is created per test and its content must never reach the PDF; a payload
  that would write a marker file via Lua or a shell must leave no file. No
  test in this suite skips.
