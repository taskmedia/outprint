---
sidebar_position: 0
---

# Install ways

Outprint has three ways to start a PDF. They share the Gateway and the Worker,
except the command line, which needs neither. Pick one on the
[Deploy overview](../README.md#which-way-fits-you), then follow its page.

| Way | The user | You run |
|-----|----------|---------|
| [Export menu in Outline](export-menu.md) | clicks **Export → Formatted PDF** inside Outline | Gateway, Worker and the injector in front of Outline |
| [PDF by link](pdf-by-link.md) | opens a link while signed in to Outline | Gateway and Worker, plus one route on the edge proxy |
| [Command line](command-line.md) | runs `outprint render` on a Markdown file | the `outprint` image only |

You can start with the link and add the injector later. All of them build on the
[common base](../common-base.md).
