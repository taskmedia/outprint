---
sidebar_position: 8
---

# Service configuration

**Reference.** The file that names the package folders and the [Bindings](glossary.md#binding) a [Collection](glossary.md#collection) may choose.

The file is YAML. `OUTPRINT_SERVICE_CONFIG` names it; the shipped image sets it
to `/opt/outprint/service.yaml`. Unknown keys are errors. This page lists the keys that name packages and
Bindings. The file also takes `imageHosts` ([Image fetching](../internals/image-fetching.md))
and `admission` ([Limits and admission](../install/operate/limits-and-admission.md)).

<!-- validate: service-config -->
```yaml
templatesRoot: /opt/outprint/templates
configurationsRoot: /opt/outprint/configurations
bindings:
  - template: neutral
    configuration: example
defaultBinding:
  template: neutral
  configuration: example
```

## `templatesRoot`

**Type:** `string` (a directory). **Required:** yes, here or in `OUTPRINT_TEMPLATES_ROOT`. **Default:** none.

The folder with one Template package per subfolder. The environment variable
wins over the file. See [Template package](template-package.md).

## `configurationsRoot`

**Type:** `string` (a directory). **Required:** yes, here or in `OUTPRINT_CONFIGURATIONS_ROOT`. **Default:** none.

The folder with one Configuration package per subfolder. The environment
variable wins over the file. See [Configuration package](configuration-package.md).

## `bindings`

**Type:** list of `{template, configuration}`. **Required:** yes, at least one entry. **Default:** none.

The allow-list: the only pairs a Collection may choose with a
[Binding block](binding-block.md). A repeated pair, or an entry that names a
missing package, is an error. Packages on disk but not on the list are
loaded and checked, but their problems are only warnings, since nothing can
use them.

**Trust model.** The allow-list is global to the service, not per Collection
or group. Anyone who can edit a Collection description, including a user who
created the Collection, can bind any pair on the list, and with it that
Configuration's logo and Fixed Values. Fixed Values protect a Configuration
against Document authors, not against Collection editors. On an installation
shared by several organisations, list only pairs that every Collection
editor may use, or run one service per organisation. Per-Collection pair
restrictions are not implemented.

**Guests.** A user who can read a Document but not its Collection gets 403
for the Binding and sees no Formatted PDF action. This is intended: the
Binding is read with the user's own permission, so access to the Collection
is required.

## `defaultBinding`

**Type:** `{template, configuration}`. **Required:** no. **Default:** none.

The pair for a Collection without a block. It must be one of `bindings`.
Without it, such a Collection has no Formatted PDF action.
