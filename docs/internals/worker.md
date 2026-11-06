---
sidebar_position: 3
---

# Worker pipeline

The Worker turns one job into one PDF. The same pipeline runs behind the
`outprint render` command and behind the Worker's HTTP endpoint
(`outprint worker`, see [Architecture](architecture.md)). This page
describes what it does, how it is locked down, what it logs and where it is
limited. The reasons for the two-role split are in
[ADR 0001](../adr/0001-split-renderer-into-gateway-and-worker.md).

## Phases

A job runs through these phases. A failure names the phase and a kind, plus
the job id; see [Errors](#errors).

| Phase | What happens |
|-------|--------------|
| `setup` | Create the job directory, find the TeX installation, write the engine configuration, copy the Template's shared files and fonts, write the bundled raster images and copy the pre-built font cache. |
| `svg` | The first phase of the job proper: convert every SVG of the bundle and the Configuration's SVG logo to a vector PDF with `rsvg-convert`; see [SVG phase](#svg-phase). |
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

A filter failure is raised as `outprint-filter:<kind>: <fixed text>`. The
Worker reads only the kind; the text never contains Document content.

## SVG phase

Every bundle image whose name ends in `.svg` (the Gateway names them
`img-<n>.svg`) and the Configuration's logo are converted before Pandoc runs:

```
rsvg-convert -f pdf -     (SVG on standard input, PDF on standard output)
```

- **Input and output are streams.** The SVG is not written to the job
  directory. Without a file, librsvg has no base location, so it resolves no
  relative reference and loads nothing but `data:` URIs, whatever the SVG
  asks for. The PDF is read from standard output into memory and written to
  `images/<name>.pdf` (the logo: `logo.pdf`) only when it is complete.
- **`--unlimited` is never passed**, so librsvg keeps its own limits (XML
  size, 50 nested layers, 500 000 referenced elements). Only `-f pdf` and `-`
  are passed.
- **Own environment**: `PATH`, `LC_ALL`, `HOME=<job>/home`, `TMPDIR=<job>`
  and `SOURCE_DATE_EPOCH` if set. Nothing is inherited, in particular no
  `XML_CATALOG_FILES` and no proxy variable.
- **Limits per file**: 20 s (`limits.SVGConvertTimeout`, bounded by what is
  left of the job's time), a source of at most 2 MiB, a PDF of at most 10 MiB
  (`limits.MaxSVGPDFBytes`). The output limit is enforced while the PDF is
  read: the write that would pass it fails and the process group is killed, so
  a runaway conversion costs at most 10 MiB of memory in the Worker.
  Memory in the converter itself is bounded by the container (1 GiB; a kill
  is reported as `resource`).
- **Process group**: like every tool, `rsvg-convert` runs in its own process
  group, which is killed on timeout, on the output limit and after exit.
- **Logging**: on failure, the job id, the kind, the exit code and the first
  line of the converter's error, reduced to printable ASCII and cut at 120
  characters. That line can name an XML element or a position; the error
  returned to the caller never holds tool output.

Afterwards the Lua filter maps each `images/<name>.svg` reference to
`images/<name>.pdf` and checks that this file exists in the job; every other
check on image paths is unchanged. The Template's `\includegraphics` places
the PDF, scaled to the size the Gateway wrote. A PDF page is a vector
picture, so it stays sharp; embedded pictures inside the SVG stay raster.

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
file (`Path=./fonts/`, see [Template package](../reference/template-package.md)). The default Latin
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

`rsvg-convert` gets only the first four and `SOURCE_DATE_EPOCH`. No secret,
token or cookie can reach a child process through the environment.

### Font cache

luaotfload caches processed font data. `outprint warm-cache` renders a small
document of every Document Type once when the image is built and keeps the
cache in `/opt/outprint/texcache`. A job copies it
into its own `texmf-var`, which is writable; the shared copy is never written.
If the directory does not exist, the job builds its cache itself and is
slower.

### Runs, time and process groups

LuaLaTeX runs once, and a second time only if the log asks for it
("Rerun to get", "Label(s) may have changed"). It never runs a third time.

The whole job has a wall-clock limit, 60 seconds by default
(`--timeout`, `OUTPRINT_TIMEOUT`). Every tool runs in its own process group;
when the limit ends, or after any tool exits, the group is killed with
`SIGKILL`, so no `pandoc`, `rsvg-convert` or `lualatex` child outlives the
job.

## Job directory

Each job gets a new directory `job-<id>` below the work root
(default the system temporary directory), mode `0700`,
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
  images/*.pdf         the converted SVGs, next to the raster images
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
| `svg` | `svg_convert`, `timeout`, `output_too_large`, `resource`, `invalid_job` | An SVG or the logo could not be converted: `rsvg-convert` failed or produced no PDF; took more than 20 s (the Worker restarts, as for any `timeout`); produced more than 10 MiB; ran out of memory; or the logo or an SVG name is invalid or the SVG is over 2 MiB. The Gateway answers 504 for `timeout` and 413 for `output_too_large`. |
| `pandoc` | `tool_failed` | Pandoc failed for a reason other than a filter. |
| `filter` | `raw_tex`, `raw_format`, `math`, `image_path` | A filter rejected the document. `raw_tex`, `raw_format` and `math` are internal errors. |
| `tex` | `tex_error`, `no_output` | LuaLaTeX failed or produced no PDF. |
| `timeout` | `timeout` | The wall-clock limit ended the job. |
| `output` | `pages_exceeded`, `pdf_too_large` | The PDF has more than `OUTPRINT_MAX_PAGES` pages, or is larger than `OUTPRINT_MAX_PDF_BYTES`. The Worker answers 413 and keeps running. |
| any | `resource` | A tool was killed by the system (out of memory), or the job directory (tmpfs) is full. The Worker answers with the error and then exits, so the container runtime starts a fresh process. |

The CLI prints the phase, kind and job id, and a hint:

```
outprint: render failed in phase tex (tex_error), job 3f1c9a0b5d7e2468: lualatex exited with code 1
  LaTeX could not typeset the document; --debug-tex-log prints the full log (it may contain document content)
```

## Logging

The Worker logs with `log/slog` to stderr. Failures are logged at `WARN` with
only these attributes: `job`, `phase`, `duration_ms`, `exit_code`, and, for
TeX failures, `tex_error`, the first line of the log that starts with `!`.
Successful phases are logged at `DEBUG` (`OUTPRINT_LOG_LEVEL=debug`). Neither
Document content nor metadata values are logged.

The first TeX error line can still name a control sequence or file the
Document used, as in `! Undefined control sequence.` or ``! LaTeX Error: File
`x' not found.``. It is the one deliberate exception.

### Debug switch

`--debug-tex-log` prints the full TeX log of
every run to stderr, between lines that say it may contain Document content.
It is off by default and meant for a developer's workstation. Do not set it in
a deployment.

## Configuration

| Setting | Flag | Environment | Default |
|---------|------|-------------|---------|
| Time limit | `--timeout` | `OUTPRINT_TIMEOUT` | `60s` |
| Page limit | | `OUTPRINT_MAX_PAGES` | `500` |
| PDF size limit | | `OUTPRINT_MAX_PDF_BYTES` | `52428800` (50 MiB; can be lowered, not raised) |
| Full TeX log | `--debug-tex-log` | | off |
| Log level | | `OUTPRINT_LOG_LEVEL` | `warn` for `render`, `info` for the Worker service |

## Limitations

- Math is not rendered; `$` is text.
- The engine lockdown confines TeX to the job directory and the TeX
  installation. It does not replace the container sandbox of ADR 0001: the
  Worker must still run with no network, no secrets and a read-only root.
- Lua can still read the whole TeX installation and the environment above.
  Neither holds anything secret.
- Text inside SVGs, Document images and logos alike, uses the fonts of the
  image; a missing font is substituted silently (`sans-serif` and unknown
  names become a TeX Gyre font). Export text as paths. See
  [Images in the pipeline](images.md#svg).
- The Worker does not repeat the Gateway's SVG pre-check. It relies on
  librsvg's own limits, the limits above and the container; the attack corpus
  is run against exactly this (see [Security](security.md)).
- Glyphs missing from a Template's fonts are dropped by LuaLaTeX with only a
  log warning, which the Worker does not turn into an error.

## Tests

- Tier 1 (`task test`): environment, engine configuration, process-group kill,
  error shape, CLI mapping.
- Tier 2 (`task test-golden`, in the image): PDF goldens for every fixture
  topic; the Worker tests (filters on hand-made ASTs, engine lockdown with
  hostile TeX, timeout, run count, debug log, cache); security layer 1.
- Security layers 2 and 3 (`task test-sandbox`): the Worker image under the
  flags of the shipped Compose file, with hostile actions and resource bombs.
  See [Security](security.md).
- Security layer 1 (`task test-security-layer1`): a corpus of hostile Documents,
  metadata values included, rendered through the real binary. Each payload
  class has one outcome, `literal`, `dropped` or `rejected`. A random canary
  file is created per test and its content must never reach the PDF; a payload
  that would write a marker file via Lua or a shell must leave no file. No
  test in this suite skips.
