"""Build the Outprint logo kit: SVG (text as outlines), PNG, ICO, social preview."""
import os, subprocess, sys
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.boundsPen import BoundsPen
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
FONTDIR = sys.argv[1] if len(sys.argv) > 1 else os.path.join(HERE, "node_modules/@fontsource/roboto/files")
OUT = sys.argv[2] if len(sys.argv) > 2 else HERE

INK, INK_DARK, ACCENT = "#111111", "#ededed", "#2ecc71"
PAPER, NIGHT = "#f2f2f2", "#080808"

RING = "M18 5H13A8 8 0 0 0 5 13V19A8 8 0 0 0 13 27H19A8 8 0 0 0 27 19V14"
FOLD = "M18 5V14H27Z"
# pixel-fitted variant for 16 px: edges land on whole pixels
RING16 = "M18 4H12A8 8 0 0 0 4 12V20A8 8 0 0 0 12 28H20A8 8 0 0 0 28 20V14"
FOLD16 = "M18 4V14H28Z"


def symbol_paths(ink, accent, ring=RING, fold=FOLD):
    return (f'<path d="{ring}" fill="none" stroke="{ink}" stroke-width="4" '
            f'stroke-linecap="round" stroke-linejoin="round"/>'
            f'<path d="{fold}" fill="{accent}" stroke="{accent}" stroke-width="4" '
            f'stroke-linejoin="round"/>')


class Face:
    def __init__(self, path):
        self.f = TTFont(path)
        self.gs = self.f.getGlyphSet()
        self.cmap = self.f.getBestCmap()
        self.upm = self.f["head"].unitsPerEm
        self.hmtx = self.f["hmtx"]

    def outline(self, text, size, x, baseline, tracking=0.0):
        """Return (svg path d, bounds) for text set at `size`, baseline y, start x."""
        s = size / self.upm
        pen = SVGPathPen(self.gs)
        bpen = BoundsPen(self.gs)
        cx = x
        for ch in text:
            g = self.cmap[ord(ch)]
            t = (s, 0, 0, -s, cx, baseline)
            self.gs[g].draw(TransformPen(pen, t))
            self.gs[g].draw(TransformPen(bpen, t))
            cx += self.hmtx[g][0] * s + tracking * size
        return pen.getCommands(), bpen.bounds


def fmt(v):
    return f"{v:.2f}".rstrip("0").rstrip(".")


def write(rel, content):
    p = os.path.join(OUT, rel)
    os.makedirs(os.path.dirname(p), exist_ok=True)
    with open(p, "w") as fh:
        fh.write(content)
    return p


def render(svg_path, png_rel, w, h=None):
    p = os.path.join(OUT, png_rel)
    os.makedirs(os.path.dirname(p), exist_ok=True)
    cmd = ["rsvg-convert", "-w", str(w)] + (["-h", str(h)] if h else []) + ["-o", p, svg_path]
    subprocess.run(cmd, check=True)
    return p


med = Face(os.path.join(FONTDIR, "roboto-latin-500-normal.woff"))
reg = Face(os.path.join(FONTDIR, "roboto-latin-400-normal.woff"))
CAP = 1456 / 2048  # Roboto cap height

# ---------- symbol ----------
VARIANTS = {
    "": (INK, ACCENT),            # full colour, for light backgrounds
    "-dark": (INK_DARK, ACCENT),  # full colour, for dark backgrounds
    "-black": ("#000000", "#000000"),
    "-white": ("#ffffff", "#ffffff"),
}
TITLE = "<title>Outprint</title>"

for suf, (ink, acc) in VARIANTS.items():
    write(f"svg/outprint-symbol{suf}.svg",
          f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">'
          f'{TITLE}{symbol_paths(ink, acc)}</svg>\n')

# ---------- lockup (symbol box height = type size = 32, gap = 8) ----------
S = 32
baseline = S / 2 + CAP * S / 2
d_word, (x0, y0, x1, y1) = med.outline("Outprint", S, S * 1.25, baseline, tracking=-0.01)
top, bottom = min(3, y0), max(29, y1)
left, right = 3, x1
pad = 0
vb = f"{fmt(left - pad)} {fmt(top - pad)} {fmt(right - left + 2 * pad)} {fmt(bottom - top + 2 * pad)}"
lock_w, lock_h = right - left, bottom - top
for suf, (ink, acc) in VARIANTS.items():
    write(f"svg/outprint-lockup{suf}.svg",
          f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}" '
          f'width="{fmt(lock_w * 4)}" height="{fmt(lock_h * 4)}">{TITLE}'
          f'{symbol_paths(ink, acc)}<path d="{d_word}" fill="{ink}"/></svg>\n')

# ---------- favicon ----------
fav_svg = write("favicon/favicon.svg",
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">'
    '<style>path{stroke-width:4;stroke-linejoin:round}.i{stroke:#111}'
    '@media (prefers-color-scheme:dark){.i{stroke:#ededed}}</style>'
    '<path class="i" fill="none" stroke-linecap="round" d="M18 5h-5a8 8 0 0 0-8 8v6a8 8 0 0 0 8 8h6a8 8 0 0 0 8-8v-5"/>'
    '<path fill="#2ecc71" stroke="#2ecc71" d="M18 5v9h9z"/></svg>')

tmp = os.path.join(OUT, ".build")
os.makedirs(tmp, exist_ok=True)


def tmp_svg(name, body, vb="0 0 32 32", bg=None):
    bgrect = f'<rect x="-100" y="-100" width="10000" height="10000" fill="{bg}"/>' if bg else ""
    p = os.path.join(tmp, name)
    with open(p, "w") as fh:
        fh.write(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}">{bgrect}{body}</svg>')
    return p


s16 = tmp_svg("s16.svg", symbol_paths(INK, ACCENT, RING16, FOLD16))
s32 = os.path.join(OUT, "svg/outprint-symbol.svg")
render(s16, "favicon/favicon-16.png", 16, 16)
render(s32, "favicon/favicon-32.png", 32, 32)
render(s32, "favicon/favicon-48.png", 48, 48)
# touch icon: opaque, symbol at ~73 % on paper
touch = tmp_svg("touch.svg", f'<g transform="translate(4.36 4.36) scale(0.727)">{symbol_paths(INK, ACCENT)}</g>', bg=PAPER)
render(touch, "favicon/apple-touch-icon.png", 180, 180)
ico = [Image.open(os.path.join(OUT, f"favicon/favicon-{n}.png")).convert("RGBA") for n in (16, 32, 48)]
ico[2].save(os.path.join(OUT, "favicon/favicon.ico"), sizes=[(16, 16), (32, 32), (48, 48)],
            append_images=ico[:2])
os.remove(os.path.join(OUT, "favicon/favicon-48.png"))

# ---------- PNG exports ----------
for suf in VARIANTS:
    render(os.path.join(OUT, f"svg/outprint-symbol{suf}.svg"), f"png/outprint-symbol{suf}-512.png", 512, 512)
    render(os.path.join(OUT, f"svg/outprint-lockup{suf}.svg"), f"png/outprint-lockup{suf}-1200w.png", 1200)

# registry avatar: opaque square, symbol 336/512
for name, bg, ink in (("", PAPER, INK), ("-dark", NIGHT, INK_DARK)):
    k = 336 / 512
    off = 16 * (1 - k)
    p = tmp_svg(f"reg{name}.svg", f'<g transform="translate({off} {off}) scale({k})">{symbol_paths(ink, ACCENT)}</g>', bg=bg)
    render(p, f"png/outprint-avatar{name}-512.png", 512, 512)

# ---------- social preview 1280 x 640 ----------
def social(bg, ink, muted, name):
    W, H = 1280, 640
    sym, fs = 128, 128
    gap = 32
    _, (wx0, _, wx1, _) = med.outline("Outprint", fs, 0, 0, tracking=-0.01)
    word_w = wx1
    block_w = sym + gap + word_w
    x = (W - block_w) / 2
    tag_size = 40
    _, (tx0, _, tx1, _) = reg.outline("PDFs from Outline documents.", tag_size, 0, 0)
    # vertical: lockup row (128) + 48 gap + tagline cap height
    row_h = sym
    tag_cap = CAP * tag_size
    total = row_h + 56 + tag_cap
    y_row = (H - total) / 2
    sym_svg = f'<g transform="translate({x} {y_row}) scale({sym / 32})">{symbol_paths(ink, ACCENT)}</g>'
    word_base = y_row + sym / 2 + CAP * fs / 2
    d_w, _ = med.outline("Outprint", fs, x + sym + gap, word_base, tracking=-0.01)
    tag_base = y_row + row_h + 56 + tag_cap
    d_t, _ = reg.outline("PDFs from Outline documents.", tag_size, (W - (tx1 - tx0)) / 2 - tx0, tag_base)
    body = (f'<rect width="{W}" height="{H}" fill="{bg}"/>{sym_svg}'
            f'<path d="{d_w}" fill="{ink}"/><path d="{d_t}" fill="{muted}"/>')
    svg = write(f"social/social-preview{name}.svg",
                f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}">{TITLE}{body}</svg>\n')
    render(svg, f"social/social-preview{name}.png", W, H)

social(PAPER, INK, "#4a4a4a", "")
social(NIGHT, INK_DARK, "#a8a8a8", "-dark")

# clean temp
for f in os.listdir(tmp):
    os.remove(os.path.join(tmp, f))
os.rmdir(tmp)


# ---------- size optimisation ----------
def local_svgo():
    path = os.path.join(HERE, "node_modules/.bin/svgo")
    return path if os.path.exists(path) else None


def optimise():
    import shutil, json, tempfile
    # SVG: svgo (npm), viewBox kept, 2 decimals. favicon.svg is hand-minimal
    # already and keeps its prefers-color-scheme <style>, so it is skipped.
    svgo = os.environ.get("SVGO") or shutil.which("svgo") or local_svgo()
    if svgo:
        cfg = os.path.join(tempfile.mkdtemp(), "svgo.config.js")
        with open(cfg, "w") as fh:
            fh.write("module.exports={multipass:true,floatPrecision:2,plugins:[{name:'preset-default',"
                     "params:{overrides:{removeViewBox:false}}}]};")
        svgs = [os.path.join(r, f) for r, _, fs in os.walk(OUT) for f in fs
                if f.endswith(".svg") and f != "favicon.svg"]
        subprocess.run([svgo, "--config", cfg, "-q", *svgs], check=True)
    else:
        print("svgo not found: SVGs left unminified")
    # PNG: palette quantisation (libimagequant, as pngquant/TinyPNG) + oxipng
    try:
        import imagequant, oxipng
    except ImportError:
        print("imagequant/oxipng missing: PNGs left as rendered")
        return
    for r, _, fs in os.walk(OUT):
        for f in fs:
            if not f.endswith(".png"):
                continue
            p = os.path.join(r, f)
            im = Image.open(p).convert("RGBA")
            q = imagequant.quantize_pil_image(im, dithering_level=1.0, max_colors=64,
                                              min_quality=80, max_quality=100)
            q.save(p, optimize=True)
            oxipng.optimize(p, level=6, strip=oxipng.StripChunks.safe())
    # rebuild the .ico from the optimised PNGs (16/32 only; 48 is rarely used)
    icos = [Image.open(os.path.join(OUT, f"favicon/favicon-{n}.png")).convert("RGBA") for n in (16, 32)]
    icos[1].save(os.path.join(OUT, "favicon/favicon.ico"), sizes=[(16, 16), (32, 32)],
                 append_images=icos[:1])


optimise()
print("lockup viewBox", vb)
