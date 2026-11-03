---
sidebar_position: 3
---

# Standard Fields

**Reference.** The Standard Fields: common field names and their types.

Standard Fields are a documented set of common field names. Templates are
encouraged to reuse them, with the type listed here, so that Template
Configurations stay portable across Templates. They are a convention, not a
requirement: a Template may declare any fields it likes.

`outprint validate` warns when a Template declares a Standard Field name with
a different type. The warning never fails validation.

The machine-readable table is
[`internal/packages/standard_fields.yaml`](https://github.com/taskmedia/outprint/blob/main/internal/packages/standard_fields.yaml);
a test keeps it and this page in step.

## Any Document Type

| Field        | Type   |
| ------------ | ------ |
| `title`      | string |
| `date`       | date   |
| `language`   | string |
| `author`     | string |

## Configuration fields

These are normally supplied by a Configuration (as Fixed Values or
Defaults) and therefore declared by every Template that uses them.

| Field          | Type   | Meaning                              |
| -------------- | ------ | ------------------------------------ |
| `name`         | string | Organisation or sender name          |
| `logo`         | asset  | Logo file inside the Configuration   |
| `accent_color` | color  | Accent colour                        |
| `font`         | string | Optional font override               |

## Letter

Applies to fields of a Document Type named `letter`.

| Field        | Type                 |
| ------------ | -------------------- |
| `recipient`  | list or text         |
| `sender`     | string, text or list |
| `subject`    | string               |
| `opening`    | string               |
| `closing`    | string               |
| `reference`  | string               |
| `enclosures` | list                 |
| `cc`         | list                 |

## Report

Applies to fields of a Document Type named `report`.

| Field      | Type   |
| ---------- | ------ |
| `subtitle` | string |
| `version`  | string |
| `abstract` | text   |
| `toc`      | bool   |
