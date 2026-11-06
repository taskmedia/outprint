---
sidebar_position: 4
---

# Images in the pipeline

How the Gateway checks, converts and places images, and how SVG is handled.
Authors find the short version in [Images and logos](../write/images-and-logos.md);
operators find the settings in [Image fetching](image-fetching.md).
The limits are listed in [Limits and admission control](../install/operate/limits-and-admission.md).

## SVG

An SVG is rendered as a vector: it stays sharp at any zoom and its text
stays text. Sizing, captions and the limits above work as for any image; the
natural size is the one in the file (see below).

**What is accepted**: a plain SVG 1.1 or 1.2 file in UTF-8 whose root element
is `svg` in the namespace `http://www.w3.org/2000/svg`. Shapes, paths, text,
gradients, patterns, clip paths, masks, filters, `<use>` and `<style>` work.
Pictures are embedded as `data:image/png`, `jpeg`, `gif` or `webp` with
`;base64,` in an `<image>`.

**Size**: the natural size comes from `width` and `height` on the root
element (units `px`, `pt`, `pc`, `mm`, `cm`, `in`; a bare number is a pixel),
and where one is missing or relative (`100%`, `em`) from the `viewBox`. At
96 dpi, like Outline's sizes. An SVG with neither is rejected. Sides beyond
100 000 pixels and viewBox sides beyond 1 000 000 units are rejected as
absurd.

**What is rejected, and why**, all as `svg_rejected` unless noted. The message
names the reason and, at most, a short part of the offending name; it never
quotes the file.

| Rejected | Why |
|----------|-----|
| a `DOCTYPE` or `ENTITY` declaration, anywhere | external entities read files and fetch URLs; nested entities expand a few bytes into gigabytes (XXE, "billion laughs"). Export without a DOCTYPE. |
| `<script>`, `<foreignObject>` | active content and embedded HTML; PDF output cannot use them anyway |
| attributes starting with `on` (`onload`, `onclick`) | event handlers |
| any `href` or `xlink:href` that is not a `#fragment` of the same file, or, on `<image>`, an embedded raster | `file:`, `http(s):`, relative paths, `javascript:` and `data:image/svg+xml` all load something from outside. Links to web pages (`<a href="https://...">`) are rejected too: a PDF made from a diagram does not need them. |
| `url()` that is not `url(#fragment)`, in any attribute, `style` attribute or `<style>`; `@import`; `image-set()`; a backslash (CSS escape) in styles | same reason; an escape could spell a keyword in a form a scan does not see |
| a processing instruction other than the XML declaration | `xml-stylesheet` loads a style sheet |
| a declared encoding other than UTF-8, and UTF-16 files | the checker and the converter must read the same bytes. A UTF-8 byte order mark is accepted and dropped. Save the file as UTF-8. |
| a file that is not well-formed XML; text or a second element after the root; a root outside the SVG namespace | the converter would refuse it or read it differently |
| more than 100 000 elements, nesting deeper than 50, `<use>` loops, and `<use>` that expands to more than 500 000 elements | the converter's own limits, and exponential `<use>` bombs (a few lines that name groups of groups of groups) |
| an embedded picture that is not valid, or has more than 40 megapixels | a decompression bomb inside the SVG |
| no usable size, or an absurd one (above) | |
| more than 2 MiB (`too_large`) | |

The logo of a Configuration is an SVG too. The Configuration comes
from the operator, so its logo is not pre-checked, but it goes through the
same sandboxed conversion as a Document's SVG.

**Text in an SVG uses the fonts of the Worker image, and a font that is
missing is replaced without any warning.** The Document's fonts are the
Template's; an SVG is typeset by the converter, which asks the system for
the font names written in the file. The image has the TeX Gyre fonts. A
`font-family="Helvetica"` or `"sans-serif"` is drawn with a TeX Gyre font
(`sans-serif` is TeX Gyre Adventor), and any other name falls back the same
way, so the picture can look different from the one on screen and no check
catches it. **Export text as paths** (Inkscape: Path, Object to Path;
Figma and most chart tools: "outline text") for a faithful result. The same
holds for the text of an SVG logo. Text drawn as paths is also not searchable
or selectable in the PDF.

An SVG that passes these checks can still be too complex to convert. The
Worker stops a conversion after 20 seconds and refuses one that produces
more than 10 MiB of PDF; the render fails with an error that names the SVG
phase (see [Worker pipeline](worker.md)). Embedded pictures are not recompressed,
so a large photo inside an SVG counts fully against the limits.

## How the Gateway works

1. The Markdown body is parsed with goldmark (CommonMark, pipe tables,
   strikethrough). Images are found inside links, lists, quotes and tables,
   in inline and reference style, and never in code. The exact source range of
   each image is recorded.
2. Each address is classified. Accepted are the attachment path
   `/api/attachments.redirect?id=...` (relative, or absolute with the Outline
   origin) and `/api/files.get?...`. A path prefix of the Outline URL is
   honoured. Everything else is `forbidden`.
3. Each distinct attachment is fetched once per render, canonicalised, and
   gets a name `img-<n>.png`, `img-<n>.jpg` or `img-<n>.svg`.
4. Each image is rewritten in place to
   `![alt](images/<name>){width=Wpt height=Hpt}`. The alt text is plain text
   with every markup character escaped; the destination and the numbers are
   generated. Nothing from the Document is passed on as syntax. An attribute
   block that the author wrote after an image is dropped.
5. Problems of the Metadata Block and of the images go into one 422 list. The
   `field` is the alt text (at most 40 characters) or `image`, `code` the kind,
   `line` the Document line; the message adds the column.

goldmark and Pandoc are two parsers. Where they disagree about what an image
is, the Worker's filter still rejects every image that is not a file of the
bundle, so a disagreement fails the render and cannot slip a path through.
Raw HTML `<img>` is not an image: the Worker drops it like all raw HTML.

The `render` command runs the same pipeline with images read from the
directory of the input file (no `..`, no links out of it).

## Notes

- Verified: images, links and tables are found at the right source range, and
  attachments are fetched with the redirect rules above, against fakes.
- **Verified (Outline 1.12.0, E2E stack): an image caption is the alt text.**
  Outline's image node has no separate caption field. The editor's caption line
  under an image is the node's `alt` attribute (`shared/editor/nodes/Image`
  renders `alt` as the `<p>` caption), and the Markdown export is
  `![caption](/api/attachments.redirect?id=<uuid> " =WxH")`: the caption in the
  brackets, the size after the title as ` =WxH` (width or height may be empty).
  A layout choice such as full width is written into the title position as a
  class name and a size follows it; the Gateway does not use them. The E2E test
  reads `documents.info` of a seeded Document with two captioned images and
  checks this, and the editor view of the same Document shows the captions
  under the images. So the specification's "alt text is the caption" holds.
- **Verified: attachments with Outline's local file storage.**
  `/api/attachments.redirect?id=...` answers 302 to a signed
  `<Outline URL>/api/files.get?sig=...`. The signature is the credential, so the
  Gateway's rule of following redirects without the cookie works. The target
  names Outline by its public URL (the `URL` setting). When the Gateway reaches
  Outline at an internal address, that host is not the configured origin and
  would be refused, so set `OUTLINE_PUBLIC_URL` on the Gateway; see
  [Image fetching](image-fetching.md#outline-behind-an-internal-address).
- SVG: the Gateway pre-check is in `internal/images/svg*.go`; the conversion is
  the first phase of the Worker (`internal/worker/svg.go`). The attack corpus
  that backs the table above is `internal/svgcorpus` (see
  [Security](security.md)).
