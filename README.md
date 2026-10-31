# outline-latex-pdf

Turns [Outline](https://www.getoutline.com/) documents into typeset PDFs.

A **Template** provides the look and structure (letter, report, ...). A
**Template Configuration** adapts it to an organisation or purpose (name, logo,
colours, defaults). A Collection admin binds a Template and a Configuration to a
Collection; a Document author fills structured fields in a `yaml metadata`
block and downloads the PDF with one click.

## Components

- **renderer**: one image, two roles. The *Gateway* talks to Outline and builds
  a job bundle; the *Worker* has no network and no secrets and only turns a
  bundle into a PDF with Pandoc and LuaLaTeX. It also offers `render` and
  `validate` CLI subcommands that run the same pipeline without Outline.
- **injector**: a small reverse proxy in front of Outline that adds a
  "Download PDF" button without forking Outline.

See [CONTEXT.md](CONTEXT.md) for the vocabulary and
[docs/adr](docs/adr) for architecture decisions.

## Status

Work in progress. Tracked in the project issues.

## Development

```sh
make build           # build bin/renderer
make test            # tier 1: Go unit tests, no external tools
make image           # build the renderer image
make test-golden     # tier 2: PDF goldens, run inside the pinned image
make update-goldens  # regenerate goldens, only ever inside the pinned image
make sanitize        # scan tree and history for forbidden strings
make test-sanitize   # test the sanitize gate itself
```

Render a letter with the shipped Template and Configuration:

```sh
docker build -t renderer .
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/data" renderer render --template neutral \
  --config example --date 2026-03-02 -o /data/out.pdf /data/testdata/fixtures/letter-full/input.md
```

Goldens compare `pdfinfo`/`pdffonts` facts and `pdftotext -layout` text, never
PDF bytes. Fixtures live in `testdata/fixtures/<topic>/`, goldens in
`testdata/golden/<topic>/`.

Check Templates and Configurations before deploying them:

```sh
docker run --rm renderer validate                  # the packages shipped in the image
renderer validate --templates-root ./templates --configs-root ./configurations
```

See [docs/metadata.md](docs/metadata.md) for the Metadata Block authors write
(syntax, layering, field types, errors),
[docs/packages.md](docs/packages.md) for the package format, field types,
typed patterns, the TeX deny-list, the drift guard and `validate`, and
[docs/standard-fields.md](docs/standard-fields.md) for the Standard Fields.
See [docs/sanitize.md](docs/sanitize.md) for the sanitisation gate.

## License

[MIT](LICENSE)
