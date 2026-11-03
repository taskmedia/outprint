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

- 50 pictures in one document.
- 10 MiB for one picture (an SVG: 2 MiB).
- 50 MiB for all pictures together.
- 40 megapixels in one picture. That is, for example, 8000 by 5000 pixels.

See [Limits you may hit](limits-you-may-hit.md) for the other limits.

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

The logo on your letterhead is not part of your document. The admin of the
Collection sets it once. You cannot change it in a document. See
[Fill in the document](fill-in-the-document.md#defaults-and-what-you-cannot-change).

## When a picture is refused

The message lists every picture that has a problem, with its line. Find the
code of the message in [When something goes wrong](when-something-goes-wrong.md).

| Code | What it means | What to do |
|------|---------------|------------|
| [`missing`](when-something-goes-wrong.md#missing) | The picture has no address, or it was deleted | Upload it again |
| [`forbidden`](when-something-goes-wrong.md#forbidden) | The picture comes from another website, or you may not read it | Upload your own copy |
| [`too_large`](when-something-goes-wrong.md#too_large) | A size or count limit was passed | Shrink it or use fewer pictures |
| [`format`](when-something-goes-wrong.md#format) | The format is not supported, or the picture is an animation | Export a PNG or JPEG |
| [`decode`](when-something-goes-wrong.md#decode) | The file is damaged | Save it again and upload the new file |
| [`pixels`](when-something-goes-wrong.md#pixels) | The picture has too many pixels | Scale it down |
| [`svg_rejected`](when-something-goes-wrong.md#svg_rejected) | The SVG holds something not allowed | Export a plain SVG, or a PNG |
| [`syntax`](when-something-goes-wrong.md#syntax) | The picture is written across several lines | Put it on a line of its own |

Do not add curly-bracket settings such as `{width=50%}` after a picture. They
are ignored. Use the size handles in Outline.
