<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/svg/outprint-banner-dark.svg">
    <img alt="Outprint: PDFs from Outline documents" src="docs/assets/logo/svg/outprint-banner.svg" width="440">
  </picture>
  <br><br>
</p>

Turn [Outline](https://www.getoutline.com/) documents into typeset PDFs
(letters, reports, your own LaTeX design) with a **Formatted PDF** action in the
Export menu. Self-hosted, and Outline is not forked.

A [Template](docs/reference/glossary.md#template) provides the look. A
[Configuration](docs/reference/glossary.md#configuration) adds the name, logo and
colours of an organisation. A [Collection](docs/reference/glossary.md#collection)
admin binds the two to a Collection, and authors fill in a few fields at the top
of a document and download the PDF.

![A document in Outline with the Export menu open and Formatted PDF in it](docs/assets/screens/bound-document-with-button.png)

## Install in three steps

1. **[Run Outprint](docs/install/run-outprint.mdx)** with Docker Compose or Helm.
2. **[Add the button](docs/install/add-the-button.mdx)** to Outline, with the
   injector or an edge-proxy recipe (nginx, Caddy, Traefik).
3. **[Bind a Template to a Collection](docs/configure/README.md)** so that its
   documents get the PDF look.

The [install overview](docs/install/README.md) explains how the parts fit
together.

## Documentation

The docs are at <https://outprint.task.media> and in [docs/](docs/README.md):
Write, Install, Configure, Templates and Reference. Template authors start with
the [Template author guide](docs/templates/template-author-guide.md). To work on
Outprint itself, see [Development](docs/internals/development.md); the
[vocabulary](CONTEXT.md), the
[architecture decision records](docs/adr/README.md) and the
[research notes](docs/research/README.md) stay on GitHub.

## Status

Work in progress. Tracked in the project issues.

## License

[MIT](LICENSE). The outprint image also contains Pandoc, TeX Live, librsvg and
fonts under their own licences; see [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES),
which includes the written source offer. Operators remain bound by Outline's
own licence for their Outline instance.
