---
sidebar_position: 7
---

# Configuration package

**Reference.** The files of a Template Configuration and the keys of its `configuration.yaml`.

A Configuration is one folder under the configurations root. The folder name
matches `^[a-z0-9][a-z0-9-]*$`. It adapts a Template to an organisation.

```
configurations/example/
  configuration.yaml
  logo.svg
```

<!-- validate: configuration template=neutral configuration=example -->
```yaml
manifest_version: 1

defaults:
  document_type: letter
  closing: Kind regards

fields:
  name: Example Garden Society
  logo: logo.svg
  accent_color: "2F6F5E"
```

Unknown keys are errors. Values are strings, booleans, dates or lists of
strings. When a Configuration is bound to a Template, every value of a declared
field is checked against that field: type, pattern or enum values, and for
assets the file path. The path rules are in
[Template checks](template-checks.md#path-confinement).

## `manifest_version`

**Type:** `integer`. **Required:** yes. **Default:** none.

Must be `1`.

## `defaults`

**Type:** mapping from field name to value. **Required:** no. **Default:** empty.

Values a Document may override. A default may name a field the Template does
not declare; it is ignored, so one Configuration serves several Templates. How
defaults combine with the Metadata Block is in
[Layering](metadata-block.md#layering).

### `defaults.document_type`

**Type:** `string`. **Required:** no. **Default:** none.

The Document Type used when a Document names none. It must be a Document Type
of the bound Template.

## `fields`

**Type:** mapping from field name to value. **Required:** no. **Default:** empty.

Fixed Values: a Document cannot change them. A Fixed Value the Template does
not declare has no effect and gives a warning. A default with the same name as
a Fixed Value is shadowed and also gives a warning.

<!-- validate: skip fragment of a single key, not a complete file -->
```yaml
fields:
  name: Example Garden Society
```

## `extends`

**Type:** none. **Required:** no. **Default:** none.

Reserved. Using it fails with "extends is not supported in v1".
