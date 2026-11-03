---
sidebar_position: 1
---

# Internals

This section is for people who want to know how Outprint works: security
reviewers, contributors and the curious. You do not need it to use or run
Outprint.

| Page | What it covers |
|------|----------------|
| [Architecture](architecture.md) | Gateway and Worker roles, the job bundle, the request flow, error mapping |
| [Worker pipeline](worker.md) | The phases of a render, the filters, the engine lockdown |
| [Images in the pipeline](images.md) | How images and SVG files are checked, converted and placed |
| [The injector and the PDF action](injector.md) | How the injector rewrites pages, and what the script does in Outline |
| [The script tag](script-tag.md) | The tag's contract and its options |
| [Edge-proxy recipes](edge-proxy-recipes.md) | The rules and caveats behind the nginx, Caddy and Traefik recipes |
| [Image fetching](image-fetching.md) | How the Gateway fetches images and which hosts it trusts |
| [The Worker baseline](worker-baseline.md) | The Worker's lock-down in Compose and Helm |
| [Security](security.md) | The security layers, what each one proves, the limits of the guarantees |
| [Development stack and end-to-end tests](development.md) | The seeded local stack with Outline, and the browser tests |
| [Architecture decision records](../adr/README.md) | Why the system is built the way it is |

## Vocabulary

The words **Template**, **Configuration**, **Collection**, **Binding**,
**Document Type** and the rest have a fixed meaning. The vocabulary lives in
`CONTEXT.md` at the root of the repository, so that the code and the docs share
one source. It is not copied into this site. Read
[the vocabulary on GitHub](https://github.com/taskmedia/outprint/blob/main/CONTEXT.md).

## Repo-only pages

Some pages are for the maintainers and are not published on this site: the
test tiers, the sanitisation gate, the publishing checklist, the release
procedure and the research notes. They are in the
[docs folder on GitHub](https://github.com/taskmedia/outprint/tree/main/docs).
Third-party licences are in the
[third-party notices](https://github.com/taskmedia/outprint/blob/main/THIRD_PARTY_NOTICES).
