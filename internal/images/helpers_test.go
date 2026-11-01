package images

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

// pattern is a w by h image whose top-left quarter is red, so that a turned
// or mirrored copy can be told from the original.
func pattern(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.NRGBA{R: 20, G: 120, B: 200, A: 255}
			if x < w/2 && y < h/2 {
				c = color.NRGBA{R: 220, G: 30, B: 30, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func pngBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, pattern(w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, pattern(w, h), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gifBytes(t testing.TB, w, h, frames int) []byte {
	t.Helper()
	pal := color.Palette{color.White, color.Black, color.RGBA{R: 220, G: 30, B: 30, A: 255}}
	g := &gif.GIF{}
	for range frames {
		fr := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		for y := range h / 2 {
			for x := range w / 2 {
				fr.SetColorIndex(x, y, 2)
			}
		}
		g.Image = append(g.Image, fr)
		g.Delay = append(g.Delay, 5)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withOrientation inserts an EXIF APP1 segment with the orientation after
// the SOI marker of a JPEG.
func withOrientation(t testing.TB, jpg []byte, orientation uint16, big bool) []byte {
	t.Helper()
	var order binary.AppendByteOrder = binary.LittleEndian
	tag := "II"
	if big {
		order, tag = binary.BigEndian, "MM"
	}
	tiff := []byte(tag)
	tiff = order.AppendUint16(tiff, 42)
	tiff = order.AppendUint32(tiff, 8)
	tiff = order.AppendUint16(tiff, 1)      // one entry
	tiff = order.AppendUint16(tiff, 0x0112) // orientation
	tiff = order.AppendUint16(tiff, 3)      // SHORT
	tiff = order.AppendUint32(tiff, 1)      // count
	tiff = order.AppendUint16(tiff, orientation)
	tiff = append(tiff, 0, 0)          // value padding
	tiff = order.AppendUint32(tiff, 0) // no next IFD
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xff, 0xe1}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	seg = append(seg, payload...)
	return append(append(append([]byte{}, jpg[:2]...), seg...), jpg[2:]...)
}

// fakeSource serves images by key and records every fetch.
type fakeSource struct {
	files   map[string][]byte
	errs    map[string]error
	fetched []string
}

func newFakeSource() *fakeSource {
	return &fakeSource{files: map[string][]byte{}, errs: map[string]error{}}
}

func (f *fakeSource) Locate(address string) (string, *Rejection) {
	switch {
	case address == "":
		return "", reject(KindMissing, "the image has no address")
	case len(address) > 4 && address[:4] == "http":
		return "", reject(KindForbidden, "images from other sites are not allowed; %s", uploadAdvice)
	}
	return address, nil
}

func (f *fakeSource) Fetch(_ context.Context, key string) ([]byte, error) {
	f.fetched = append(f.fetched, key)
	if err := f.errs[key]; err != nil {
		return nil, err
	}
	data, ok := f.files[key]
	if !ok {
		return nil, ErrNotFound
	}
	return data, nil
}

// noisePNG is a side x side PNG of pseudo-random pixels, which does not
// compress and so stays large.
func noisePNG(t testing.TB, side int, seed uint32) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	state := seed
	for i := range img.Pix {
		state = state*1664525 + 1013904223
		img.Pix[i] = byte(state >> 24)
	}
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
