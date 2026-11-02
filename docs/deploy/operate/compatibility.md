---
sidebar_position: 3
---

# Compatibility with Outline

The injector places the PDF action by reading Outline's page, because Outline
has no extension point for its menus (see
[Injector and PDF action](../components/injector.md#the-pdf-action)). A new Outline release can
change that markup. This page says which versions have been checked.

| Outline | Outprint | Status |
|---------|----------|--------|
| 1.12.0 | development (no release yet) | **tested** on every change to the injector, the script and the browser tests; this is the version pinned in `deploy/dev/versions.env` |
| newest stable release | development | **tested weekly**, not blocking; a failure opens an issue |
| 1.12.0 (`OUTLINE_OLDEST_IMAGE`) | development | the **oldest supported** version; the same tests run against it in CI once it differs from the pin |
| older than 1.12.0 | | **untested** |

The script is the same when an edge proxy adds the tag instead of the injector
([Reverse-proxy recipes](../components/reverse-proxy-recipes.md)). The browser
tests run against the **nginx** recipe too (`task e2e:run-nginx`, Outline 1.12.0).
The Caddy and Traefik recipes are **untested**. The
[mounted page template](../components/mounted-template.md) is tested against
Outline 1.12.0 too (`task e2e:run-template`); unlike the other ways it depends on
the path and format of one file inside the Outline image, so it is the first to
need a check after an Outline upgrade.

| Way to load the action | Covered by |
|------------------------|------------|
| Injector | `task e2e:run`, the full browser suite |
| Edge-proxy recipe, nginx | `task e2e:run-nginx` |
| Edge-proxy recipe, Caddy and Traefik | not tested |
| Mounted page template | `task e2e:run-template` |
| Bare tag with options from the Gateway | `web/button.test.js` and the Gateway's status tests (`go test ./internal/gateway`) |
| `data-` attributes of the injector's tag | `go test ./internal/inject` and `web/button.test.js` |

"Tested" means the browser tests pass: the **Export → Formatted PDF** entry and
the header button appear for a bound document, a click downloads a PDF, and the
entry is not duplicated by opening the menu again.

The browser tests also sign in as a second user with the viewer role, who cannot
edit: the menu entry works for that user, and the header button sits before the
**…** button because Outline shows no **Share**.

Two more cases are tested against Outline 1.12.0 (`e2e/tests/compatibility.spec.ts`):

- **Outline's own PDF entry.** The community image of Outline 1.12.0 ignores the
  `PDF_EXPORT_ENABLED` environment variable: the server does not read it, does
  not pass it to the page, and answers a PDF export request with "not available
  in the community edition". The browser shows Outline's **PDF** entry only
  when the page sets `window.env.PDF_EXPORT_ENABLED`, so the test sets it there.
  Result: both entries (**PDF** and **Formatted PDF**) are present and told apart
  by their labels, ours is added once however often the menu is opened, it still
  downloads the formatted PDF, the header button is unaffected, and clicking
  Outline's entry is left to Outline. Whether Outline's entry works end to end
  was not tested; it cannot in the community edition.
- **A German Outline** (user language `de_DE`, set through `users.update`). The
  **…** button, **Exportieren** submenu and **Teilen** button are found, the
  entry is labelled **Formatiertes PDF**, the entry, the header button and the
  floating fallback work as in English. Other languages were not tested.

If a new Outline version breaks the placement, the floating button appears and
the browser console says why. Nothing else stops working: the Gateway and the
PDF are not affected.

## Current CI status

The table above is a statement about releases and does not change by itself.
These badges show what the browser tests do **today on the `main` branch**, so a
run that has just failed is visible here. They do not describe the version of the
docs you are reading.

| Run | Covers | Status |
|-----|--------|--------|
| [`e2e`](https://github.com/taskmedia/outprint/actions/workflows/e2e.yml) | the pinned and the oldest supported Outline | [![e2e](https://github.com/taskmedia/outprint/actions/workflows/e2e.yml/badge.svg?branch=main)](https://github.com/taskmedia/outprint/actions/workflows/e2e.yml) |
| [`e2e-latest-outline`](https://github.com/taskmedia/outprint/actions/workflows/e2e-latest-outline.yml) | the newest stable Outline, weekly | [![e2e-latest-outline](https://github.com/taskmedia/outprint/actions/workflows/e2e-latest-outline.yml/badge.svg?branch=main)](https://github.com/taskmedia/outprint/actions/workflows/e2e-latest-outline.yml) |

The badges come from GitHub itself and need no other service. The weekly run does
not block changes, so a red badge there means "look at the newest Outline", not
"this release is broken".

## Try another Outline version

The browser tests run against the dev stack. `OUTLINE_IMAGE` in the shell wins
over the pin file:

```sh
OUTLINE_IMAGE=outlinewiki/outline:1.12.0 task dev:down-volumes dev e2e:run
```

See [Development stack and end-to-end tests](../../internals/development.md).
