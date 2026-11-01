package images

import (
	"encoding/binary"
	"image"
	"image/color"
)

const (
	tagOrientation = 0x0112
	typeShort      = 3
	ifdEntryLen    = 12
)

// jpegOrientation reads the EXIF orientation (1 to 8) of a JPEG, or 1 when
// there is none or it is malformed.
func jpegOrientation(data []byte) int {
	pos := 2 // after SOI
	for pos+4 <= len(data) && data[pos] == 0xff {
		marker := data[pos+1]
		if marker == 0xda || marker == 0xd9 { // SOS, EOI: no more headers
			return 1
		}
		length := int(binary.BigEndian.Uint16(data[pos+2:]))
		end := pos + 2 + length
		if length < 2 || end > len(data) {
			return 1
		}
		if marker == 0xe1 {
			if o := exifOrientation(data[pos+4 : end]); o != 0 {
				return o
			}
		}
		pos = end
	}
	return 1
}

// exifOrientation reads the orientation from the payload of an APP1 segment.
// It returns 0 if the segment is not EXIF or holds no valid orientation.
func exifOrientation(seg []byte) int {
	const header = "Exif\x00\x00"
	if len(seg) < len(header)+8 || string(seg[:len(header)]) != header {
		return 0
	}
	tiff := seg[len(header):]
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	ifd := int(order.Uint32(tiff[4:]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 0
	}
	count := int(order.Uint16(tiff[ifd:]))
	for i := range count {
		at := ifd + 2 + i*ifdEntryLen
		if at+ifdEntryLen > len(tiff) {
			return 0
		}
		if order.Uint16(tiff[at:]) != tagOrientation || order.Uint16(tiff[at+2:]) != typeShort {
			continue
		}
		if v := int(order.Uint16(tiff[at+8:])); v >= 1 && v <= 8 {
			return v
		}
		return 0
	}
	return 0
}

// applyOrientation returns img turned and mirrored as the EXIF orientation
// says, so that it is upright without the tag.
func applyOrientation(img image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			dx, dy := orientedPoint(orientation, x, y, w, h)
			dst.Set(dx, dy, color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return dst
}

// orientedPoint maps a source pixel to its place in the upright image.
func orientedPoint(orientation, x, y, w, h int) (int, int) {
	switch orientation {
	case 2: // mirrored horizontally
		return w - 1 - x, y
	case 3: // rotated 180
		return w - 1 - x, h - 1 - y
	case 4: // mirrored vertically
		return x, h - 1 - y
	case 5: // transposed
		return y, x
	case 6: // rotated 90 clockwise
		return h - 1 - y, x
	case 7: // transversed
		return h - 1 - y, w - 1 - x
	default: // 8: rotated 90 counter-clockwise
		return y, w - 1 - x
	}
}
