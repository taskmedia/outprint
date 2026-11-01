package images

import (
	"bytes"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"

	_ "golang.org/x/image/webp"

	"github.com/taskmedia/outline-latex-pdf/internal/limits"
)

func decodeBack(t *testing.T, c Canonical) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(c.Data))
	if err != nil {
		t.Fatalf("canonical image does not decode: %v", err)
	}
	return img
}

func redAt(t *testing.T, img image.Image, x, y int) bool {
	t.Helper()
	r, g, b, _ := img.At(x, y).RGBA()
	return r>>8 > 150 && g>>8 < 90 && b>>8 < 90
}

func TestCanonicaliseFormats(t *testing.T) {
	webp, err := os.ReadFile("../../testdata/fixtures/images-formats/banner.webp")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		data  []byte
		ext   string
		magic string
	}{
		{"png", pngBytes(t, 40, 30), "png", "\x89PNG"},
		{"jpeg", jpegBytes(t, 40, 30), "jpg", "\xff\xd8\xff"},
		{"gif", gifBytes(t, 40, 30, 1), "png", "\x89PNG"},
		{"webp", webp, "png", "\x89PNG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rej := Canonicalise(tt.data)
			if rej != nil {
				t.Fatalf("rejected: %v", rej)
			}
			if c.Ext != tt.ext || !strings.HasPrefix(string(c.Data), tt.magic) {
				t.Errorf("ext = %s, data starts %q", c.Ext, c.Data[:4])
			}
			b := decodeBack(t, c).Bounds()
			if float64(b.Dx()) != c.Width || float64(b.Dy()) != c.Height || c.Width == 0 {
				t.Errorf("reported %vx%v, decoded %dx%d", c.Width, c.Height, b.Dx(), b.Dy())
			}
		})
	}
}

func TestCanonicaliseOrientation(t *testing.T) {
	for orientation := uint16(1); orientation <= 8; orientation++ {
		for _, big := range []bool{false, true} {
			src := withOrientation(t, jpegBytes(t, 80, 40), orientation, big)
			c, rej := Canonicalise(src)
			if rej != nil {
				t.Fatalf("orientation %d: %v", orientation, rej)
			}
			wantW, wantH := 80, 40
			if orientation >= 5 {
				wantW, wantH = 40, 80
			}
			if c.Width != float64(wantW) || c.Height != float64(wantH) {
				t.Errorf("orientation %d: size %vx%v, want %dx%d", orientation, c.Width, c.Height, wantW, wantH)
			}
			if bytes.Contains(c.Data, []byte("Exif")) {
				t.Errorf("orientation %d: EXIF survived", orientation)
			}
		}
	}
}

// The red quarter starts in the top left. After the turn it is where the
// orientation puts it.
func TestOrientationMovesPixels(t *testing.T) {
	want := map[int][2]int{ // orientation: a pixel inside the red quarter
		1: {5, 5}, 2: {74, 5}, 3: {74, 34}, 4: {5, 34},
		5: {5, 5}, 6: {34, 5}, 7: {34, 74}, 8: {5, 74},
	}
	for orientation, at := range want {
		c, rej := Canonicalise(withOrientation(t, jpegBytes(t, 80, 40), uint16(orientation), false))
		if rej != nil {
			t.Fatal(rej)
		}
		if !redAt(t, decodeBack(t, c), at[0], at[1]) {
			t.Errorf("orientation %d: no red at %v", orientation, at)
		}
	}
}

func TestCanonicalisePNGStaysLossless(t *testing.T) {
	src := pngBytes(t, 33, 21)
	c, rej := Canonicalise(src)
	if rej != nil {
		t.Fatal(rej)
	}
	a, _ := png.Decode(bytes.NewReader(src))
	b := decodeBack(t, c)
	for y := range 21 {
		for x := range 33 {
			if a.At(x, y) != b.At(x, y) {
				t.Fatalf("pixel %d,%d changed", x, y)
			}
		}
	}
}

func TestCanonicaliseRejections(t *testing.T) {
	huge := pngBytes(t, 4, 4)
	setPNGSize(huge, 8000, 8000) // 64 MP in the header of a tiny file
	corruptPNG := pngBytes(t, 40, 30)
	corruptPNG = append(corruptPNG[:len(corruptPNG)/2], make([]byte, 32)...)
	tests := []struct {
		name, kind, contains string
		data                 []byte
	}{
		{"animated gif", KindFormat, "animated GIF", gifBytes(t, 20, 20, 3)},
		{"svg without a namespace", KindSVGRejected, "namespace", []byte("<?xml version=\"1.0\"?>\n<svg/>")},
		{"html sign-in page", KindFormat, "HTML", []byte("<!DOCTYPE html><html></html>")},
		{"pdf", KindFormat, "PDF", []byte("%PDF-1.7 ...")},
		{"bmp", KindFormat, "BMP", []byte("BM......")},
		{"empty", KindFormat, "empty", nil},
		{"wrong magic", KindFormat, "unknown", []byte("not an image at all")},
		{"pixel bomb", KindPixels, "64 megapixels", huge},
		{"corrupt png", KindDecode, "could not be decoded", corruptPNG},
		{"truncated jpeg", KindDecode, "could not be decoded", jpegBytes(t, 40, 30)[:len(jpegBytes(t, 40, 30))-60]},
		{"jpeg header only", KindDecode, "header is not valid", jpegBytes(t, 40, 30)[:100]},
		{"bad webp", KindDecode, "webp", []byte("RIFF\x04\x00\x00\x00WEBPVP8 ")},
		{"animated webp", KindFormat, "animated WebP", animatedWebP()},
		{"zero size png", KindDecode, "header", zeroSizePNG(t)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, rej := Canonicalise(tt.data)
			if rej == nil {
				t.Fatal("accepted")
			}
			if rej.Kind != tt.kind || !strings.Contains(rej.Message, tt.contains) {
				t.Errorf("got %s: %s, want %s containing %q", rej.Kind, rej.Message, tt.kind, tt.contains)
			}
		})
	}
}

func TestCanonicaliseTooLargeAfterConversion(t *testing.T) {
	// Noise does not compress, so the PNG is far larger than the limit.
	side := 2500
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	state := uint32(1)
	for i := range img.Pix {
		state = state*1664525 + 1013904223
		img.Pix[i] = byte(state >> 24)
	}
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	_, rej := Canonicalise(b.Bytes())
	if rej == nil || rej.Kind != KindTooLarge {
		t.Fatalf("got %v, want too_large", rej)
	}
	if limits.MaxImageBytes != 10<<20 {
		t.Fatal("limit changed")
	}
}

func TestAtPixelLimitIsAccepted(t *testing.T) {
	// 40 MP exactly (8000 x 5000) in the header is allowed; the header
	// check alone is what is exercised, a decode would need real data.
	data := pngBytes(t, 4, 4)
	setPNGSize(data, 8000, 5001)
	if rej := checkHeader(formatPNG, data); rej == nil || rej.Kind != KindPixels {
		t.Errorf("40.005 MP: %v", rej)
	}
	setPNGSize(data, 8000, 5000)
	if rej := checkHeader(formatPNG, data); rej != nil {
		t.Errorf("40 MP: %v", rej)
	}
}

func TestGIFFrameCounter(t *testing.T) {
	for frames := 1; frames <= 4; frames++ {
		if got := gifFrames(gifBytes(t, 10, 10, frames)); got != frames {
			t.Errorf("%d frames counted as %d", frames, got)
		}
	}
	if gifFrames([]byte("GIF89a")) != 0 || gifFrames(nil) != 0 {
		t.Error("short input")
	}
	cut := gifBytes(t, 10, 10, 3)
	for i := range cut {
		gifFrames(cut[:i]) // must not panic on any truncation
	}
}

func TestJPEGOrientationIgnoresGarbage(t *testing.T) {
	base := jpegBytes(t, 10, 10)
	good := withOrientation(t, base, 6, false)
	for i := range good {
		jpegOrientation(good[:i]) // must not panic on any truncation
	}
	if jpegOrientation(withOrientation(t, base, 9, false)) != 1 {
		t.Error("orientation 9 accepted")
	}
	if jpegOrientation(base) != 1 {
		t.Error("no EXIF should be 1")
	}
}

// setPNGSize rewrites the width and height in the IHDR chunk and its CRC.
func setPNGSize(data []byte, w, h uint32) {
	putBE32(data[16:], w)
	putBE32(data[20:], h)
	putBE32(data[29:], crc32.ChecksumIEEE(data[12:29]))
}

func putBE32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
}

func zeroSizePNG(t *testing.T) []byte {
	d := pngBytes(t, 4, 4)
	setPNGSize(d, 0, 0)
	return d
}

func animatedWebP() []byte {
	d := []byte("RIFF\x16\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00")
	return append(d, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0)
}
