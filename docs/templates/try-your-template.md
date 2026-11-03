---
sidebar_position: 3
---

# Try your Template

**How-to.** Render a Document and validate your packages before you roll them out.

The `outprint` image has two commands that run the same pipeline as the Worker,
without Outline and without a Gateway. They do not connect to Outline.

- `validate` checks Templates and Configurations.
- `render` turns one Markdown file into a PDF with a Template and a
  Configuration.

You need Docker. From the repository root, build the image:

```sh
docker build -t outprint .
```

## Validate

Check the packages shipped in the image, or your own mounted over them:

```sh
docker run --rm outprint validate
docker run --rm -v "$PWD/templates:/opt/outprint/templates:ro" \
  -v "$PWD/configurations:/opt/outprint/configurations:ro" \
  outprint validate --template mine --config example
```

It ends with `validate: ok` when nothing is wrong. Errors give exit code 1,
warnings alone are success. The checks and the output format are in
[Template checks](../reference/template-checks.md#output-of-validate).

## Render

```sh
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/data" outprint render \
  --templates-root /data/templates --configs-root /data/configurations \
  --template mine --config example --date 2026-03-02 \
  -o /data/out.pdf /data/input.md
```

This writes `out.pdf` in the current directory. Without the two `--*-root`
flags, `render` uses the packages in the image, for example `--template neutral
--config example`. Useful flags:

- `--title TEXT` and `--date YYYY-MM-DD` set the default title and date.
- `--json-errors` prints metadata problems as the JSON error body on stdout.
- `--debug-tex-log` prints the full TeX log. It may contain document content.
- `--timeout DURATION` limits the render.

`outprint --help` prints every flag.

## Typical problems

| What you see | What to do |
|--------------|-----------|
| `validate` lists problems | fix what it names; see [Template checks](../reference/template-checks.md) |
| `read input: open /data/...: no such file or directory` | mount the directory with `-v "$PWD:/data"` and use the path inside the container |
| `template/nope manifest.yaml: read: ... no such file or directory` | pick a name from `/opt/outprint/templates`, or mount your own packages |

The [Template author guide](template-author-guide.md) covers the rest of the
workflow.
