---
sidebar_position: 10
---

# Compatibility with Outline

The injector places the PDF action by reading Outline's page, because Outline
has no extension point for its menus (see
[Injector and PDF action](injector.md#the-pdf-action)). A new Outline release can
change that markup. This page says which versions have been checked.

| Outline | Outprint | Status |
|---------|----------|--------|
| 1.12.0 | development (no release yet) | **tested** on every change to the injector, the script and the browser tests; this is the version pinned in `deploy/dev/versions.env` |
| newest stable release | development | **tested weekly**, not blocking; a failure opens an issue |
| older than 1.12.0 | | **untested** |

"Tested" means the browser tests pass: the **Export → Formatted PDF** entry and
the header button appear for a bound document, a click downloads a PDF, and the
entry is not duplicated by opening the menu again.

Two things were not tested yet: a user with a read-only role (the dev stack
seeds only an administrator), and the **PDF** entry that Outline adds itself
when `PDF_EXPORT_ENABLED` is set.

If a new Outline version breaks the placement, the floating button appears and
the browser console says why. Nothing else stops working: the Gateway and the
PDF are not affected.

## Try another Outline version

The browser tests run against the dev stack. `OUTLINE_IMAGE` in the shell wins
over the pin file:

```sh
OUTLINE_IMAGE=outlinewiki/outline:1.12.0 task dev:down-volumes dev e2e:run
```

See [Development stack and end-to-end tests](../internals/development.md).
