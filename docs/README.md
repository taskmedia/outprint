# Outprint documentation

Outprint turns Outline documents into typeset PDFs. Pick the section that fits
your job. The same files make up the [documentation site](https://outprint.task.media).

| Section | For | What you find |
|---------|-----|---------------|
| [Write](write/README.md) | people who write documents in Outline | get a PDF, fill in the details at the top of a document, add images, fix a message |
| [Install](install/README.md) | people who run Outline | three steps: run Outprint (Docker Compose or Helm), add the button (injector or edge-proxy recipe), bind a Template; then security, compatibility, limits, upgrading |
| [Configure](configure/README.md) | admins of a Collection | bind a Template to a Collection |
| [Templates](templates/README.md) | people who make Templates | the catalogue, the author guide and how to try a Template before you roll it out |
| [Reference](reference/README.md) | everyone | the Metadata Block, Binding block, package files, Standard Fields, error messages, limits, settings and the [Glossary](reference/glossary.md), key by key |

## Only on GitHub

These pages are for the maintainers and are not part of the site.

| Page | What it covers |
|------|----------------|
| [Testing](testing.md) | test tiers and CI |
| [Sanitisation gate](sanitize.md) | the sanitisation gate and the pre-publish audit |
| [Publishing checklist](publishing.md) | the checklist before the repository goes public |
| [Releasing](releasing.md) | the release procedure: release PR, image publishing, one-time settings |
| [Internals](internals/README.md) | architecture, the Worker, images, security, the local stack, decision records |
| [Decision records](adr/README.md) | why the design is the way it is |
| [Research notes](research/README.md) | the research behind the design |
| [Vocabulary](../CONTEXT.md) | the full vocabulary for contributors, with the words to avoid |
| [Third-party notices](../THIRD_PARTY_NOTICES) | third-party licences and the Pandoc source offer |
