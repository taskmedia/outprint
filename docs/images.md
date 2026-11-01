# Images

Images uploaded to Outline appear in the PDF at the size the author chose.
The Gateway fetches and checks them; the Worker only sees clean files.

## For authors

**Supported formats**: PNG, JPEG, WebP, static GIF and SVG, detected by
content, not by name. Animated GIF and animated WebP are rejected. Upload the image to Outline; images linked from other sites,
`data:` and `file:` addresses are rejected.

**What happens to a file**: a raster image is decoded and written again. PNG
stays lossless, JPEG is saved at quality 95, WebP and GIF become PNG. A JPEG's
EXIF orientation is applied, so a photo taken sideways is upright. All other
metadata (camera, location, colour profile) is dropped. An SVG is checked and
then kept as it is; see [SVG](#svg).

**Size**: Outline's size syntax is respected. Pixels are converted at 96 dpi
(a pixel is 0.75 pt).

| In Outline | Result |
|------------|--------|
| no size | natural size |
| `=400x300` | 400 by 300 px |
| `=400` or `=400x` | width 400 px, height from the aspect ratio |
| `=x300` | height 300 px, width from the aspect ratio |

An image that does not fit is scaled down, aspect kept, to the text width
and to 0.9 of the text height. It is never cut off.

**Captions**: an image alone in its paragraph, with alt text, becomes a
numbered figure and the alt text is the caption. An image without alt text, or
inside a line of text, has no caption. Alt text is plain text: markup in it is
shown as written.

**Limits**: 50 images per Document, 10 MiB per file (as uploaded and after
conversion), 50 MiB in total, 40 megapixels per image.

**Problems** are all reported at once, each with its line, its alt text and a
kind:

| Kind | Meaning |
|------|---------|
| `missing` | no address or attachment id, or the attachment was not found |
| `forbidden` | not an Outline attachment (other site, `data:`, `file:`, a relative path), not readable by you, or stored on a host the service may not fetch from |
| `too_large` | over 10 MiB (an SVG: over 2 MiB), over 50 images, or over 50 MiB in total |
| `format` | not PNG, JPEG, WebP, a static GIF or an SVG; the message names what was found |
| `svg_rejected` | an SVG that is not allowed or cannot be used; the message says why |
| `decode` | the file is damaged |
| `pixels` | more than 40 megapixels |
| `syntax` | the image Markdown spans lines; put it on one line |

Do not use Pandoc attribute blocks (`{width=...}`) after an image: they are
ignored.

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

The logo of a Template Configuration is an SVG too. The Configuration comes
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
phase (see [worker.md](worker.md)). Embedded pictures are not recompressed,
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

## For operators

**Fetching images is the Gateway's only outbound request besides Outline's
API.** The model:

- The user's cookie is sent to the Outline origin and nowhere else. The first
  request goes to the configured Outline URL, with the cookie.
- Redirects are followed by hand, at most 5, **without the cookie**, and only
  to hosts on the allow-list: the Outline origin and `imageHosts`. Only
  `http` and `https`, no credentials in the URL.
- Names are resolved by the Gateway, once per request. Every answer must be a
  public address: loopback, private, link-local (including the cloud metadata
  address), carrier-grade NAT, multicast, reserved and documentation ranges,
  and IPv4 written as IPv6, NAT64, 6to4 and Teredo forms are refused. The
  connection goes to the address that was checked, and the address is checked
  again when the socket connects, so a name that changes its answer cannot
  redirect the request (DNS rebinding).
- One exception: the configured Outline origin itself (host and port) may be
  at a private address, which the local development stack needs. No other
  host gets this.
- No proxy from the environment is used. A request is bounded by 30 s and a
  body by 10 MiB.

**`imageHosts`** in the service configuration lists the other hosts a redirect
may lead to, for example the file storage of an Outline that keeps files in
S3-compatible storage. It is empty by default, so such an Outline shows
`forbidden` with the host's name until the operator adds it:

```yaml
imageHosts:
  - files.example.invalid        # ports 80 and 443
  - storage.example.invalid:9000 # this port only
```

Entries are lower-case host names, optionally with a port: no scheme, path or
wildcard. A redirect target needs a public address, so a storage host on a
private network cannot be listed.

**Templates** must scale images to the page. The Gateway gives every image
an explicit size in points, so only the Template knows the text area. The
`neutral` Template redefines `\includegraphics` to scale an image down to the
line width and `0.9\textheight`, keeping the aspect ratio, and sets figure
captions through the `caption` package. A Template that shows Document images
must do the same.

## Notes

- Verified: images, links and tables are found at the right source range, and
  attachments are fetched with the redirect rules above, against fakes.
- **Assumption, not yet verified: Outline stores an image caption in the alt
  text.** The research notes in this repository describe the size syntax
  (`![alt](url " =WxH")`) and the attachment URL, but contain no export of a
  real document showing a caption. Alt text is therefore used as the caption,
  as the specification says.
  <!-- TODO(#28): in the end-to-end stack, upload an image, give it a caption in
  Outline, export the document and check that the caption is in the alt text
  and where the size appears. Also check that a redirect from an attachment
  to Outline's own file path works without the cookie (local storage). -->
- A redirect to the Outline origin carries no cookie, like every redirect.
  If an Outline with local file storage needs the cookie there, the end-to-end
  check will show it.
- SVG: the Gateway pre-check is in `internal/images/svg*.go`; the conversion is
  the first phase of the Worker (`internal/worker/svg.go`). The attack corpus
  that backs the table above is `internal/svgcorpus` (see
  [security.md](security.md)).
