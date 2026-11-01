# Outprint logo

Symbol: a page that is also the letter O. The folded corner is the only
green element, so the mark also works in a single colour.

## Files

| Folder | File | Use |
|---|---|---|
| `svg/` | `outprint-symbol.svg` | Symbol, full colour, for light backgrounds |
| | `outprint-symbol-dark.svg` | Symbol, full colour, for dark backgrounds |
| | `outprint-symbol-black.svg`, `-white.svg` | One-colour symbol (print, embossing, overlays) |
| | `outprint-lockup.svg`, `-dark`, `-black`, `-white` | Symbol + wordmark "Outprint", same four variants |
| `png/` | `outprint-symbol*-512.png` | Transparent symbol, 512 × 512 |
| | `outprint-lockup*-1200w.png` | Transparent lockup, 1200 px wide |
| | `outprint-avatar-512.png`, `-dark-512.png` | Opaque square for the container registry / GitHub org avatar |
| `favicon/` | `favicon.svg` | Adapts to light and dark browser themes (`prefers-color-scheme`) |
| | `favicon.ico` | 16 and 32 px in one file |
| | `favicon-16.png`, `favicon-32.png` | The 16 px file uses a pixel-fitted drawing |
| | `apple-touch-icon.png` | 180 × 180, opaque |
| `social/` | `social-preview.png` | GitHub social preview, 1280 × 640 (light) |
| | `social-preview-dark.png` | Same, dark |

All SVGs are plain paths: the wordmark is converted to outlines, there is no
live text, no embedded raster image and no external reference. SVGs are
minified with svgo (2 decimals, viewBox kept); PNGs are palette-quantised
with libimagequant (the engine behind pngquant/TinyPNG) and recompressed
with oxipng. The kit is about 105 KB in total.

## Snippets

Docs site / any HTML page:

```html
<link rel="icon" href="/favicon.ico" sizes="any">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
```

README header that follows GitHub's light/dark theme:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/svg/outprint-lockup-dark.svg">
  <img alt="Outprint" src="docs/assets/logo/svg/outprint-lockup.svg" width="200">
</picture>
```

GitHub social preview: repository Settings → General → Social preview →
upload `social/social-preview.png`.

## Rules

- **Clear space:** at least F on every side, where F is the folded corner
  (9 of the 32 grid units, about a quarter of the symbol).
- **Minimum size:** symbol 16 px on screen / 5 mm in print; lockup 20 px
  high / 6 mm. Smaller than that, use the symbol alone.
- **Lockup proportions:** symbol box height = type size; gap = ¼ of the box.
- **Colour:** ink `#111111` on light, `#ededed` on dark, accent `#2ecc71`
  on the fold only. Never colour the ring or wordmark green.
- Do not add gradients, shadows, outlines or effects; do not rotate or
  stretch the mark.

## Typeface

Wordmark and tagline: **Roboto** (Medium 500 for the wordmark, Regular 400
for the tagline), version 3.009, © Google, licensed under the
**SIL Open Font License 1.1**. The SVGs contain outlines only, no font
file, so no font is redistributed with the logo.

## Rights

Original work, created for the Outprint project. Rights to be assigned to
the maintainer (task.media), who decides the licence of these files.
**Licence: not yet decided.** Until then: usable by the project and by
anyone running or linking to Outprint; not to be used to imply
endorsement.

Outprint is independent of Outline. The logo uses none of Outline's,
Adobe's or the TeX project's marks or colours.

## Rebuild

`build_logo.py` regenerates every file from the geometry in the script:

```sh
npm install                       # roboto fonts + svgo (SVG minification)
python3 -m venv .venv             # Homebrew Python blocks global pip (PEP 668)
.venv/bin/pip install -r requirements.txt   # plus rsvg-convert (librsvg)
.venv/bin/python build_logo.py    # fonts and output default to this folder
```

In VS Code, pick `.venv/bin/python` via *Python: Select Interpreter*; the
script then runs without arguments. Optional: `build_logo.py <fontdir> <outdir>`.
