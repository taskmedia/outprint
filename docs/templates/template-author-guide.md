---
sidebar_position: 2
---

# Template author guide

**How-to.** Write a Template or a Template Configuration.

This guide is for people who write a **Template** (a style family such as a
letter or a report) or a **Template Configuration** (the values that adapt a
Template to an organisation). It ties together the reference pages and walks
through one complete example. Terms are defined in [Vocabulary](https://github.com/taskmedia/outprint/blob/main/CONTEXT.md).

Reference pages this guide builds on:

- [Packages](packages.md): package format, field types, typed patterns, the
  TeX deny-list, drift guard, `validate`
- [Standard Fields](standard-fields.md): the Standard Fields
- [Metadata Block reference](metadata-reference.md): what a Document author can write
  and how values are layered
- [Template catalogue](catalogue/README.md): the shipped Templates, as the
  authors see them
- [Worker pipeline](https://github.com/taskmedia/outprint/blob/main/docs/internals/worker.md): what the engine allows, and why
- [Images in the pipeline](https://github.com/taskmedia/outprint/blob/main/docs/internals/images.md): how images are checked and placed

## The model in one minute

A Template package declares **fields** and one or more **Document Types**
(each with its own Pandoc/LaTeX file). A Configuration package supplies
**Fixed Values** (a Document cannot change them) and **Defaults** (a Document
may override them). A Document's Metadata Block sets the rest. For every field
the highest layer wins: Fixed Values, then the Document, then Configuration
Defaults, then manifest defaults and derived values (`title`, `date`).

Everything a Document or a Configuration can put into the TeX source goes
through a declared field with a type. That is what keeps authors from
injecting TeX.

## Package layout

```
templates/mine/
  manifest.yaml        fields and Document Types
  letter.latex         one Pandoc template per Document Type
  shared/mine.tex      shared TeX parts, included with \input{shared/mine.tex}
  fonts/               optional; copied to ./fonts/ in every job
  assets/              optional; package-relative files

configurations/example/
  configuration.yaml
  logo.svg
```

Directory names match `^[a-z0-9][a-z0-9-]*$`. A package is self-contained:
nothing is shared between packages, paths stay inside the package (also after
resolving symlinks), and Git LFS pointer stubs are rejected.

## manifest.yaml

<!-- validate: skip reduced manifest fragment, not a complete Template package -->
```yaml
manifest_version: 1          # must be 1

fields:                      # each field is declared once
  subject:
    type: string
    description: Subject line.
  accent_color:
    type: color
    pattern: '^[0-9A-Fa-f]{6}$'

document_types:
  letter:
    template: letter.latex
    fields:
      - accent_color
      - subject:
          required: true     # override for this Document Type only
```

A field has a `type`, and optionally `required`, `description`, `default`,
`pattern` and (for `enum`) `values`. Field names match `^[a-z][a-z0-9_]*$`.
Unknown keys anywhere are errors. See [Packages](packages.md#manifestyaml).

### Field types

| Type     | Value                               | Where it may go in TeX |
| -------- | ----------------------------------- | ---------------------- |
| `string` | one line of text                    | text                   |
| `text`   | multi-line text                     | text                   |
| `list`   | list of strings                     | text                   |
| `date`   | a date (free text or a YAML date)   | text                   |
| `bool`   | `true` or `false`                   | condition (`$if$`)     |
| `enum`   | one of `values`                     | command or option     |
| `color`  | checked by `pattern`                | command or option     |
| `asset`  | package-relative path, `pattern`    | command or option     |

Text values are escaped by Pandoc's LaTeX writer. Values in **non-text
positions** (the argument of `\definecolor`, `\includegraphics`,
`\setlength`, `\usepackage`, `\setmainfont`, any option list in square
brackets, and so on) are not escaped for you, so they need a strict pattern.

### Typed patterns

- `color` and `asset` fields must have a `pattern`; `enum` fields must have
  `values` made of letters, digits, space, `.`, `_`, `-`.
- Any other field used in a non-text position must have a `pattern` too.
- A pattern is an RE2 expression matched against the whole value. It must not
  match any of a set of hostile probes (backslash, braces, `%`, `$`, `&`, `#`,
  `~`, `^`, `|`, quotes, newlines, `..`, a leading `/`). `.*` and `[^ ]+` are
  rejected as too permissive.
- Defaults must pass their own type and pattern. An asset value must be a
  clean relative path without `..`.

Prefer the narrowest pattern that does the job. In `neutral`, the accent
colour is `'^[0-9A-Fa-f]{6}$'` and the logo is a path ending in `.svg`.

## Pandoc templates

Each Document Type names one Pandoc template (`.latex`). It uses Pandoc's
variable syntax: `$name$`, `$if(name)$ ... $else$ ... $endif$`,
`$for(name)$ ... $sep$ ... $endfor$`. Partials and applied functions are not
supported, and a variable may never build a control sequence name
(`\foo$name$`).

Things the Pandoc LaTeX writer needs from a template (taken from the
[fidelity research](https://github.com/taskmedia/outprint/blob/main/docs/research/markdown-latex-fidelity.md) and what `neutral`
does):

- `longtable`, `booktabs`, `array`, `calc`, `etoolbox` for tables, and
  `\tightlist` and `\pandocbounded` defined (see `shared/neutral.tex`);
- the Pandoc variable `highlighting-macros` inside `$if(...)$`, for code;
- sectioning commands: a letter class has none, so `neutral` defines them as
  bold lines;
- guard optional fields (`$if(recipient)$`) so a missing value does not break
  the layout.

### Fonts

Fonts are loaded by **file path** with `Path=./fonts/`, never by name (the
engine cannot build the font name database under paranoid reads; see
[Worker pipeline](https://github.com/taskmedia/outprint/blob/main/docs/internals/worker.md#reading-below-the-job-directory-only)):

```tex
\setmainfont{texgyrepagella-regular.otf}[Path=./fonts/,
  BoldFont=texgyrepagella-bold.otf, ItalicFont=texgyrepagella-italic.otf,
  BoldItalicFont=texgyrepagella-bolditalic.otf]
```

Ship only fonts whose licence allows redistribution, unmodified, with their
licence text, and list them in `THIRD_PARTY_NOTICES`. `neutral` ships TeX Gyre
under the GUST Font License. Do not ship proprietary fonts in a public
repository.

### Images and logos

A logo is an `asset` field. SVG files are converted to vector PDF; raster
formats are limited as described in [Images in the pipeline](https://github.com/taskmedia/outprint/blob/main/docs/internals/images.md). Text inside
an SVG is drawn with the fonts of the Worker image, not the Template's fonts.

**Templates** must scale images to the page. The Gateway gives every image
an explicit size in points, so only the Template knows the text area. The
`neutral` Template redefines `\includegraphics` to scale an image down to the
line width and `0.9\textheight`, keeping the aspect ratio, and sets figure
captions through the `caption` package. Its `letter` has no float: the KOMA
letter class has none, so the Template defines a `figure` environment that
keeps the figure in place on one page, with the same caption style and
numbering as the report. A Template that shows Document images
must do the same.

## The TeX deny-list

Every TeX file of a Template (`.latex`, `.tex`, `.sty`, `.cls`, `.def`, `.cfg`,
`.ltx`, `.clo`) is scanned after comments are removed. These are errors:

- Lua and shell execution (`\directlua`, `\latelua`, `\luaexec`,
  `\ShellEscape`, `luacode`, `minted`, `pythontex`, `markdown`, ...);
- writing files (`\write`, `\newwrite`, `\openout`, `\immediate`, ...);
- reading files (`\read`, `\openin`, `\verbatiminput`, `\CatchFileDef`, ...);
- inclusion (`\include`, `\InputIfFileExists`, `\@@input`, ...);
- obfuscation (`^^` notation, `\catcode`, `\csname`, `\scantokens`);
- `\input` unless written exactly `\input{shared/NAME}` for a file in the
  package's own `shared/` directory;
- `\includegraphics` with a literal absolute or `..` path;
- any `.lua` file in the package.

The full list is in [Packages](packages.md#tex-deny-list). It is a safety
net against accidents and careless Templates, not a sandbox: the Worker still
runs without network or secrets and with shell escape off
([Security](https://github.com/taskmedia/outprint/blob/main/docs/internals/security.md)).

## Standard Fields

Reuse the Standard Fields (`title`, `date`, `language`, `author`, `name`,
`logo`, `accent_color`, letter and report fields) with the listed types so
that Configurations stay portable between Templates. The page
[Standard Fields](standard-fields.md) and the machine-readable table
`internal/packages/standard_fields.yaml` (used by the loader) are kept in step
by a test, and `neutral` declares its fields with those types. `validate`
warns, never fails, when a Template uses a Standard Field name with another
type. A Template may declare any other field it likes; `font`, for example, is
a Standard Field that `neutral` does not declare.

## Configuration packages

<!-- validate: configuration template=neutral configuration=example -->
```yaml
manifest_version: 1

defaults:                 # a Document may override these
  document_type: letter
  closing: Kind regards

fields:                   # Fixed Values: a Document cannot change these
  name: Example Garden Society
  logo: logo.svg
  accent_color: "2F6F5E"
```

When a Configuration is bound to a Template, every value of a declared field
is checked against that field. A Fixed Value the Template does not declare has
no effect and gives a warning, so one Configuration can serve several
Templates. `extends` is reserved and not supported in v1. See
[Packages](packages.md#configuration-package).

## validate and render

Both commands are subcommands of the `outprint` binary (or the container
image, whose entrypoint it is). The usage is printed by `outprint --help`.

```text
outprint validate [--service-config FILE]
                  [--templates-root DIR] [--configs-root DIR]
                  [--template NAME] [--config NAME]

outprint render --template NAME --config NAME [--title TEXT] [--date YYYY-MM-DD]
                [--json-errors] [--templates-root DIR] [--configs-root DIR]
                [--timeout DURATION] [--debug-tex-log]
                -o OUTPUT.pdf INPUT.md
```

**`validate`** runs the same loader as service startup: manifests, field
types and patterns, the TeX deny-list, the drift guard, path confinement, and
(with `--template` and `--config`) the pair checks. All findings are
collected, one per line; errors give exit code 1, warnings alone are success.

```sh
task build
bin/outprint validate --templates-root ./templates --configs-root ./configurations
bin/outprint validate --templates-root ./templates --configs-root ./configurations \
  --template mine --config example
```

**`render`** runs the real pipeline (Pandoc and LuaLaTeX) on a Markdown file.
It needs the toolchain, so use the image, which is also what the golden tests
use:

```sh
docker build -t outprint .
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/data" outprint render \
  --templates-root /data/templates --configs-root /data/configurations \
  --template mine --config example --date 2026-03-02 \
  -o /data/out.pdf /data/input.md
```

`--debug-tex-log` prints the full TeX log (it may contain document content;
see [Worker pipeline](https://github.com/taskmedia/outprint/blob/main/docs/internals/worker.md)); `--json-errors` prints the JSON error body on
stdout ([Metadata Block reference](metadata-reference.md)).

### Drift guard

`validate` and startup compare the variables a Pandoc template uses with the
manifest: an undeclared variable, or one not listed by the Document Type that
uses it, is an **error**; a declared field no template uses is a **warning**.
Pandoc's own variables (`body`, `highlighting-macros`, `lang`, ...) are
ignored. See [Packages](packages.md#drift-guard).

## Goldens for Template authors

The repository's golden tests render Markdown fixtures through the real
pipeline inside the pinned image and compare **facts and text**, never PDF
bytes: `pdfinfo` and `pdffonts` facts, and `pdftotext -layout` output, plus
assertions such as "fonts are embedded", "this string survives", "this table
header repeats on each page".

To cover your Template:

1. Add a fixture directory `testdata/fixtures/<topic>/input.md`.
2. Add a case to the table in `internal/golden/golden_test.go` naming the
   Template and Configuration, and any literal or forbidden strings.
3. Generate the goldens inside the image only, then review the diff:
   `task update-goldens` (writes `testdata/golden/<topic>/`), and run
   `task test-golden`.

**Not available yet:** a per-Document-Type opt-in to compare rasterised pages
(PNG diff). The research suggested it as an optional narrow check
([test environment research](https://github.com/taskmedia/outprint/blob/main/docs/research/test-environment.md)), but it is not
implemented: there is no PNG golden and no manifest or fixture switch for it.
Today, layout regressions are caught by the text goldens and by the asserted
facts only. If you need visual checks, render and look at the PDF; do not rely
on a pixel gate that does not exist.

## Worked example: a letter Template derived from `neutral`

Goal: a `mine` Template with only the letter Document Type and an extra
`department` field printed in the letter's place line. The example was checked
with `validate` and `render` against the image of this repository.

1. Copy the package and remove what you do not need:

   ```sh
   cp -R templates/neutral templates/mine
   rm templates/mine/report.latex
   ```

2. Edit `templates/mine/manifest.yaml`: delete the `report:` Document Type and
   the report-only fields (`subtitle`, `version`, `abstract`, `toc`; otherwise
   `validate` warns that they are never used), then declare the new field and
   list it in the letter:

   <!-- validate: skip fragment with ellipses, edits the derived mine Template of the worked example -->
   ```yaml
   fields:
     # ...existing fields...
     department:
       type: string
       description: Department shown in the letter's place line.
       pattern: '^[A-Za-z0-9 .,-]{1,40}$'

   document_types:
     letter:
       template: letter.latex
       fields:
         # ...existing entries...
         - department
   ```

   The field is printed as text, so the pattern is not strictly needed for
   safety; it keeps the value short and plain.

3. Use it in `templates/mine/letter.latex`, next to the subject:

   ```tex
   \setkomavar{subject}{$subject$}
   $if(department)$
   \setkomavar{place}{$department$}
   $endif$
   ```

4. Check it:

   ```sh
   go run ./cmd/outprint validate --templates-root ./templates \
     --configs-root ./configurations --template mine --config example
   ```

   Remove the `department` line from the letter and run it again: the drift
   guard now warns that the field is never used. Use `$department$` without
   listing the field and it is an error.

5. Render a Document that sets it. The Metadata Block of a Document is a
   `yaml` block at its start ([Metadata Block reference](metadata-reference.md)):

   <!-- validate: skip Metadata Block for the derived mine Template of the worked example, which is not shipped -->
   ````markdown
   ```yaml
   subject: Opening hours
   department: Front desk
   ```
   Our office is open from Monday to Friday.
   ````

   ```sh
   docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/data" outprint render \
     --templates-root /data/templates --configs-root /data/configurations \
     --template mine --config example --date 2026-03-02 \
     -o /data/out.pdf /data/input.md
   ```

6. To offer it to Collections, add the pair to `bindings` in the service
   configuration ([Packages](packages.md#service-configuration)) and bake
   the package into the image or mount it.

## Checklist before you publish a Template

- `validate` is clean (no errors; warnings understood).
- Fonts load by path, are redistributable, and are listed in
  `THIRD_PARTY_NOTICES` with their licence text.
- Every non-text field has a narrow pattern.
- A golden fixture covers each Document Type.
- No organisation-specific names, logos or addresses in a public repository:
  run `task sanitize` ([Sanitisation gate](https://github.com/taskmedia/outprint/blob/main/docs/sanitize.md)).
