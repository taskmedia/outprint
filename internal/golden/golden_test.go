//go:build golden

// Tier-2 tests: render fixtures with the real outprint binary and compare
// PDF facts and extracted text with checked-in goldens. PDF bytes are never
// compared. These tests are meant to run inside the pinned outprint image
// (see the golden target in the Dockerfile and `task test-golden`).
package golden

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// fixedDate is the last-modified date every fixture is rendered with, so
// that goldens do not depend on file times.
const fixedDate = "2026-03-02"

type testCase struct {
	name     string // fixture topic under testdata/fixtures
	template string
	config   string
	title    string // derived title, as the Outline document title would be
	// checkLogo asserts whether the letterhead logo is drawn in the upper
	// right of page one; reports place theirs elsewhere and skip it.
	checkLogo  bool
	expectLogo bool
	// literal lists strings that must survive into the PDF text verbatim.
	literal []string
	// forbidden lists strings that must not appear in the PDF text.
	forbidden []string
	// ownLine lists strings that must each stand on a line of their own, as
	// they do after a line break.
	ownLine []string
	// minPages is the least page count the document must reach.
	minPages int
	// repeatHeader is the table header text that every page showing a row
	// containing rowMarker must repeat.
	repeatHeader, rowMarker string
	// imageFacts adds the images of the PDF (pdfimages -list) to the facts
	// golden: page, pixel size, encoding and resolution.
	imageFacts bool
	// maxImageInches caps the placed size (pixels over resolution) of every
	// image of the PDF in both directions; zero means no check.
	maxImageInches float64
	// noRaster asserts that the PDF holds no raster image at all, which is
	// what a vector SVG leaves behind.
	noRaster bool
	// uprightImages lists images (by order) that must be taller than wide.
	uprightImages bool
}

var cases = []testCase{
	{name: "letter-basic", template: "neutral", config: "example", checkLogo: true, expectLogo: true},
	{name: "letter-full", template: "neutral", config: "example", title: "Spring plant sale", checkLogo: true, expectLogo: true},
	{name: "letter-no-recipient", template: "neutral", config: "example", checkLogo: true, expectLogo: true},
	{name: "letter-subject-default", template: "neutral", config: "example", title: "Page title as subject", checkLogo: true, expectLogo: true,
		literal: []string{"Page title as subject"}},
	{name: "letter-subject-explicit", template: "neutral", config: "example", title: "Page title unused", checkLogo: true, expectLogo: true,
		literal: []string{"Own subject line"}, forbidden: []string{"Page title unused"}},
	{name: "letter-subject-empty", template: "neutral", config: "example", title: "Page title unused", checkLogo: true, expectLogo: true,
		forbidden: []string{"Page title unused"}},
	{name: "letter-subject-null", template: "neutral", config: "example", title: "Page title unused", checkLogo: true, expectLogo: true,
		forbidden: []string{"Page title unused"}},
	{name: "letter-subject-special", template: "neutral", config: "example", title: `R&D: 50% off $5 #1 a_b {x} ~y ^z \ end`, checkLogo: true, expectLogo: true,
		literal: []string{`R&D: 50% off $5 #1 a_b {x} ~y ^z \ end`}},
	{name: "letter-override-default", template: "neutral", config: "example", checkLogo: true, expectLogo: true},
	{name: "letter-nonascii", template: "neutral", config: "example", checkLogo: true, expectLogo: true,
		literal: []string{"Grüße aus Köln", "Zażółć gęślą jaźń", "Åse Müller-Łukasiewicz", "Œillets", "2. März 2026"}},
	{name: "letter-literal-text", template: "neutral", config: "example", checkLogo: true, expectLogo: true,
		literal: []string{"$x$", `\input{foo}`, `\textbf{not bold}`, "$5 & 10% of #7", "under_score", "^caret"}},
	{name: "report-toc-abstract", template: "neutral", config: "example", title: "Annual garden report"},
	{name: "tables-narrow", template: "neutral", config: "example", title: "Garden inventory",
		literal: []string{"Tulip 12", "Crocus 140", "long cell mid 1.5"}},
	{name: "tables-wide", template: "neutral", config: "example", title: "Plot schedule",
		literal:   []string{"Sunny corner by the old fence", "secretary-of-the-garden-society@mail.example.org", "Needs a lot of"},
		forbidden: []string{"<br", "|---"}},
	{name: "tables-long", template: "neutral", config: "example", title: "Seed catalogue",
		minPages: 3, repeatHeader: "Ref Description Qty", rowMarker: "Item number",
		literal: []string{"Item number 1 with a short note", "Item number 90 with a short note", "End of the catalogue."}},
	{name: "linebreaks", template: "neutral", config: "example", checkLogo: true, expectLogo: true,
		ownLine:   []string{"First line", "second line", "third line", "fourth line", "A backslash break", "and a two-space break", "both end a line.", "1 Sample Lane", "12345 Exampleton"},
		literal:   []string{"The raw HTML bold-looking text keeps its words, and a span too.", "A div keeps its text."},
		forbidden: []string{"<br", "<b>", "<script", "alert", "not shown", "textless"}},
	{name: "address-block", template: "neutral", config: "example", checkLogo: true, expectLogo: true,
		ownLine: []string{"Ada Example", "Flat 4, 1 Sample Lane", "12345 Exampleton", "Sampleland"}},
	{name: "nonascii-words", template: "neutral", config: "example", checkLogo: true, expectLogo: true,
		literal: []string{"Błażej Łukasiewicz", "Őrség utca 5", "Çiğdem Öztürk-Weiß", "İstanbul", "Ångström", "Dvořák", "Žižek", "Ærø", "Ñandú",
			"Søren", "Wałęsa", "Szőke", "Straße", "Fuß", "Çelik", "Şahin", "Łódź und Kraków", "Brașov und Tîrgu Mureș", "São Paulo und Curaçao"},
		forbidden: []string{"\uFFFD", "\ufffd"}},
	{name: "literal-dollars", template: "neutral", config: "example", checkLogo: true, expectLogo: true,
		literal: []string{"$5 per bag and $10 for two", "$5.00 against $6.00", "$x$", "$a + b$", "$$y = 2$$", "Escaped $ signs and US$ 1,000",
			"cost = $5 + $6", "total=$(( $a + $b ))", `echo "$HOME"`, "100% of $#1 & {braces} ~tilde^ under_score."}},
	{name: "images-formats", template: "neutral", config: "example", imageFacts: true,
		literal: []string{"Figure 1: PNG chart", "Figure 2: JPEG photo", "Figure 3: WebP banner", "Figure 4: GIF badge"}},
	{name: "images-exif", template: "neutral", config: "example", imageFacts: true, uprightImages: true,
		literal: []string{"Figure 1: Upright photo"}},
	{name: "images-sizes", template: "neutral", config: "example", imageFacts: true,
		literal:   []string{"Figure 1: Natural size", "Figure 2: Width only", "Figure 3: Both sides", "Figure 4: Height only", "image sits in a line of text"},
		forbidden: []string{"=120", "=x200", "inline"}},
	{name: "images-oversize", template: "neutral", config: "example", imageFacts: true, maxImageInches: 8,
		literal: []string{"Figure 1: Natural size, tall", "Figure 2: Requested width"}},
	{name: "images-figure", template: "neutral", config: "example", imageFacts: true,
		literal:   []string{"Figure 1: A chart: stars, under_score & 100% of $5 {braces}", "Column Picture", "Figure 2: In a list"},
		forbidden: []string{"Figure 3", "Linked image", "In a cell"}},
	{name: "images-svg", template: "neutral", config: "example", imageFacts: true, noRaster: true,
		literal:   []string{"Figure 1: A diagram: seed to flower", "Figure 2: In a list", "Seed to flower"},
		forbidden: []string{"=120", "=60x30", "diagram.svg"}},
	{name: "letter-figure", template: "neutral", config: "example", checkLogo: true, expectLogo: true, imageFacts: true,
		literal: []string{"Figure 1: Plants sold: stars, under_score & 100% of $5", "Figure 2: Second chart"}},
	{name: "letter-figure-svg", template: "neutral", config: "example", checkLogo: true, expectLogo: true, imageFacts: true, noRaster: true,
		literal: []string{"Figure 1: A diagram: seed to flower"}},
	{name: "images-svg-embedded", template: "neutral", config: "example", imageFacts: true,
		literal: []string{"Figure 1: Badge with an embedded picture"}},
}

func TestGoldens(t *testing.T) {
	root := repoRoot(t)
	for _, tool := range []string{"pdfinfo", "pdffonts", "pdftotext", "pdftoppm"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s not found: golden tests must run in the outprint image (task test-golden)", tool)
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pdf := render(t, root, tc)

			facts := pdfFacts(t, pdf)
			if tc.imageFacts {
				facts += imageFactsText(t, pdf)
			}
			checkGolden(t, root, tc.name, "facts.txt", facts)
			checkGolden(t, root, tc.name, "text.txt", pdfText(t, pdf))
			assertFontsEmbedded(t, pdf)
			if tc.checkLogo {
				if got := logoDrawn(t, pdf); got != tc.expectLogo {
					t.Errorf("logo drawn in letterhead = %v, want %v", got, tc.expectLogo)
				}
			}
			assertLiteral(t, pdf, tc.literal)
			assertForbidden(t, pdf, tc.forbidden)
			assertOwnLines(t, pdf, tc.ownLine)
			assertPages(t, pdf, tc.minPages)
			assertImageGeometry(t, pdf, tc)
			assertNoRaster(t, pdf, tc.noRaster)
			assertHeaderRepeats(t, pdf, tc.repeatHeader, tc.rowMarker)
			if tc.title != "" {
				if got := field(t, tool(t, "pdfinfo", pdf), "Title"); got != tc.title {
					t.Errorf("PDF title = %q, want %q", got, tc.title)
				}
			}
		})
	}
}

// TestValidateShippedPackages runs `outprint validate` against the packages
// shipped in the image: once as a service would load them (service
// configuration from the image environment), once as the pair an author
// would check.
func TestValidateShippedPackages(t *testing.T) {
	bin := os.Getenv("OUTPRINT_BIN")
	if bin == "" {
		bin = "outprint"
	}
	for name, args := range map[string][]string{
		"service configuration": {"validate"},
		"explicit pair":         {"validate", "--template", "neutral", "--config", "example"},
	} {
		t.Run(name, func(t *testing.T) {
			output, err := exec.Command(bin, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("outprint %v failed: %v\n%s", args, err, output)
			}
			if !strings.Contains(string(output), "validate: ok, 0 warning(s)") {
				t.Fatalf("expected a clean validation, got:\n%s", output)
			}
		})
	}
}

func render(t *testing.T, root string, tc testCase) string {
	t.Helper()
	fixture := filepath.Join(root, "testdata", "fixtures", tc.name)
	out := filepath.Join(t.TempDir(), "out.pdf")
	args := []string{
		"render", "--template", tc.template, "--config", tc.config, "--date", fixedDate,
		"-o", out, filepath.Join(fixture, "input.md"),
	}
	if tc.title != "" {
		args = slices.Insert(args, 1, "--title", tc.title)
	}
	bin := os.Getenv("OUTPRINT_BIN")
	if bin == "" {
		bin = "outprint"
	}
	if output, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
		t.Fatalf("outprint failed: %v\n%s", err, output)
	}
	return out
}

// assertLiteral checks that each string appears in the PDF text exactly as
// written, after layout whitespace is collapsed.
func assertLiteral(t *testing.T, pdf string, want []string) {
	t.Helper()
	text := strings.Join(strings.Fields(pdfText(t, pdf)), " ")
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("PDF text lacks %q", w)
		}
	}
}

func assertForbidden(t *testing.T, pdf string, forbidden []string) {
	t.Helper()
	text := pdfText(t, pdf)
	for _, f := range forbidden {
		if strings.Contains(text, f) {
			t.Errorf("PDF text contains %q", f)
		}
	}
}

// assertOwnLines checks that each string is the whole text of some line of
// the layout text, or a prefix or suffix of it, but never shares a line with
// another string of the list.
func assertOwnLines(t *testing.T, pdf string, want []string) {
	t.Helper()
	lines := strings.Split(pdfText(t, pdf), "\n")
	for _, w := range want {
		found := false
		for _, line := range lines {
			if !strings.Contains(line, w) {
				continue
			}
			found = true
			for _, other := range want {
				if other != w && !strings.Contains(w, other) && strings.Contains(line, other) {
					t.Errorf("%q shares a line with %q: %q", w, other, strings.TrimSpace(line))
				}
			}
		}
		if !found {
			t.Errorf("no line contains %q", w)
		}
	}
}

func assertPages(t *testing.T, pdf string, minPages int) {
	t.Helper()
	if minPages == 0 {
		return
	}
	pages, err := strconv.Atoi(field(t, tool(t, "pdfinfo", pdf), "Pages"))
	if err != nil || pages < minPages {
		t.Errorf("PDF has %d pages (%v), want at least %d", pages, err, minPages)
	}
}

// assertHeaderRepeats checks that every page that shows a table row also
// shows the table header, which proves the header repeats across pages.
func assertHeaderRepeats(t *testing.T, pdf, header, rowMarker string) {
	t.Helper()
	if header == "" {
		return
	}
	rowPages := 0
	for i, page := range strings.Split(pdfText(t, pdf), "\f") {
		flat := strings.Join(strings.Fields(page), " ")
		if !strings.Contains(flat, rowMarker) {
			continue
		}
		rowPages++
		if !strings.Contains(flat, header) {
			t.Errorf("page %d shows table rows but not the header %q", i+1, header)
		}
	}
	if rowPages < 2 {
		t.Errorf("table rows appear on %d page(s), want at least 2", rowPages)
	}
}

func pdfFacts(t *testing.T, pdf string) string {
	t.Helper()
	info := tool(t, "pdfinfo", pdf)
	var lines []string
	lines = append(lines, "pages: "+field(t, info, "Pages"))
	lines = append(lines, "page size: "+field(t, info, "Page size"))
	lines = append(lines, "fonts:")
	lines = append(lines, fontLines(t, pdf)...)
	return strings.Join(lines, "\n") + "\n"
}

var subsetPrefix = regexp.MustCompile(`^[A-Z]{6}\+`)

// fontLines returns "name type embedded" per font, subset prefix removed.
func fontLines(t *testing.T, pdf string) []string {
	t.Helper()
	rows := strings.Split(strings.TrimSpace(tool(t, "pdffonts", pdf)), "\n")
	if len(rows) < 2 {
		t.Fatalf("pdffonts reported no fonts:\n%s", strings.Join(rows, "\n"))
	}
	var fonts []string
	for _, row := range rows[2:] { // header and separator
		cols := strings.Fields(row)
		// name, type (may have spaces), encoding, emb, sub, uni, object, id
		if len(cols) < 8 {
			t.Fatalf("unexpected pdffonts row %q", row)
		}
		name := subsetPrefix.ReplaceAllString(cols[0], "")
		typ := strings.Join(cols[1:len(cols)-6], " ")
		emb := cols[len(cols)-5]
		fonts = append(fonts, fmt.Sprintf("  %s | %s | embedded=%s", name, typ, emb))
	}
	slices.Sort(fonts)
	return fonts
}

func assertFontsEmbedded(t *testing.T, pdf string) {
	t.Helper()
	for _, f := range fontLines(t, pdf) {
		if !strings.HasSuffix(f, "embedded=yes") {
			t.Errorf("font not embedded: %s", f)
		}
	}
}

func pdfText(t *testing.T, pdf string) string {
	t.Helper()
	return tool(t, "pdftotext", "-layout", pdf, "-")
}

// logoDrawn renders page one at low resolution and looks for ink in the
// upper right, where the letterhead puts the logo. A vector logo leaves no
// image object in the PDF, so pixels are the robust signal.
func logoDrawn(t *testing.T, pdf string) bool {
	t.Helper()
	prefix := filepath.Join(t.TempDir(), "page")
	tool(t, "pdftoppm", "-r", "50", "-png", "-f", "1", "-l", "1", "-singlefile", pdf, prefix)
	data, err := os.ReadFile(prefix + ".png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	return hasInk(img, image.Rect(b.Min.X+b.Dx()*55/100, b.Min.Y, b.Max.X, b.Min.Y+b.Dy()*20/100))
}

func hasInk(img image.Image, r image.Rectangle) bool {
	const nearWhite = 0xf000
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			red, green, blue, _ := img.At(x, y).RGBA()
			if red < nearWhite || green < nearWhite || blue < nearWhite {
				return true
			}
		}
	}
	return false
}

func checkGolden(t *testing.T, root, name, file, got string) {
	t.Helper()
	path := filepath.Join(root, "testdata", "golden", name, file)
	if os.Getenv("UPDATE_GOLDENS") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run task update-goldens): %v", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden %s\n--- got ---\n%s\n--- want ---\n%s", file, path, got, want)
	}
}

func tool(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(out)
}

func field(t *testing.T, info, key string) string {
	t.Helper()
	for _, line := range strings.Split(info, "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	t.Fatalf("pdfinfo has no %q", key)
	return ""
}

func repoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("OUTPRINT_REPO_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// pdfImage is one row of `pdfimages -list`.
type pdfImage struct {
	page, width, height int
	kind, colour, enc   string
	xppi, yppi          int
}

func listImages(t *testing.T, pdf string) []pdfImage {
	t.Helper()
	rows := strings.Split(strings.TrimSpace(tool(t, "pdfimages", "-list", pdf)), "\n")
	var out []pdfImage
	for _, row := range rows[min(2, len(rows)):] { // header and separator
		cols := strings.Fields(row)
		// page num type width height color comp bpc enc interp object ID x-ppi y-ppi size ratio
		if len(cols) < 14 {
			t.Fatalf("unexpected pdfimages row %q", row)
		}
		out = append(out, pdfImage{
			page: atoi(t, cols[0]), width: atoi(t, cols[3]), height: atoi(t, cols[4]),
			kind: cols[2], colour: cols[5], enc: cols[8], xppi: atoi(t, cols[12]), yppi: atoi(t, cols[13]),
		})
	}
	return out
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("not a number: %q", s)
	}
	return n
}

// imageFactsText describes the images of a PDF without their pixels.
func imageFactsText(t *testing.T, pdf string) string {
	t.Helper()
	lines := []string{"images:"}
	for _, im := range listImages(t, pdf) {
		lines = append(lines, fmt.Sprintf("  page %d | %s | %d x %d px | %s | %s | %d x %d ppi",
			im.page, im.kind, im.width, im.height, im.colour, im.enc, im.xppi, im.yppi))
	}
	return strings.Join(lines, "\n") + "\n"
}

// assertImageGeometry checks what the goldens only record: images upright
// and inside the page.
func assertImageGeometry(t *testing.T, pdf string, tc testCase) {
	t.Helper()
	for i, im := range listImages(t, pdf) {
		if im.kind != "image" {
			continue
		}
		if tc.uprightImages && im.height <= im.width {
			t.Errorf("image %d is %d x %d px: not upright", i+1, im.width, im.height)
		}
		if tc.maxImageInches > 0 && im.xppi > 0 && im.yppi > 0 {
			w, h := float64(im.width)/float64(im.xppi), float64(im.height)/float64(im.yppi)
			if w > tc.maxImageInches || h > tc.maxImageInches {
				t.Errorf("image %d is placed %.1f x %.1f in, at most %.1f", i+1, w, h, tc.maxImageInches)
			}
		}
	}
}

// assertNoRaster checks that the PDF has no image object: an SVG converted
// to a vector PDF contributes only paths and text.
func assertNoRaster(t *testing.T, pdf string, want bool) {
	t.Helper()
	if !want {
		return
	}
	if images := listImages(t, pdf); len(images) > 0 {
		t.Errorf("the PDF holds %d raster image(s), want none: %v", len(images), images)
	}
}
