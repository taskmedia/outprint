---
slug: /overview
---

# Documentation

## Using and running it

| Page | What it covers |
|------|----------------|
| [Metadata](metadata.md) | the Metadata Block a Document author writes: syntax, layering, field types, errors |
| [Binding](binding.md) | the block a Collection admin uses to choose a Template and Configuration |
| [Images](images.md) | images and logos: formats, sizes, captions, limits, errors |
| [Deploying with Docker Compose](deploy-compose.md) | running Gateway and Worker with Docker Compose |
| [Injector and download button](injector.md) | the reverse proxy that adds the Download PDF button |
| [Limits and admission control](limits.md) | limits and admission control |

## Writing Templates

| Page | What it covers |
|------|----------------|
| [Template author guide](template-author-guide.md) | start here: package format, fields, patterns, deny-list, `validate`, `render`, goldens, a worked example |
| [Packages](packages.md) | reference for packages, field types, typed patterns, TeX deny-list, drift guard |
| [Standard Fields](standard-fields.md) | the Standard Fields |

## How it works

| Page | What it covers |
|------|----------------|
| [Architecture](architecture.md) | Gateway and Worker roles, the job bundle, request flow, error mapping |
| [Worker pipeline](worker.md) | the Worker pipeline, filters, engine lockdown |
| [Security](security.md) | the security layers and what each proves |
| [Architecture decision records](adr/README.md) | architecture decision records |
| [Vocabulary](https://github.com/taskmedia/outprint/blob/main/CONTEXT.md) | the vocabulary |

## Developing and publishing

| Page | What it covers |
|------|----------------|
| [Development stack and end-to-end tests](development.md) | the seeded local stack with Outline |
| [Testing](https://github.com/taskmedia/outprint/blob/main/docs/testing.md) | test tiers and CI |
| [Sanitisation gate](https://github.com/taskmedia/outprint/blob/main/docs/sanitize.md) | the sanitisation gate and the pre-publish audit |
| [Publishing checklist](https://github.com/taskmedia/outprint/blob/main/docs/publishing.md) | the maintainer's checklist before the repository goes public |
| [Releasing](https://github.com/taskmedia/outprint/blob/main/docs/releasing.md) | the maintainer's release procedure: release PR, image publishing, one-time settings |
| [Research notes](https://github.com/taskmedia/outprint/blob/main/docs/research/README.md) | the research notes behind the design |
| [Third-party notices](https://github.com/taskmedia/outprint/blob/main/THIRD_PARTY_NOTICES) | third-party licences and the Pandoc source offer |
