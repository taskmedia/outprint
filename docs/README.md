# Documentation

## Using and running it

| Page | What it covers |
|------|----------------|
| [metadata.md](metadata.md) | the Metadata Block a Document author writes: syntax, layering, field types, errors |
| [binding.md](binding.md) | the block a Collection admin uses to choose a Template and Configuration |
| [images.md](images.md) | images and logos: formats, sizes, captions, limits, errors |
| [deploy-compose.md](deploy-compose.md) | running Gateway and Worker with Docker Compose |
| [injector.md](injector.md) | the reverse proxy that adds the Download PDF button |
| [limits.md](limits.md) | limits and admission control |

## Writing Templates

| Page | What it covers |
|------|----------------|
| [template-author-guide.md](template-author-guide.md) | start here: package format, fields, patterns, deny-list, `validate`, `render`, goldens, a worked example |
| [packages.md](packages.md) | reference for packages, field types, typed patterns, TeX deny-list, drift guard |
| [standard-fields.md](standard-fields.md) | the Standard Fields |

## How it works

| Page | What it covers |
|------|----------------|
| [architecture.md](architecture.md) | Gateway and Worker roles, the job bundle, request flow, error mapping |
| [worker.md](worker.md) | the Worker pipeline, filters, engine lockdown |
| [security.md](security.md) | the security layers and what each proves |
| [adr/](adr) | architecture decision records |
| [../CONTEXT.md](../CONTEXT.md) | the vocabulary |

## Developing and publishing

| Page | What it covers |
|------|----------------|
| [development.md](development.md) | the seeded local stack with Outline |
| [testing.md](testing.md) | test tiers and CI |
| [sanitize.md](sanitize.md) | the sanitisation gate and the pre-publish audit |
| [publishing.md](publishing.md) | the maintainer's checklist before the repository goes public |
| [research/](research/README.md) | the research notes behind the design |
| [../THIRD_PARTY_NOTICES](../THIRD_PARTY_NOTICES) | third-party licences and the Pandoc source offer |
