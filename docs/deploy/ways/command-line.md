---
sidebar_position: 3
---

# Command line

## What it does

The `outprint` image has two commands that run the same pipeline as the Worker,
without Outline and without a Gateway:

- `render` turns one Markdown file into a PDF with a Template and a
  Configuration.
- `validate` checks Templates and Configurations.

## When it fits

- You write or change a Template and want to see the PDF before you roll it out.
- You want to check your own packages in CI.
- You have Markdown files and no Outline.

It does not connect to Outline. It cannot read a document from there or add an
action.

## Prerequisites

- Docker, and the `outprint` image: `docker build -t outprint .` in the
  repository. No other part of the [common base](../common-base.md) is needed.

## Install

Nothing to run permanently. These commands use the image.

### Docker

Run these from the repository root.

```sh
docker build -t outprint .
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/data" outprint render --template neutral \
  --config example --date 2026-03-02 -o /data/out.pdf /data/testdata/fixtures/letter-full/input.md
```

Check the packages shipped in the image, or your own:

```sh
docker run --rm outprint validate
docker run --rm -v "$PWD/templates:/opt/outprint/templates:ro" \
  -v "$PWD/configurations:/opt/outprint/configurations:ro" \
  outprint validate
```

### Kubernetes

Not applicable.

## Check that it works

1. `docker run --rm outprint validate` ends with `validate: ok`.
2. The `render` command above writes `out.pdf` in the current directory.

## Typical problems

| What you see | Cause | What to do |
|--------------|-------|-----------|
| `validate` lists problems | a package breaks a rule | fix what it names; see [Packages](../../templates/packages.md) |
| `outprint: read input: open /data/...: no such file or directory` | the file is not mounted where the command looks | mount the directory with `-v "$PWD:/data"` and use the path inside the container |
| `template/nope manifest.yaml: read: ... no such file or directory` | the Template name is not in the image | pick a name from `/opt/outprint/templates`, or mount your own packages as shown above |

The [Template author guide](../../templates/template-author-guide.md) covers the
rest of the workflow.
