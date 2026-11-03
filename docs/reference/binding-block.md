---
sidebar_position: 4
---

# Binding block

**Reference.** The block in a Collection description that chooses a Template and a Configuration.

A Collection admin adds the block to the description of a Collection in
Outline. How to do it, step by step, is in
[Bind a Template to a Collection](../configure/README.md).

<!-- validate: binding -->
````markdown
```yaml
binding:
  template: neutral
  configuration: example
```
````

## Rules for the block

- It is a fenced code block with the language `yaml`. Only a block whose root
  key is `binding` counts; other `yaml` blocks are ordinary text.
- It has exactly the key `binding`, and below it exactly the keys `template`
  and `configuration`. A missing, extra or repeated key is
  [`binding_malformed`](error-messages.md#binding_malformed).
- A second block is [`binding_duplicate`](error-messages.md#binding_duplicate).
- Every problem is reported at once, with the source `collection` and the line
  counted from the first line of the description.

## `binding`

**Type:** mapping. **Required:** yes. **Default:** none.

The only key at the root of the block.

## `binding.template`

**Type:** `string`. **Required:** yes. **Default:** none.

The name of an installed Template. A name that is not installed is
[`binding_unknown`](error-messages.md#binding_unknown).

<!-- validate: skip fragment of a single key, not a complete block -->
```yaml
binding:
  template: neutral
```

## `binding.configuration`

**Type:** `string`. **Required:** yes. **Default:** none.

The name of an installed Template Configuration. A name that is not installed
is [`binding_unknown`](error-messages.md#binding_unknown). If both names exist
but the pair is not in the `bindings` list of the
[service configuration](service-configuration.md#bindings), the error is
[`binding_not_allowed`](error-messages.md#binding_not_allowed).

<!-- validate: skip fragment of a single key, not a complete block -->
```yaml
binding:
  configuration: example
```

## Without a block

| The Collection has | Outprint |
|--------------------|----------|
| a block | uses it; a mistake in it is an error, never a fallback to the default |
| no block, and the installation has a [`defaultBinding`](service-configuration.md#defaultbinding) | uses the default pair |
| neither | answers [`no_binding`](error-messages.md#no_binding) |
