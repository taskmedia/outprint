---
sidebar_position: 5
---

# Images and logos

**How-to.** Add pictures to a document and fix a refused picture.

Pictures you add to a document in Outline appear in the PDF. This page tells
you what works and what to do when a picture is refused.

## Add a picture

Upload the picture to the document in Outline. Drag it in, paste it, or use the
image button of the editor. Pictures that you only link from another website
do not work.

Outprint accepts these formats:

- PNG
- JPEG
- WebP
- GIF, if it is not animated
- SVG, the format of drawings and diagrams

Outprint looks at the content of the file, not at its name. A file called
`photo.png` that is really something else is refused.

## Size

Resize the picture in Outline. The PDF uses the size you chose. A picture that
is too big for the page is scaled down to fit. It is never cut off.

If you do not choose a size, the picture has its natural size.

## Captions

A picture on a line of its own gets a number and a caption in the PDF. The
caption is the text you type under the picture in Outline.

A picture without a caption text, or one that sits inside a line of text, has no
caption.

## What happens to your file

Outprint saves a clean copy of every picture. A photo taken sideways is turned
upright. Details such as the camera model or the place of the photo are
removed. PNG stays sharp, and JPEG stays at high quality.

## Limits

A document can hold up to 50 pictures. One picture may have up to 10 MiB (an
SVG: 2 MiB) and 40 megapixels. See [Limits](../reference/limits.md) for all
sizes.

## Drawings and diagrams (SVG)

An SVG stays sharp at every size. Two things help.

- **Turn text into outlines.** Outprint draws text in an SVG with its own fonts.
  If your font is missing, it uses another one without telling you. The picture
  can then look different from the screen. In your drawing program, choose
  "Object to path" or "Outline text" before you export. The text then looks as
  you drew it.
- **Export plain SVG.** Pictures with scripts, links to other files or
  external styles are refused for safety.

## The logo

The logo on your letterhead is not part of your document. It comes from the
[Configuration](../reference/glossary.md#configuration) of the
[Collection](../reference/glossary.md#collection), so you cannot change it in a
document. See
[Get a PDF](get-a-pdf.md#fill-in-the-details-at-the-top).

## When a picture is refused

The message lists every picture that has a problem, with its line. Each code has
an entry in [Error messages](../reference/error-messages.md) that says what to
do. The usual fixes:

- Upload the picture again: [`missing`](../reference/error-messages.md#missing),
  [`decode`](../reference/error-messages.md#decode). Upload your own copy if it
  came from another website:
  [`forbidden`](../reference/error-messages.md#forbidden).
- Make it smaller or use fewer pictures:
  [`too_large`](../reference/error-messages.md#too_large),
  [`pixels`](../reference/error-messages.md#pixels).
- Export a PNG, a JPEG or a plain SVG:
  [`format`](../reference/error-messages.md#format),
  [`svg_rejected`](../reference/error-messages.md#svg_rejected).
- Put the picture on a line of its own:
  [`syntax`](../reference/error-messages.md#syntax).

Do not add curly-bracket settings such as `{width=50%}` after a picture. They
are ignored. Use the size handles in Outline.
