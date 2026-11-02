---
sidebar_position: 4
---

# Start new documents with the block filled in

Typing the [metadata block](fill-in-the-document.md) by hand is tedious when
every document in a Collection starts the same way. Outline's own **templates**
solve this. A template that starts with the block gives every new document the
block already filled in. Outprint needs no extra setup for this.

This page is for the person who sets up the template, usually a Collection
admin, and for the authors who use it.

## Set up the template

1. Open Outline's **Settings** and choose **Templates**, then **New
   template…**.
2. Give the template a name. Make it a template of your Collection, so that it
   is offered only there.
3. Start the template with a **YAML** code block, as in
   [Fill in the document](fill-in-the-document.md#how-to-make-the-block-in-outline).
   Put the fields that are the same for most documents, and a placeholder for
   the rest. Copy a block from the
   [Template catalogue](../templates/catalogue/README.md) to begin.
4. Write the rest of the template below the block, for example a first line or a
   heading structure.

<!-- validate: metadata template=neutral configuration=example -->
````markdown
```yaml
type: letter
author: Robin Sample
recipient:
  - Name of the recipient
  - Street
  - Postcode and town
opening: Dear Ms Example,
```
````

A template belongs to one Collection or to the whole workspace. By default only
admins can create and change templates.

Instead of a fixed name after `author:`, you can write `{author}`. Outline
replaces it with the name of the person who makes the document.

Fields that every document shares, such as the logo, the colours or a closing,
do not belong in the template. The Collection's Configuration already supplies
them as [defaults](fill-in-the-document.md#defaults-and-what-you-cannot-change).

## Use the template

An author opens the Collection and creates a new document with **New doc**. The
new document is empty. The **Templates** menu at the top lists the templates of
the Collection. Choosing one fills the document with the template.

- The block arrives at the very start of the document, so Outprint reads it as
  usual. If the PDF details are folded, the block sits behind the **PDF
  details** line like any other block (see
  [Where did my block go?](fill-in-the-document.md#where-did-my-block-go)).
- The document takes the **title of the template**. Change the title: a letter
  without a `subject` uses the title as its subject.
- Replace the placeholders in the block with real values, then click
  **Formatted PDF** as usual.

## Good to know

- Choose a template in a **new, empty** document. Choosing it in a document that
  already has a block adds a second block. Outprint uses only the first block;
  the second one is printed as text.
- A template is copied when the document is made. Changing the template later
  does not change documents that already exist.
- The block is checked when someone clicks **Formatted PDF**, not when the
  template is saved. Test your template once: make a document from it and
  download the PDF. A mistake in the template then shows up once, not in every
  document.
- A workspace-wide template is offered in every Collection, including those
  without a PDF look, so give a template that carries a block to its own
  Collection.
