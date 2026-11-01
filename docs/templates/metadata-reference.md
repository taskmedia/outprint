---
sidebar_position: 5
---

# Metadata Block reference

This page is the reference for Template authors and operators. Document authors
find a plain-language version in [Fill in the document](../write/fill-in-the-document.md).

A Document author sets field values, and picks the Document Type, in a
**Metadata Block** at the very start of the Document. The block is removed
from the rendered body. Every problem found is reported at once, with the line
it is on.

## The block

<!-- validate: metadata template=neutral configuration=example -->
````markdown
```yaml
type: letter
subject: Spring plant sale
reference: PS-2026-03
recipient:
  - Ada Example
  - 1 Sample Lane
  - 12345 Exampleton
closing: With best wishes
opening:                      # empty: leave the salutation out
```
Thank you for your enquiry ...
````

- The block is the first thing in the Document (blank lines before it are
  fine), a fenced code block whose info string is exactly `yaml`. Outline keeps
  only the first word of an info string, so the block is told apart by its
  place, not by a second word.
- The content is YAML: a mapping from field names to values. Duplicate keys,
  aliases and anything that is not a mapping are errors.
- Only the topmost block counts. A later `yaml` block, or one inside another
  fenced block, is ordinary text in the body, so a Document can still show YAML.
- A Document that does not start with a `yaml` block has no Metadata Block. That
  is valid when every required field has a default.
- A block spelled with the old info string `yaml metadata` is not recognised;
  it is body text.

## Document Type

`type` is the only reserved key. The Document Type comes from the Document's
`type`, else from `document_type` in the Configuration Defaults. If neither is
set, or the name is not a Document Type of the Template, that is an error. The
type decides which fields exist: `neutral` offers `letter` and `report`.
When there is no usable type, the other keys are still checked against all
fields the Template declares, so typos are reported in the same pass.

## Layering

For every field of the Document Type the highest layer that has a value wins:

| Layer (highest first)              | Where it comes from                                   |
| ---------------------------------- | ----------------------------------------------------- |
| Fixed Values                       | Configuration `fields:`; a Document cannot change them |
| Document                           | the Metadata Block                                    |
| Configuration Defaults             | Configuration `defaults:`                             |
| Derived value                      | `title`, `subject` and `date`, see below              |
| Manifest default                   | the Template's `fields:` and Document Type overrides  |

- An **empty** Document value suppresses every lower layer: `closing:`,
  `closing: ""` and `closing: []` all remove a Default. If the field is
  required, that is an error.
- Configuration Defaults may carry fields the Template does not declare; they
  are ignored. They are not an error, so one Configuration serves several
  Templates.
- A key that names a Fixed Value (for example `name` or `logo`) is rejected in
  a Document.

### Derived values

The pipeline supplies the document title and the last-modified date, from the
Outline document in the Gateway and from `--title` and `--date` in the `render` CLI:

- `title`: the document title, the default of the field `title`.
- `subject`: the document title too, the default of the field `subject`. A
  Document that sets `subject: "Some text"` prints that text. An empty
  `subject:` or `subject: ""` opts out, and the letter has no subject line.
- `date`: the last-modified date as ISO `YYYY-MM-DD`, the default of the field
  `date`. The Template writes it out by `language` (`en`: `2 March 2026`,
  `de`: `2. März 2026`). Text that is not an ISO date, for example
  `date: 14 March 2026` in the block, is printed as written.

`language` is an ordinary field: the Template's default, which a Configuration
may override (and a Document, where the Document Type lists it). It never comes
from the reader's locale.

## Types

Strict by design: a value that does not fit is an error, never silently
converted.

| Type     | Accepts                                                                     |
| -------- | --------------------------------------------------------------------------- |
| `string` | one line of plain text                                                      |
| `text`   | multi-line text with inline Markdown (emphasis, links, lists)               |
| `list`   | a YAML list, or a multi-line string (one item per line, blank lines dropped) |
| `date`   | an ISO date, or free text                                                   |
| `bool`   | `true` or `false`                                                           |
| `enum`   | one of the Template's values                                                |
| `color`, `asset` | as the Template's pattern allows                                    |

- Only scalars become text: `subject: 2026` is the text `2026`, but a list or a
  mapping where text is expected is a `type` error.
- `string`, `list` and the other non-`text` types are plain text: `*stars*`,
  `$x$` and `\input{foo}` are printed as written. Only `text` fields (for
  example `abstract`) may carry Markdown.
- Control characters are rejected.

Values reach TeX only as Pandoc metadata (a metadata file), never as `-V`
variables or by building strings.

## Errors

The result is either the body and the merged values, or the whole list of
problems:

```json
{"errors":[
  {"field":"recipent","source":"document","code":"unknown","line":4,
   "message":"unknown key \"recipent\": not a field of document type \"letter\" (did you mean \"recipient\"?)"},
  {"field":"side","source":"document","code":"enum","line":5,
   "message":"\"middle\" is not one of left, right"}
]}
```

| Key       | Meaning                                                                         |
| --------- | ------------------------------------------------------------------------------- |
| `field`   | the field name, empty when the problem is not about one field                   |
| `source`  | who fixes it: `document`, `configuration` or `template` (later stages add `collection`) |
| `code`    | `required`, `unknown`, `type`, `enum`, and `block` for a broken block           |
| `message` | readable text                                                                   |
| `line`    | the line in the Document, `null` if the problem has no position                |

`block` covers the structure of the block itself: a missing closing fence, invalid YAML, duplicate keys. Clients must tolerate
codes they do not know; later stages add their own (for example image
problems). Pattern mismatches are `type` errors.

Misspelled keys get a hint when a field name is within two edits. Only the
winning layer of a field is checked, so a faulty Default that the Document
overrides is not reported.

## Rendering from the command line

```sh
outprint render --template neutral --config example \
  --title "Spring plant sale" --date 2026-03-02 -o out.pdf letter.md
```

- `--title` and `--date` stand in for the Outline title and last-modified date.
  Without `--date`, the input file's modification date (UTC) is used.
- Problems are printed one per line on stderr and the exit code is 1:

  ```
  outprint: letter.md has 2 problem(s):
    document line 4: recipent: unknown key "recipent": ... (unknown)
    document line 5: side: "middle" is not one of left, right (enum)
  ```

- `--json-errors` prints the JSON error body on stdout instead.
