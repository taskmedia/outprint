<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/svg/outprint-banner-dark.svg">
    <img alt="Outprint: PDFs from Outline documents" src="docs/assets/logo/svg/outprint-banner.svg" width="440">
  </picture>
</p>

Turn [Outline](https://www.getoutline.com/) documents into typeset PDFs (letters, reports, custom LaTeX templates) with a Download PDF button. Self-hosted, no Outline fork.

Pick a Template, bind it to an Outline Collection, write a few fields at the
top of a Document and download a typeset PDF: letters, reports or your own
design in LaTeX, with fixed organisation values (name, logo, colours) kept apart
from the content. Nothing in Outline is forked.

A **Template** provides the look and structure (letter, report, ...). A
**Template Configuration** adapts it to an organisation or purpose (name, logo,
colours, defaults). A Collection admin binds a Template and a Configuration to a
Collection; a Document author fills structured fields in a leading
`yaml` block and downloads the PDF with one click.

## Components

- **renderer**: one image, two roles. The *Gateway* talks to Outline and builds
  a job bundle; the *Worker* has no network and no secrets and only turns a
  bundle into a PDF with Pandoc and LuaLaTeX. It also offers `render` and
  `validate` CLI subcommands that run the same pipeline without Outline.
- **injector**: a small reverse proxy in front of Outline that adds a
  "Download PDF" button without forking Outline.

See [CONTEXT.md](CONTEXT.md) for the vocabulary and
[docs/adr](docs/adr) for architecture decisions.

## Quick start

You need Docker, Go and `make`.

```sh
make dev     # Outline with login, the injector, Gateway and Worker, seeded, at http://localhost:8090
make e2e     # optional: the browser tests against that stack
```

The log-in, the seed data and what to click are described in
[docs/development.md](docs/development.md). To try only the renderer, see
"Development" below.

## Documentation

Start at the index, [docs/README.md](docs/README.md). Template authors begin
with the [Template author guide](docs/template-author-guide.md). The research
behind the design is in [docs/research](docs/research/README.md).

## Status

Work in progress. Tracked in the project issues. Known gaps are listed there.

## Development

```sh
make build           # build bin/outprint
make test            # tier 1: Go unit tests, and the button script tests when Node is installed
make image-injector  # build the slim injector image
make image           # build the outprint image
make test-golden     # tier 2: PDF goldens, Worker and security tests, in the pinned image
make test-security   # security layers 1-3: hostile payloads, sandbox, resource bombs
make test-sandbox    # security layers 2 and 3 only (needs a Docker daemon)
make update-goldens  # regenerate goldens, only ever inside the pinned image
make sanitize        # scan tree and history for forbidden strings
make test-sanitize   # test the sanitize gate itself
make audit           # pre-publish audit: sanitize plus issues and pull requests via gh
make dev             # Outline + injector + Gateway + Worker, seeded, at http://localhost:8090
make e2e             # tier 3: browser tests against that stack
```

The local stack, its login, the seeding and the end-to-end tests are described
in [docs/development.md](docs/development.md).

Render a letter with the shipped Template and Configuration:

```sh
docker build -t outprint .
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/data" outprint render --template neutral \
  --config example --date 2026-03-02 -o /data/out.pdf /data/testdata/fixtures/letter-full/input.md
```

Goldens compare `pdfinfo`/`pdffonts` facts and `pdftotext -layout` text, never
PDF bytes. Fixtures live in `testdata/fixtures/<topic>/`, goldens in
`testdata/golden/<topic>/`.

Check Templates and Configurations before deploying them:

```sh
docker run --rm outprint validate                  # the packages shipped in the image
outprint validate --templates-root ./templates --configs-root ./configurations
```

See [docs/metadata.md](docs/metadata.md) for the Metadata Block authors write
(syntax, layering, field types, errors),
[docs/packages.md](docs/packages.md) for the package format, field types,
typed patterns, the TeX deny-list, the drift guard and `validate`, and
[docs/standard-fields.md](docs/standard-fields.md) for the Standard Fields.
See [docs/images.md](docs/images.md) for images: formats, sizes, captions,
limits, error kinds and the `imageHosts` setting.
See [docs/binding.md](docs/binding.md) for the `binding:` block a
Collection admin uses to choose the Template and Configuration.
See [docs/worker.md](docs/worker.md) for the Worker pipeline: phases, filters,
engine lockdown, errors, logging and the debug switch.
See [docs/architecture.md](docs/architecture.md) for the Gateway and Worker
roles, the job bundle, the request flow and the error mapping, and
[docs/deploy-compose.md](docs/deploy-compose.md) for running both with Docker
Compose and the Worker security baseline.
See [docs/injector.md](docs/injector.md) for the injector, the download button,
compression and CSP handling, routing and the security notes.
See [docs/security.md](docs/security.md) for the security layers, what each
proves, the limits and how to run the suite, and
[docs/testing.md](docs/testing.md) for the test tiers and the CI schedule.
See [docs/limits.md](docs/limits.md) for the limits and admission control
(slots, queue, per-user limit, and how to configure them).
See [docs/sanitize.md](docs/sanitize.md) for the sanitisation gate and the
pre-publish audit, and [docs/publishing.md](docs/publishing.md) for the
maintainer's checklist.

## License

[MIT](LICENSE). The outprint image also contains Pandoc, TeX Live, librsvg and
fonts under their own licences; see [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES),
which includes the written source offer. Operators remain bound by Outline's
own licence for their Outline instance.
