//go:build golden

// Tier-2 tests: render fixtures with the real renderer binary and compare
// PDF facts and extracted text with checked-in goldens. PDF bytes are never
// compared. These tests are meant to run inside the pinned renderer image
// (see the golden target in the Dockerfile and `make test-golden`).
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
	"strings"
	"testing"
)

type testCase struct {
	name         string // fixture topic under testdata/fixtures
	template     string
	config       string
	documentType string
	expectLogo   bool
}

var cases = []testCase{
	{name: "letter-basic", template: "neutral", config: "example", documentType: "letter", expectLogo: true},
}

func TestGoldens(t *testing.T) {
	root := repoRoot(t)
	for _, tool := range []string{"pdfinfo", "pdffonts", "pdftotext", "pdftoppm"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s not found: golden tests must run in the renderer image (make test-golden)", tool)
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pdf := render(t, root, tc)

			facts := pdfFacts(t, pdf)
			checkGolden(t, root, tc.name, "facts.txt", facts)
			checkGolden(t, root, tc.name, "text.txt", pdfText(t, pdf))
			assertFontsEmbedded(t, pdf)
			if got := logoDrawn(t, pdf); got != tc.expectLogo {
				t.Errorf("logo drawn in letterhead = %v, want %v", got, tc.expectLogo)
			}
		})
	}
}

// TestValidateShippedPackages runs `renderer validate` against the packages
// shipped in the image: once as a service would load them (service
// configuration from the image environment), once as the pair an author
// would check.
func TestValidateShippedPackages(t *testing.T) {
	bin := os.Getenv("RENDERER_BIN")
	if bin == "" {
		bin = "renderer"
	}
	for name, args := range map[string][]string{
		"service configuration": {"validate"},
		"explicit pair":         {"validate", "--template", "neutral", "--config", "example"},
	} {
		t.Run(name, func(t *testing.T) {
			output, err := exec.Command(bin, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("renderer %v failed: %v\n%s", args, err, output)
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
		"render", "--template", tc.template, "--config", tc.config, "--type", tc.documentType,
		"-o", out, filepath.Join(fixture, "input.md"),
	}
	if fields := filepath.Join(fixture, "fields.yaml"); fileExists(fields) {
		args = slices.Insert(args, 1, "--fields", fields)
	}
	bin := os.Getenv("RENDERER_BIN")
	if bin == "" {
		bin = "renderer"
	}
	if output, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
		t.Fatalf("renderer failed: %v\n%s", err, output)
	}
	return out
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
		t.Fatalf("read golden (run make update-goldens): %v", err)
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
	if root := os.Getenv("RENDERER_REPO_ROOT"); root != "" {
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
