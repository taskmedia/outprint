---
sidebar_position: 1
---

# Deploy

This section is for the person who runs Outline and Outprint. It shows how to
install Outprint next to an Outline you host.

## Prerequisites

Check these before you start. If one is missing, Outprint cannot work.

1. **Outline must be self-hosted.** You need control over the web server in
   front of Outline. Hosted Outline (the cloud service of the Outline company)
   cannot be used, because you cannot put anything in front of it.
2. **The injector is a required step, not an option.** Outline has no plug-in
   point for a toolbar button, and Outprint does not change Outline. The
   injector is a small reverse proxy that adds one `<script>` tag to Outline's
   pages. That script shows the **Download PDF** button. Without the injector
   there is no button. The reasons are in the
   [research notes on Outline](https://github.com/taskmedia/outprint/blob/main/docs/research/outline-surface.md).
3. **The injector sits between your edge proxy and Outline.** The edge proxy
   keeps TLS and your host name. Everything goes to the injector. The injector
   forwards to Outline, and sends `/pdf/*` to the Outprint Gateway.
4. **Docker and Docker Compose** run the services. Other ways are
   [planned](helm-kubernetes.md).

```text
browser --> edge proxy (TLS) --> injector --> Outline
                                     |
                                     +-- /pdf/*  --> Gateway --> Worker
```

Users sign in to Outline as usual. Their session cookie travels with the
request to `/pdf/*`, so the Gateway acts for that user and nobody else.
Because everything is on one address, there is no cross-origin traffic and no
second login.

## What runs

| Part | Image | What it does |
|------|-------|--------------|
| `injector` | `outprint-injector` | Reverse proxy in front of Outline. Adds the script tag. Routes `/pdf/*` to the Gateway. |
| `gateway` | `outprint` | Reads the document from Outline as the user, checks it, and asks the Worker for the PDF. |
| `worker` | `outprint` | Typesets the PDF with Pandoc and LuaLaTeX. No network. |

## Where to go next

| Page | What it covers |
|------|----------------|
| [Docker Compose](docker-compose.md) | Run all three services with the shipped Compose file. |
| [Injector and download button](injector.md) | Put the injector in front of Outline, and the settings `OUTPRINT_INJECTOR_*`. |
| [Image fetching](image-fetching.md) | Which hosts the Gateway may fetch images from. |
| [Limits and admission control](limits-and-admission.md) | How large a document can be, and how many PDFs run at once. |
| [Security notes for operators](security-for-operators.md) | What to expose, what to keep inside, and what to check. |
| [Upgrading](upgrading.md) | What to check between versions. |

Planned, not available yet:

| Page | Status |
|------|--------|
| [Helm and Kubernetes](helm-kubernetes.md) | Coming soon. |
| [Reverse-proxy recipes](reverse-proxy-recipes.md) | Coming soon. |
| [Podman](podman.md) | Coming soon. |

After the install, a Collection admin chooses a Template for each Collection.
See [Configure](../configure/README.md).
