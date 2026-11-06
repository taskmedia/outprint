package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRsvgArgsNeverUnlimitedAndReadStdin(t *testing.T) {
	args := rsvgArgs()
	for _, a := range args {
		if a == "-u" || a == "--unlimited" {
			t.Fatalf("args %v lift the librsvg limits", args)
		}
	}
	if !slices.Contains(args, "pdf") || args[len(args)-1] != "-" {
		t.Errorf("args = %v, want a PDF conversion that reads standard input", args)
	}
}

func TestBuildSVGEnvIsRebuiltFromScratch(t *testing.T) {
	host := map[string]string{"XML_CATALOG_FILES": "/evil.xml", "HTTP_PROXY": "http://proxy", "SECRET_TOKEN": "hunter2", "SOURCE_DATE_EPOCH": "1700000000"}
	env := buildSVGEnv("/job", func(k string) (string, bool) { v, ok := host[k]; return v, ok })
	want := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LC_ALL=C.UTF-8", "HOME=/job/home", "TMPDIR=/job", "SOURCE_DATE_EPOCH=1700000000"}
	if !slices.Equal(env, want) {
		t.Errorf("env = %v, want %v", env, want)
	}
}

func TestSVGNamesAndConvertedNames(t *testing.T) {
	images := map[string][]byte{"img-2.svg": nil, "img-1.png": nil, "img-3.SVG": nil, "img-4.jpg": nil}
	if got := svgNames(images); !slices.Equal(got, []string{"img-2.svg", "img-3.SVG"}) {
		t.Errorf("svgNames = %v", got)
	}
	if got := pdfNameFor("img-3.SVG"); got != "img-3.pdf" {
		t.Errorf("pdfNameFor = %q", got)
	}
}

func TestFirstLineIsShortAndPlain(t *testing.T) {
	got := firstLine("\n  Error \x1b[31mreading\x00 " + strings.Repeat("x", 500) + "\nsecond line")
	if strings.ContainsAny(got, "\x1b\x00\n") || len(got) > maxLoggedLine || strings.Contains(got, "second") {
		t.Errorf("firstLine = %q", got)
	}
}

func TestWriteImagesSkipsSVG(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, imagesDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeImages(dir, map[string][]byte{"img-1.png": {1}, "img-2.svg": {2}}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, imagesDirName))
	if len(entries) != 1 || entries[0].Name() != "img-1.png" {
		t.Errorf("job images = %v", entries)
	}
}

// fakeConverter puts a shell script named rsvg-convert first on PATH and
// returns the job directory to convert in.
func fakeConverter(t *testing.T, script string) string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "rsvg-convert"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	dir := t.TempDir()
	for _, d := range []string{homeDir, imagesDirName} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func svgPipeline(dir string, timeout time.Duration, maxPDF int) *pipeline {
	return &pipeline{
		id: "job1", dir: dir, ctx: context.Background(), timeout: time.Minute,
		log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		svgTimeout: timeout, svgMaxPDF: maxPDF,
	}
}

func TestConvertSVGWritesThePDF(t *testing.T) {
	dir := fakeConverter(t, "cat >/dev/null; printf '%%PDF-1.7 fake'\n")
	dst := filepath.Join(dir, "images", "a.pdf")
	if err := svgPipeline(dir, 5*time.Second, 1<<20).convertSVG([]byte("<svg/>"), dst); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "%PDF-1.7 fake" {
		t.Errorf("converted file = %q", data)
	}
}

func TestConvertSVGFailures(t *testing.T) {
	tests := []struct {
		name, script string
		timeout      time.Duration
		maxPDF       int
		kind         Kind
	}{
		{"exit code", "cat >/dev/null; echo 'boom' >&2; exit 1\n", 5 * time.Second, 1 << 20, KindSVGConvert},
		{"not a pdf", "cat >/dev/null; echo hello\n", 5 * time.Second, 1 << 20, KindSVGConvert},
		{"nothing written", "cat >/dev/null\n", 5 * time.Second, 1 << 20, KindSVGConvert},
		{"too large", "cat >/dev/null; printf '%%PDF'; head -c 3000000 /dev/zero\n", 5 * time.Second, 1 << 20, KindOutputTooLarge},
		{"too slow", "sleep 30\n", 500 * time.Millisecond, 1 << 20, KindTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := fakeConverter(t, tt.script)
			start := time.Now()
			err := svgPipeline(dir, tt.timeout, tt.maxPDF).convertSVG([]byte("<svg/>"), filepath.Join(dir, "images", "a.pdf"))
			var we *Error
			if !errors.As(err, &we) || we.Phase != PhaseSVG || we.Kind != tt.kind || we.JobID != "job1" {
				t.Fatalf("error = %v, want phase svg kind %s", err, tt.kind)
			}
			if strings.Contains(we.Error(), "boom") || time.Since(start) > 10*time.Second {
				t.Errorf("error %q or duration %s", we, time.Since(start))
			}
			if _, statErr := os.Stat(filepath.Join(dir, "images", "a.pdf")); statErr == nil {
				t.Error("a PDF was written for a failed conversion")
			}
		})
	}
}

func TestConvertSVGKillsTheWholeProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "alive")
	dir := fakeConverter(t, "(sleep 2; touch "+marker+") &\nsleep 30\n")
	err := svgPipeline(dir, 300*time.Millisecond, 1<<20).convertSVG([]byte("<svg/>"), filepath.Join(dir, "images", "a.pdf"))
	if err == nil {
		t.Fatal("expected a timeout")
	}
	time.Sleep(3 * time.Second)
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("a child of the converter outlived it")
	}
}

func TestJobTimeoutDuringConversionIsAJobTimeout(t *testing.T) {
	dir := fakeConverter(t, "sleep 30\n")
	p := svgPipeline(dir, 20*time.Second, 1<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	p.ctx = ctx
	err := p.convertSVG([]byte("<svg/>"), filepath.Join(dir, "images", "a.pdf"))
	var we *Error
	if !errors.As(err, &we) || we.Phase != PhaseTimeout || we.Kind != KindTimeout {
		t.Fatalf("error = %v, want the job's timeout", err)
	}
}

func TestCallerCancelDuringConversionIsCanceledNotTimeout(t *testing.T) {
	dir := fakeConverter(t, "sleep 30\n")
	p := svgPipeline(dir, 20*time.Second, 1<<20)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	p.ctx = ctx
	err := p.convertSVG([]byte("<svg/>"), filepath.Join(dir, "images", "a.pdf"))
	var we *Error
	if !errors.As(err, &we) || we.Kind != KindCanceled {
		t.Fatalf("error = %v, want a canceled job", err)
	}
}

func TestConvertSVGRefusesAnOversizeSource(t *testing.T) {
	dir := fakeConverter(t, "cat >/dev/null; printf '%%PDF'\n")
	err := svgPipeline(dir, time.Second, 1<<20).convertSVG(make([]byte, 3<<20), filepath.Join(dir, "images", "a.pdf"))
	var we *Error
	if !errors.As(err, &we) || we.Kind != KindInvalidJob {
		t.Fatalf("error = %v", err)
	}
}
