package images

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/webp"

	"github.com/taskmedia/outline-latex-pdf/internal/limits"
)

const (
	jpegQuality = 95
	extPNG      = "png"
	extJPEG     = "jpg"
)

// Formats by magic bytes.
const (
	formatPNG     = "png"
	formatJPEG    = "jpeg"
	formatGIF     = "gif"
	formatWebP    = "webp"
	formatSVG     = "svg"
	formatUnknown = "unknown"
)

const uploadHint = "use PNG, JPEG, WebP, a static GIF or an SVG"

// Canonical is an image ready for the bundle.
type Canonical struct {
	Data []byte
	Ext  string
	// Width and Height are the natural size in CSS pixels: the pixels of a
	// raster image, the declared or viewBox size of an SVG.
	Width, Height float64
}

// Canonicalise detects the format by magic bytes, checks the pixel count on
// the header before decoding, decodes, applies the EXIF orientation of a
// JPEG and re-encodes: PNG lossless, JPEG at quality 95, WebP and GIF as
// PNG. Metadata is dropped because the encoders write none.
func Canonicalise(data []byte) (Canonical, *Rejection) {
	switch f := detectFormat(data); f {
	case formatPNG, formatJPEG, formatGIF, formatWebP:
		return canonicaliseRaster(f, data)
	case formatSVG:
		return canonicaliseSVG(data)
	default:
		return Canonical{}, reject(KindFormat, "this is not a supported image (detected: %s); %s", describeUnknown(data), uploadHint)
	}
}

func canonicaliseRaster(format string, data []byte) (c Canonical, rej *Rejection) {
	defer func() {
		if recover() != nil {
			c, rej = Canonical{}, reject(KindDecode, "the %s image could not be decoded", format)
		}
	}()
	if rej := checkHeader(format, data); rej != nil {
		return Canonical{}, rej
	}
	img, err := decodeRaster(format, data)
	if err != nil {
		return Canonical{}, reject(KindDecode, "the %s image could not be decoded", format)
	}
	if format == formatJPEG {
		img = applyOrientation(img, jpegOrientation(data))
	}
	return encode(format, img)
}

func encode(format string, img image.Image) (Canonical, *Rejection) {
	var buf bytes.Buffer
	ext := extPNG
	var err error
	if format == formatJPEG {
		ext = extJPEG
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	}
	if err != nil {
		return Canonical{}, reject(KindDecode, "the %s image could not be re-encoded", format)
	}
	if buf.Len() > limits.MaxImageBytes {
		return Canonical{}, reject(KindTooLarge, "the image is %s after conversion, at most %s", mib(buf.Len()), mib(limits.MaxImageBytes))
	}
	b := img.Bounds()
	return Canonical{Data: buf.Bytes(), Ext: ext, Width: float64(b.Dx()), Height: float64(b.Dy())}, nil
}

func mib(n int) string { return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20)) }

// checkHeader rejects animated images and images with too many pixels
// without decoding any pixel.
func checkHeader(format string, data []byte) *Rejection {
	if rej := rejectAnimated(format, data); rej != nil {
		return rej
	}
	cfg, err := decodeConfig(format, data)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return reject(KindDecode, "the %s image header is not valid", format)
	}
	if int64(cfg.Width)*int64(cfg.Height) > limits.MaxImagePixels {
		return reject(KindPixels, "the image is %d x %d pixels (%d megapixels), at most %d megapixels",
			cfg.Width, cfg.Height, int64(cfg.Width)*int64(cfg.Height)/1_000_000, limits.MaxImagePixels/1_000_000)
	}
	return nil
}

func rejectAnimated(format string, data []byte) *Rejection {
	switch {
	case format == formatGIF && gifFrames(data) > 1:
		return reject(KindFormat, "animated GIF is not supported; %s", uploadHint)
	case format == formatWebP && isAnimatedWebP(data):
		return reject(KindFormat, "animated WebP is not supported; %s", uploadHint)
	}
	return nil
}

func decodeConfig(format string, data []byte) (image.Config, error) {
	r := bytes.NewReader(data)
	switch format {
	case formatPNG:
		return png.DecodeConfig(r)
	case formatJPEG:
		return jpeg.DecodeConfig(r)
	case formatGIF:
		return gif.DecodeConfig(r)
	default:
		return webp.DecodeConfig(r)
	}
}

func decodeRaster(format string, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch format {
	case formatPNG:
		return png.Decode(r)
	case formatJPEG:
		return jpeg.Decode(r)
	case formatGIF:
		return gif.Decode(r)
	default:
		return webp.Decode(r)
	}
}

// detectFormat names a format by its first bytes.
func detectFormat(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return formatPNG
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return formatJPEG
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return formatGIF
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return formatWebP
	case looksLikeSVG(data):
		return formatSVG
	}
	return formatUnknown
}

// looksLikeSVG says whether data is an SVG candidate: markup that is not an
// HTML page and names an svg element near its start, or UTF-16 text, which
// the pre-check then refuses with a clear message. Whether the XML root is
// really svg is decided by the pre-check, not here.
func looksLikeSVG(data []byte) bool {
	if isUTF16(data) {
		return true
	}
	head := bytes.TrimLeft(bytes.TrimPrefix(data[:min(len(data), 2048)], []byte(utf8BOM)), " \t\r\n")
	if len(head) == 0 || head[0] != '<' {
		return false
	}
	lower := bytes.ToLower(head)
	if bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html")) {
		return false
	}
	return bytes.Contains(lower, []byte("<svg")) || bytes.Contains(lower, []byte(":svg")) ||
		bytes.Contains(lower, []byte("<!doctype svg"))
}

// describeUnknown names a format that is not supported, for the message.
func describeUnknown(data []byte) string {
	head := bytes.TrimLeft(data[:min(len(data), 512)], " \t\r\n")
	lower := bytes.ToLower(head)
	switch {
	case bytes.HasPrefix(lower, []byte("<!doctype html")), bytes.HasPrefix(lower, []byte("<html")):
		return "HTML, perhaps a sign-in page"
	case bytes.HasPrefix(head, []byte("%PDF")):
		return "PDF"
	case bytes.HasPrefix(head, []byte("BM")):
		return "BMP"
	case bytes.HasPrefix(head, []byte("II*\x00")), bytes.HasPrefix(head, []byte("MM\x00*")):
		return "TIFF"
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		return "HEIF or AVIF"
	case bytes.HasPrefix(head, []byte("\x00\x00\x01\x00")):
		return "ICO"
	case len(data) == 0:
		return "an empty file"
	}
	return "unknown"
}

// isAnimatedWebP reads the animation flag of the VP8X chunk.
func isAnimatedWebP(data []byte) bool {
	const flagsAt, animation = 20, 0x02
	return len(data) > flagsAt && string(data[12:16]) == "VP8X" && data[flagsAt]&animation != 0
}

// gifFrames counts the image descriptors of a GIF by walking its blocks,
// without decoding any frame, so a GIF with many frames costs nothing.
func gifFrames(data []byte) int {
	const headerLen, screenLen = 6, 7
	if len(data) < headerLen+screenLen {
		return 0
	}
	pos := headerLen + screenLen
	pos += colorTableLen(data[headerLen+4])
	frames := 0
	for pos < len(data) {
		switch data[pos] {
		case 0x21: // extension: introducer, label, sub-blocks
			pos = skipSubBlocks(data, pos+2)
		case 0x2c: // image descriptor
			frames++
			if pos+10 > len(data) {
				return frames
			}
			pos += 10 + colorTableLen(data[pos+9])
			pos = skipSubBlocks(data, pos+1) // LZW minimum code size, then data
		default:
			return frames
		}
	}
	return frames
}

// colorTableLen is the size in bytes of the color table announced by a
// packed field, which is 0 when the table flag is not set.
func colorTableLen(packed byte) int {
	if packed&0x80 == 0 {
		return 0
	}
	return 3 * (1 << (int(packed&0x07) + 1))
}

func skipSubBlocks(data []byte, pos int) int {
	for pos < len(data) {
		n := int(data[pos])
		pos += 1 + n
		if n == 0 {
			break
		}
	}
	return pos
}
