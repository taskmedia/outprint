---
sidebar_position: 2
---

# Template author guide

**How-to.** Write a Template or a Configuration.

This guide is for people who write a [Template](../reference/glossary.md#template)
(a style family with one or more
[Document Types](../reference/glossary.md#document-type), such as a letter or a
report) or a [Configuration](../reference/glossary.md#configuration) (the values
that adapt a Template to an organisation). It ties together the reference pages
and walks through one complete example. Terms are in the
[Glossary](../reference/glossary.md).

It builds on the [Reference](../reference/README.md) pages and the
[Template catalogue](catalogue/README.md). Background on the engine:
[Worker pipeline](https://github.com/taskmedia/outprint/blob/main/docs/internals/worker.md) and
[Images in the pipeline](https://github.com/taskmedia/outprint/blob/main/docs/internals/images.md).

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

Every key is listed in [Template package](../reference/template-package.md).

### Field types and typed patterns

Text values are escaped by Pandoc's LaTeX writer. Values in non-text positions
(a colour, a path, a length) are not, so they need a strict pattern: see
[Typed patterns](../reference/template-checks.md#typed-patterns). Prefer the
narrowest pattern that does the job. In `neutral`, the accent colour is
`'^[0-9A-Fa-f]{6}$'` and the logo is a path ending in `.svg`.

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

## Safety checks and reuse

Every TeX file is scanned for constructs that run code or touch files
([TeX deny-list](../reference/template-checks.md#tex-deny-list)). It is a safety
net against accidents, not a sandbox: the Worker still runs without network or
secrets and with shell escape off
([Security](https://github.com/taskmedia/outprint/blob/main/docs/internals/security.md)).
Reuse the names in [Standard Fields](../reference/standard-fields.md) so that
Configurations stay portable between Templates. A Configuration's Defaults and
Fixed Values are in [Configuration package](../reference/configuration-package.md).

## Try it

Run `validate` and `render` as described in [Try your Template](try-your-template.md).

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
   `yaml` block at its start ([Metadata Block](../reference/metadata-block.md)):

   <!-- validate: skip Metadata Block for the derived mine Template of the worked example, which is not shipped -->
   ````markdown
   ```yaml
   subject: Opening hours
   department: Front desk
   ```
   Our office is open from Monday to Friday.
   ````

   Save it as `input.md` and render it with the command in
   [Try your Template](try-your-template.md#render).

6. To offer it to Collections, add the pair to `bindings` in the service
   configuration ([Template package](../reference/service-configuration.md)) and bake
   the package into the image or mount it.

## Checklist before you publish a Template

- `validate` is clean (no errors; warnings understood).
- Fonts load by path, are redistributable, and are listed in
  `THIRD_PARTY_NOTICES` with their licence text.
- Every non-text field has a narrow pattern.
- A golden fixture covers each Document Type.
- No organisation-specific names, logos or addresses in a public repository:
  run `task sanitize` ([Sanitisation gate](https://github.com/taskmedia/outprint/blob/main/docs/sanitize.md)).
