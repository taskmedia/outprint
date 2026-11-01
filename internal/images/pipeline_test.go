package images

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/taskmedia/outline-latex-pdf/internal/limits"
)

func process(t *testing.T, src Source, body string) (Result, []Problem) {
	t.Helper()
	res, problems, err := Process(context.Background(), []byte(body), 0, src)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	return res, problems
}

func TestProcessRewritesImages(t *testing.T) {
	src := newFakeSource()
	src.files["a"] = pngBytes(t, 400, 200)
	src.files["b"] = jpegBytes(t, 100, 100)
	body := "Intro\n\n![Chart *one*](a \"=200x\")\n\nText ![](b) and ![again](a).\n"
	res, problems := process(t, src, body)
	if len(problems) > 0 {
		t.Fatalf("problems: %v", problems)
	}
	want := "Intro\n\n![Chart one](images/img-1.png){width=150.00pt height=75.00pt}\n\n" +
		"Text ![](images/img-2.jpg){width=75.00pt height=75.00pt} and ![again](images/img-1.png){width=300.00pt height=150.00pt}.\n"
	if string(res.Markdown) != want {
		t.Errorf("markdown:\n%s\nwant:\n%s", res.Markdown, want)
	}
	if len(res.Images) != 2 || len(res.Images["img-1.png"]) == 0 || len(res.Images["img-2.jpg"]) == 0 {
		t.Errorf("images = %v", keys(res.Images))
	}
	if len(src.fetched) != 2 {
		t.Errorf("fetched %v: a duplicate must be fetched once", src.fetched)
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestProcessWithoutImagesLeavesBodyAlone(t *testing.T) {
	body := "# Title\n\nNo pictures, just `![a](b)` in code.\n"
	res, problems := process(t, newFakeSource(), body)
	if len(problems) != 0 || string(res.Markdown) != body || len(res.Images) != 0 {
		t.Errorf("got %q %v %v", res.Markdown, problems, keys(res.Images))
	}
}

func TestProcessGeneratedNamesMatchBundleRule(t *testing.T) {
	re := regexp.MustCompile(`^img-[0-9]+\.(png|jpg)$`)
	src := newFakeSource()
	var body strings.Builder
	for i := range 12 {
		key := fmt.Sprintf("k%d", i)
		src.files[key] = pngBytes(t, 8, 8+i)
		fmt.Fprintf(&body, "![x](%s)\n\n", key)
	}
	res, problems := process(t, src, body.String())
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	for name := range res.Images {
		if !re.MatchString(name) {
			t.Errorf("name %q", name)
		}
	}
}

func TestProcessCollectsAllProblems(t *testing.T) {
	src := newFakeSource()
	src.files["ok"] = pngBytes(t, 10, 10)
	src.files["svg"] = []byte("<svg/>")
	src.files["gif"] = gifBytes(t, 10, 10, 2)
	src.files["bad"] = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 40)...)
	src.errs["denied"] = ErrForbidden
	src.errs["huge"] = ErrTooLarge
	body := strings.Join([]string{
		"![fine](ok)",
		"![missing one](nowhere)",
		"![ext](https://cdn.example.invalid/x.png)",
		"![denied pic](denied)",
		"![huge pic](huge)",
		"![svg pic](svg)",
		"![gif pic](gif)",
		"![bad pic](bad)",
		"![](ok)  ![no address]()",
	}, "\n\n")
	res, problems, err := Process(context.Background(), []byte(body), 10, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 0 || res.Markdown != nil {
		t.Error("a result with problems must be empty")
	}
	var got []string
	for _, p := range problems {
		got = append(got, fmt.Sprintf("%d:%d %s %q", p.Line, p.Col, p.Kind, p.Alt))
	}
	want := []string{
		`13:1 missing "missing one"`, `15:1 forbidden "ext"`, `17:1 forbidden "denied pic"`, `19:1 too_large "huge pic"`,
		`21:1 format "svg pic"`, `23:1 format "gif pic"`, `25:1 decode "bad pic"`, `27:10 missing "no address"`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestProblemErrorEntry(t *testing.T) {
	e := Problem{Kind: KindFormat, Message: "bad", Line: 7, Col: 3, Alt: strings.Repeat("ä", 60)}.Error()
	if e.Source != "document" || e.Code != "format" || *e.Line != 7 || e.Message != "bad (line 7, column 3)" {
		t.Errorf("%+v", e)
	}
	if n := len([]rune(e.Field)); n != altTruncate {
		t.Errorf("field has %d characters", n)
	}
	if got := (Problem{Kind: KindMissing}.Error()).Field; got != "image" {
		t.Errorf("field without alt = %q", got)
	}
}

func TestProcessSourceFailureAborts(t *testing.T) {
	src := newFakeSource()
	src.errs["x"] = errors.New("outline down")
	_, _, err := Process(context.Background(), []byte("![a](x)"), 0, src)
	if err == nil {
		t.Fatal("an infrastructure failure must abort")
	}
}

func TestProcessRepeatedProblemIsReportedEachTime(t *testing.T) {
	src := newFakeSource()
	_, problems := process(t, src, "![a](gone)\n\n![b](gone)\n")
	if len(problems) != 2 || len(src.fetched) != 1 {
		t.Errorf("problems %d, fetched %v", len(problems), src.fetched)
	}
}

func TestProcessImageCountLimit(t *testing.T) {
	src := newFakeSource()
	var body strings.Builder
	for i := range limits.MaxImages + 2 {
		key := fmt.Sprintf("k%d", i)
		src.files[key] = pngBytes(t, 4, 4+i)
		fmt.Fprintf(&body, "![n%d](%s)\n\n", i, key)
	}
	_, problems := process(t, src, body.String())
	if len(problems) != 2 || problems[0].Kind != KindTooLarge || !strings.Contains(problems[0].Message, "more than 50 images") {
		t.Errorf("problems = %+v", problems)
	}
	if len(src.fetched) != limits.MaxImages {
		t.Errorf("fetched %d images, want %d", len(src.fetched), limits.MaxImages)
	}
}

func TestProcessFiftyImagesAreAllowed(t *testing.T) {
	src := newFakeSource()
	var body strings.Builder
	for i := range limits.MaxImages {
		key := fmt.Sprintf("k%d", i)
		src.files[key] = pngBytes(t, 4, 4+i)
		fmt.Fprintf(&body, "![n](%s)\n\n", key)
	}
	res, problems := process(t, src, body.String())
	if len(problems) != 0 || len(res.Images) != limits.MaxImages {
		t.Errorf("problems %v, images %d", problems, len(res.Images))
	}
}

func TestProcessSourceTooLarge(t *testing.T) {
	src := newFakeSource()
	src.files["big"] = make([]byte, limits.MaxImageBytes+1)
	_, problems := process(t, src, "![a](big)")
	if len(problems) != 1 || problems[0].Kind != KindTooLarge {
		t.Errorf("problems = %+v", problems)
	}
}

func TestProcessTotalLimit(t *testing.T) {
	src := newFakeSource()
	// Each canonical image is about 9 MiB: noise that does not compress.
	for i := range 6 {
		src.files[fmt.Sprintf("n%d", i)] = noisePNG(t, 1500, uint32(i+1))
	}
	var body strings.Builder
	for i := range 6 {
		fmt.Fprintf(&body, "![n%d](n%d)\n\n", i, i)
	}
	_, problems := process(t, src, body.String())
	if len(problems) == 0 {
		t.Fatal("six large images must exceed the total")
	}
	for _, p := range problems {
		if p.Kind != KindTooLarge || !strings.Contains(p.Message, "together") {
			t.Errorf("problem %+v", p)
		}
	}
}

func TestProcessRejectsMultiLineImage(t *testing.T) {
	_, problems := process(t, newFakeSource(), "![a\nb](x)")
	if len(problems) != 1 || problems[0].Kind != KindSyntax {
		t.Errorf("problems = %+v", problems)
	}
}

func TestRewriteIsHostileSafe(t *testing.T) {
	src := newFakeSource()
	src.files["a"] = pngBytes(t, 20, 10)
	hostile := []string{
		`x}{width=9999pt height=1pt}{.evil}`,
		`"quoted" <b>bold</b> $x$ \input{/etc/passwd} [link](http://evil.example.invalid)`,
		"line\nbreak\r\nand\ttab",
		"`code` ![nested](http://evil.example.invalid/x.png) {#id}",
		"\x00ctrl\x1b[0m end   sep",
		`](http://evil.example.invalid) trailing \`,
	}
	for _, alt := range hostile {
		body := "![" + strings.ReplaceAll(strings.ReplaceAll(alt, "\n", " "), "\r", " ") + "](a)\n"
		res, problems := process(t, src, body)
		if len(problems) > 0 || len(res.Images) == 0 {
			continue // no image here, or a parse the pipeline refuses
		}
		line := strings.TrimSuffix(string(res.Markdown), "\n")
		re := regexp.MustCompile(`^!\[((?:[^\\\[\]\n]|\\.)*)\]\(images/img-1\.png\)\{width=[0-9.]+pt height=[0-9.]+pt\}$`)
		if !re.MatchString(line) {
			t.Errorf("alt %q produced %q", alt, line)
		}
		for _, bad := range []string{"\x00", "\x1b", " "} {
			if strings.Contains(line, bad) {
				t.Errorf("alt %q leaked %q", alt, bad)
			}
		}
	}
}

func TestRewriteSwallowsAuthorAttributes(t *testing.T) {
	src := newFakeSource()
	src.files["a"] = pngBytes(t, 20, 10)
	res, _ := process(t, src, `![a](a){width=9999pt #id .cls}{.next} and {curly} text`+"\n")
	want := "![a](images/img-1.png){width=15.00pt height=7.50pt}{.next} and {curly} text\n"
	if string(res.Markdown) != want {
		t.Errorf("got %q want %q", res.Markdown, want)
	}
}

func TestRewriteHostileTitleAndDestination(t *testing.T) {
	src := newFakeSource()
	src.files["a"] = pngBytes(t, 20, 10)
	res, problems := process(t, src, `![a](a "x}{width=1pt} =10x10")`+"\n")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	if string(res.Markdown) != "![a](images/img-1.png){width=7.50pt height=7.50pt}\n" {
		t.Errorf("got %q", res.Markdown)
	}
	// A hostile destination is never passed on.
	_, problems = process(t, src, "![a](https://evil.example.invalid/x?a=%22%20onerror=%22alert%281%29)\n")
	if len(problems) == 0 {
		t.Error("external destination accepted")
	}
}

func TestRewriteInTableAndQuote(t *testing.T) {
	src := newFakeSource()
	src.files["a"] = pngBytes(t, 20, 10)
	res, problems := process(t, src, "| h |\n|---|\n| ![a\\|b](a) |\n\n> ![q](a)\n")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	if !strings.Contains(string(res.Markdown), `| ![a\|b](images/img-1.png){width=15.00pt height=7.50pt} |`) ||
		!strings.Contains(string(res.Markdown), "> ![q](images/img-1.png)") {
		t.Errorf("got %q", res.Markdown)
	}
}

func TestRewriteReferenceStyle(t *testing.T) {
	src := newFakeSource()
	src.files["a"] = pngBytes(t, 20, 10)
	res, problems := process(t, src, "Look ![pic][r].\n\n[r]: a \"=40x\"\n")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	if !strings.HasPrefix(string(res.Markdown), "Look ![pic](images/img-1.png){width=30.00pt height=15.00pt}.") {
		t.Errorf("got %q", res.Markdown)
	}
}
