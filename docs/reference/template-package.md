---
sidebar_position: 5
---

# Template package

**Reference.** The files of a Template and the keys of its `manifest.yaml`.

A [Template](glossary.md#template) is one folder under the templates root. The folder name matches
`^[a-z0-9][a-z0-9-]*$`. A package is self-contained: nothing is shared between
packages. To write one, start with the
[Template author guide](../templates/template-author-guide.md).

```
templates/neutral/
  manifest.yaml        fields and Document Types
  letter.latex         one Pandoc template per Document Type
  shared/neutral.tex   shared TeX parts (inside the package only)
  fonts/               optional; copied to ./fonts/ of every job
  assets/              optional
```

<!-- validate: skip reduced manifest fragment, not a complete Template package -->
```yaml
manifest_version: 1

fields:
  accent_color:
    type: color
    description: Colour of the margin stripe.
    pattern: '^[0-9A-Fa-f]{6}$'
  subject:
    type: string
    description: Subject line.

document_types:
  letter:
    template: letter.latex
    fields:
      - accent_color
      - subject:
          required: true
```

Unknown keys anywhere are errors. The checks that run on the whole package
are in [Template checks](template-checks.md).

## `manifest_version`

**Type:** `integer`. **Required:** yes. **Default:** none.

Must be `1`. Any other value is rejected before the rest of the file is read.

## `fields`

**Type:** mapping from field name to definition. **Required:** no. **Default:** none.

Declares each field once. Field names match `^[a-z][a-z0-9_]*$`. Reuse the
names in [Standard Fields](standard-fields.md) where they fit.

### `fields.<name>.type`

**Type:** `string`. **Required:** yes. **Default:** none.

One of the types below.

| Type     | Value                                              | Position in TeX |
|----------|----------------------------------------------------|-----------------|
| `string` | one line of text                                   | text            |
| `text`   | multi-line text                                    | text            |
| `list`   | list of strings                                    | text            |
| `date`   | a date (free text or a YAML date)                  | text            |
| `bool`   | `true` or `false`                                  | condition       |
| `enum`   | one of `values`                                    | non-text        |
| `color`  | a colour, checked by `pattern`                     | non-text        |
| `asset`  | a package-relative file path, checked by `pattern` | non-text        |

### `fields.<name>.required`

**Type:** `bool`. **Required:** no. **Default:** `false`.

The Document must supply a value (or a Default must).

### `fields.<name>.description`

**Type:** `string`. **Required:** no. **Default:** empty.

Free text. The generated [catalogue](../templates/catalogue/README.md) shows it.

### `fields.<name>.default`

**Type:** the field's type. **Required:** no. **Default:** none.

The value used when nothing else supplies one. It must pass the field's own
type and pattern.

### `fields.<name>.pattern`

**Type:** `string` (an RE2 expression). **Required:** for `color` and `asset`, and for any field in a non-text position. **Default:** none.

Matched against the whole value. See [Typed patterns](template-checks.md#typed-patterns).

<!-- validate: skip fragment of a single key, not a complete file -->
```yaml
pattern: '^[0-9A-Fa-f]{6}$'
```

### `fields.<name>.values`

**Type:** list of `string`. **Required:** for `enum`, and only allowed there. **Default:** none.

The allowed values, each made of letters, digits, space, `.`, `_` and `-`.

<!-- validate: skip fragment of a single key, not a complete file -->
```yaml
type: enum
values: [left, right]
```

## `document_types`

**Type:** mapping from Document Type name to definition. **Required:** yes, at least one entry. **Default:** none.

A Template offers one or more Document Types, such as `letter` or `report`.

### `document_types.<name>.template`

**Type:** `string` (a path). **Required:** yes. **Default:** none.

The Pandoc template file of the Document Type, relative to the package. It uses
the Pandoc variable syntax (`$name$`, `$if(name)$`, `$for(name)$`). Partials
and applied functions are not supported, and a variable may never build a TeX
control sequence name (`\foo$name$` is an error).

### `document_types.<name>.fields`

**Type:** list. **Required:** no. **Default:** empty.

The fields the Document Type uses. An entry is a name, or a mapping from the
name to `required` and/or `default`, which override the declaration for this
Document Type only.

### `document_types.<name>.description`

**Type:** `string`. **Required:** no. **Default:** empty.

Free text.

## Fonts

Fonts are loaded by file path, never by name: the TeX engine of the Worker
cannot use the system font database. A Template loads its fonts from the
package with the fontspec option `Path=./fonts/`:

```tex
\setmainfont{texgyrepagella-regular.otf}[Path=./fonts/,
  BoldFont=texgyrepagella-bold.otf, ItalicFont=texgyrepagella-italic.otf,
  BoldItalicFont=texgyrepagella-bolditalic.otf]
```
