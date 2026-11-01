# Binding

A **Binding** pairs one Template with one Template Configuration. A
Collection admin chooses it for every Document in the Collection, nested
Documents included, with a fenced `yaml` block holding a `binding:` key in the
Collection overview. Authors cannot choose or override it, and neither can a request.

## The block

````markdown
# Town Council

Notes and letters of the council.

```yaml
binding:
  template: neutral
  configuration: example
```
````

- The block is a fenced code block (backticks or tildes, three or more,
  indented at most three spaces) whose info string is exactly `yaml`. Outline
  keeps only the first word of an info string, so the block is told apart from
  other YAML by its root key `binding:`, not by a second word.
- It may be anywhere in the overview. The first block with a `binding:` key
  counts; a second one is a `binding_duplicate` error. Other `yaml` blocks are
  ordinary text.
- The content is YAML with exactly one key, `binding`, holding a mapping with
  exactly the keys `template` and `configuration`, each the name of an
  installed package, as a string.
- A `binding:` inside another fenced block, in inline code, in an indented code
  block or nested below another key is not a block.
- A block spelled with the old info string `yaml binding` is not recognised.
- The pair must be on the installation's allow-list (the `bindings` of the
  service configuration).

## Which Binding applies

1. The overview has a block: that block. Any problem with it is an error.
2. The overview has no block at all: the installation's default Binding.
3. No block and no default Binding: an error (`no_binding`).

There is no silent fallback. A broken block never falls back to the default,
and a Collection that cannot be read is an error, not a default render.
The overview is read on every render, so an edit takes effect on the next
click. Only people who can edit the Collection can change it.

## Errors

Binding errors are part of the common error body (status 422), with
`source: "collection"` and `line` counting from the first line of the
overview:

```json
{"errors": [
  {"field": "font", "source": "collection", "code": "binding_malformed",
   "message": "unknown key \"font\": the binding block has exactly the keys template and configuration",
   "line": 8}
]}
```

| Code | Meaning |
|------|---------|
| `binding_malformed` | block not closed, broken YAML, `binding` is not a mapping, a missing, extra or repeated key, or a value that is not a name |
| `binding_unknown` | no installed Template or Configuration of that name (`field` names the key) |
| `binding_not_allowed` | both exist but the pair is not on the allow-list |
| `binding_duplicate` | a second block with a `binding:` key |
| `no_binding` | no block and no default Binding |

All problems are reported at once. A Collection the user cannot read answers
403 or 404 with a message that names the Collection.

## The button

The Document view shows the button where the Collection has a block (valid or
not) or the installation has a default Binding, see the status endpoint in
[Architecture](architecture.md#status-endpoint). A block with a typo keeps
the button visible so that the author sees the error on click.
