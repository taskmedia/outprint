<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/svg/outprint-banner-dark.svg">
    <img alt="Outprint: PDFs from Outline documents" src="docs/assets/logo/svg/outprint-banner.svg" width="440">
  </picture>
  <br><br>
</p>

Turn [Outline](https://www.getoutline.com/) documents into typeset PDFs (letters, reports, custom LaTeX templates) with a Formatted PDF action in the Export menu. Self-hosted, no Outline fork.

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
  "Formatted PDF" action to Outline's Export menu without forking Outline.

See the [vocabulary](CONTEXT.md) and the
[architecture decision records](docs/adr/README.md).

## Quick start

You need Docker, Go and [Task](https://taskfile.dev) 3.54 or newer
(`brew install go-task`, or see the
[installation guide](https://taskfile.dev/installation/)). `task` alone lists
every task.

```sh
task dev # Outline with login, the injector, Gateway and Worker, seeded, at http://localhost:8090
task e2e:run # optional: the browser tests against that stack
```

The log-in, the seed data and what to click are described in
[Development stack and end-to-end tests](docs/internals/development.md). To try only the renderer, see
"Development" below.

## Documentation

The docs are at <https://outprint.task.media> and in [docs/](docs/README.md), in
five sections: Write, Configure, Deploy, Templates and Internals. Template authors
begin with the [Template author guide](docs/templates/template-author-guide.md). The research
behind the design is in the [research notes](docs/research/README.md).

## Status

Work in progress. Tracked in the project issues. Known gaps are listed there.

## Development

```sh
task build           # build bin/outprint
task test            # tier 1: Go unit tests, and the button script tests when Node is installed
task image-injector  # build the slim injector image
task image           # build the outprint image
task test-golden     # tier 2: PDF goldens, Worker and security tests, in the pinned image
task test-security   # security layers 1-3: hostile payloads, sandbox, resource bombs
task test-sandbox    # security layers 2 and 3 only (needs a Docker daemon)
task update-goldens  # regenerate goldens, only ever inside the pinned image
task sanitize        # scan tree and history for forbidden strings
task test-sanitize   # test the sanitize gate itself
task audit           # pre-publish audit: sanitize plus issues and pull requests via gh
task dev              # Outline + injector + Gateway + Worker, seeded, at http://localhost:8090
task e2e:run         # tier 3: browser tests against that stack
```

The local stack, its login, the seeding and the end-to-end tests are described
in [Development stack and end-to-end tests](docs/internals/development.md).

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

The reference for each part is in the docs: the [Metadata Block reference](docs/templates/metadata-reference.md),
[Packages](docs/templates/packages.md) (format, field types, typed patterns,
the TeX deny-list, the drift guard and `validate`) and
[Standard Fields](docs/templates/standard-fields.md); [Images in the pipeline](docs/internals/images.md);
[Choose a Template and Configuration](docs/configure/choose-a-template.md) for the
`binding:` block of a Collection admin; the [Worker pipeline](docs/internals/worker.md);
[Architecture](docs/internals/architecture.md); [Docker Compose](docs/deploy/docker-compose.md)
with the Worker security baseline; the [Injector and PDF action](docs/deploy/injector.md);
[Security](docs/internals/security.md) and [Testing](docs/testing.md) for the test tiers
and the CI schedule; [Limits and admission control](docs/deploy/limits-and-admission.md);
and, for the maintainer, the [Sanitisation gate](docs/sanitize.md) and the
[Publishing checklist](docs/publishing.md).

### Documentation site

The pages in `docs/` are also published as a site (`website/`, Docusaurus).

```sh
task docs:build              # strict build of the site (broken links fail)
task docs:catalogue          # regenerate the Template catalogue pages from the packages
task docs:catalogue-check    # fail if the generated catalogue pages are out of date
task docs:catalogue-preview  # re-render the catalogue preview images (needs Docker)
task docs:errors-check       # fail if the error catalogue and the source codes differ
```

## License

[MIT](LICENSE). The outprint image also contains Pandoc, TeX Live, librsvg and
fonts under their own licences; see [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES),
which includes the written source offer. Operators remain bound by Outline's
own licence for their Outline instance.
