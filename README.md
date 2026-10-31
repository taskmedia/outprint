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
make sanitize        # scan tree and history for forbidden strings
make test-sanitize   # test the sanitize gate itself
```

See [docs/sanitize.md](docs/sanitize.md) for the sanitisation gate.

## License

[MIT](LICENSE)
