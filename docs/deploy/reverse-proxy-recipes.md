---
sidebar_label: Reverse-proxy recipes (coming soon)
sidebar_position: 14
---

# Reverse-proxy recipes

**Status: coming soon.** Recipes for nginx, Traefik and Caddy are planned.

Each recipe will show the edge proxy in front of the injector: TLS, compression,
and the forwarding headers that the injector expects. The shape is always the
same: `edge proxy -> injector -> Outline`, with `/pdf/*` going to the Gateway.

Today there is one worked example, for Traefik, in
[Injector and PDF action](injector.md#routing-example). The rules in
[How it works](injector.md#how-it-works) hold for every proxy.

Progress is tracked in [issue #51](https://github.com/taskmedia/outprint/issues/51).
