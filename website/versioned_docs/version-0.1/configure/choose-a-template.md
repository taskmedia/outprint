---
sidebar_position: 2
---

# Choose a Template and Configuration

You give a Collection its PDF look by adding a short block to the Collection
description in Outline. The block names one Template and one Configuration. The
pair is called a **Binding**.

You need to be allowed to edit the Collection. You also need the names of an
installed Template and Configuration. Ask the person who runs Outprint, or look
at the [Template catalogue](../templates/catalogue/README.md). The first
Template, `neutral`, comes with a Configuration called `example`.

## Add the block

Open the Collection in Outline and edit its description. Add the block below
anywhere in the description. In the example the description is a title, one
sentence and the block. The part you add is the fenced `yaml` block.

<!-- validate: binding -->
````markdown
# Town Council

Notes and letters of the council.

```yaml
binding:
  template: neutral
  configuration: example
```
````

To make the block in Outline, type three backticks, choose **YAML** as the
language, and type the lines inside. Then save.

The change counts at once. Every document in the Collection, including documents
in folders below it, now offers **Formatted PDF** in its Export menu.

![The binding block in the description of a Collection in Outline](../assets/screens/collection-binding-block.png)

## The rules

- The block has exactly one key, `binding`. Below it, exactly two keys:
  `template` and `configuration`. Both hold a name.
- The names must match an installed Template and Configuration. Spell them
  exactly.
- The pair must be one that the person who runs Outprint allows. If you choose
  another pair, you get the error [`binding_not_allowed`](../write/when-something-goes-wrong.md#binding_not_allowed).
- The language of the block must be `yaml`, nothing else.
- Use one block per Collection. A second block is the error
  [`binding_duplicate`](../write/when-something-goes-wrong.md#binding_duplicate).
- Other `yaml` blocks in the description are ordinary text. Only the block with
  the key `binding` counts.
- Authors cannot change the Binding. Only people who can edit the Collection can.

## What happens without a block

1. The Collection has a block: Outprint uses it. A mistake in the block is an
   error, not a silent fallback.
2. The Collection has no block, and the installation has a default pair:
   Outprint uses the default pair.
3. Neither exists: the author gets the error
   [`no_binding`](../write/when-something-goes-wrong.md#no_binding).

A document in a Collection without a Binding and without a default shows no
**Formatted PDF**.

## If something is wrong

Mistakes in the block show up when an author clicks **Formatted PDF**. The action
stays visible on purpose, so that the mistake is seen. The error says
`collection` and gives the line, counted from the first line of the description.

| Code | What it means |
|------|---------------|
| [`binding_malformed`](../write/when-something-goes-wrong.md#binding_malformed) | The block is not closed, is not valid YAML, or has a missing, extra or repeated key |
| [`binding_unknown`](../write/when-something-goes-wrong.md#binding_unknown) | No installed Template or Configuration has that name |
| [`binding_not_allowed`](../write/when-something-goes-wrong.md#binding_not_allowed) | Both names exist, but this pair is not allowed |
| [`binding_duplicate`](../write/when-something-goes-wrong.md#binding_duplicate) | The description holds a second block |
| [`no_binding`](../write/when-something-goes-wrong.md#no_binding) | No block, and no default pair |

All problems are reported at once. A person who may not read the Collection
gets a message that names the Collection.

## Try it

1. Add the block to a test Collection.
2. Open a document in it. **Export → Formatted PDF** appears in the **…** menu.
3. Click it. Open the PDF. The letterhead shows the name and logo of the
   Configuration.

If a document starts with a block of its own at the top, the author can add
details such as a recipient. See [Fill in the document](../write/fill-in-the-document.md).
