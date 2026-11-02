---
sidebar_position: 1
---

# Deploy

This section is for the person who runs Outline and Outprint. It shows how to
install Outprint next to an Outline you host.

Outline has no plug-in point, and Outprint does not change Outline. Outprint
runs next to it and talks to Outline through its API, with the cookie of the
signed-in user. There is no webhook and no API token. What differs between
installs is **how the user starts a PDF**. That gives three ways.

## Which way fits you?

Ask: how do you want to use Outprint?

| I want to... | Way | What you run |
|--------------|-----|--------------|
| click **Export → Formatted PDF** inside Outline | [Export menu in Outline](ways/export-menu.md) | Gateway, Worker and injector; the injector sits in front of Outline |
| download a PDF from a link, and leave the way Outline is served as it is | [PDF by link](ways/pdf-by-link.md) | Gateway and Worker; `/pdf` on the address of Outline goes to the Gateway |
| click **Export → Formatted PDF** and already run an nginx, Caddy or Traefik that can rewrite HTML | [Export menu](ways/export-menu.md) with [Reverse-proxy recipes](components/reverse-proxy-recipes.md) | Gateway and Worker; your edge proxy adds the script tag, no injector |
| make PDFs from Markdown files, without Outline | [Command line](ways/command-line.md) | the `outprint` image only |

If you are unsure, take the first way. It is the one the browser tests cover.

```mermaid
flowchart TB
    user([User in the browser])
    edge[Edge proxy<br/>TLS, host name]
    inj[Injector]
    outline[(Outline)]
    gw[Gateway]
    wk[Worker<br/>no network]
    cli[outprint CLI]
    md[/Markdown file/]
    pdf[/PDF file/]

    user --> edge
    edge -->|Export menu: all traffic| inj
    edge -.->|PDF by link: /pdf only| gw
    inj -->|everything else| outline
    inj -->|/pdf/*| gw
    gw -->|user's cookie| outline
    gw --> wk
    md --> cli --> pdf
```

Solid arrows are the Export menu way. The dotted arrow is the PDF by link way,
which skips the injector. The [recipes](components/reverse-proxy-recipes.md)
replace the injector box by a rewrite in the edge proxy. The command line stands alone.

## What runs

| Part | Image | Needed by | What it does |
|------|-------|-----------|--------------|
| `gateway` | `outprint` | menu, link | Reads the document from Outline as the user, checks it, and asks the Worker for the PDF. |
| `worker` | `outprint` | menu, link | Typesets the PDF with Pandoc and LuaLaTeX. No network. |
| `injector` | `outprint-injector` | menu | Reverse proxy in front of Outline. Adds the script that draws the action. Routes `/pdf/*` to the Gateway. **Optional** for the link way, and replaceable by a [recipe](components/reverse-proxy-recipes.md) for the menu. |
| `outprint` CLI | `outprint` | command line | `render` and `validate` run the same pipeline without Outline. |

## Combine the ways

The ways share parts, so they combine:

- **Link and menu** are one install. The menu way is the link way plus the
  injector. A link such as `/pdf/render/<id>` keeps working next to the menu.
- **The command line** can run next to either. It is also the quick check of a
  Template before you roll it out ([Command line](ways/command-line.md)).

A sensible order:

1. Read the [common base](common-base.md) and bring up Gateway and Worker.
2. Check that the link works ([PDF by link](ways/pdf-by-link.md)). This proves the
   Gateway reaches Outline and the Worker renders.
3. Add the injector for the menu ([Export menu in Outline](ways/export-menu.md)). It is
   the only step that touches how Outline is served.

Without the injector everything except the action in Outline's interface keeps
working.

## What Outprint does not do

- **No automatic PDFs on change.** Outprint reacts to a request from a user. It
  does not listen to Outline webhooks.
- **No API token.** The Gateway uses the cookie of the user, so Outline decides
  what the user may read. Nothing in Outprint holds a token for Outline.
- **No hosted Outline.** You need control over how Outline is served for the
  menu, and over the routes in front of it for the link.
- **Kubernetes through the Helm chart.** The chart lives in
  [taskmedia/helm](https://github.com/taskmedia/helm/tree/main/charts/outprint);
  see [Helm and Kubernetes](platforms/helm-kubernetes.md). There are no plain manifests.

## Where to go next

| Group | Pages |
|-------|-------|
| Start here | [Common base](common-base.md): what every way needs, images, Outline access, settings. |
| [Install ways](ways/README.md) | [Export menu in Outline](ways/export-menu.md), [PDF by link](ways/pdf-by-link.md), [Command line](ways/command-line.md). |
| Platforms | [Docker Compose](platforms/docker-compose.md), the shipped Compose file and the Worker baseline. [Helm and Kubernetes](platforms/helm-kubernetes.md), the chart from taskmedia/helm. Planned: [Podman](platforms/podman.md). |
| Components | [Injector and PDF action](components/injector.md), [Image fetching](components/image-fetching.md), [Limits and admission control](components/limits-and-admission.md). [Reverse-proxy recipes](components/reverse-proxy-recipes.md): nginx, Caddy and Traefik instead of the injector. |
| Operate | [Security notes for operators](operate/security-for-operators.md), [Upgrading](operate/upgrading.md), [Compatibility with Outline](operate/compatibility.md). |

After the install, a Collection admin chooses a Template for each Collection.
See [Configure](../configure/README.md).
