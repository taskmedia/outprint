package images

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	// pointsPerPixel converts CSS pixels (96 per inch) to points as Pandoc
	// reads them (72 per inch).
	pointsPerPixel = 0.75
	// maxPoints keeps a requested size inside what TeX can represent. The
	// Template clamps the image to the page anyway.
	maxPoints = 10000.0
	minPoints = 1.0
)

// Size is the size an author asked for in Outline's syntax, in CSS pixels.
// A zero side is unset.
type Size struct{ Width, Height int }

// sizeRE matches Outline's size suffix of the image title: "=400x300",
// "=400x", "=x300" and "=400", preceded by the title (possibly empty) and a
// space.
var sizeRE = regexp.MustCompile(`(?:^|\s)=(\d{1,5})?(?:x(\d{1,5})?)?$`)

// ParseSize reads Outline's size from an image title. It reports false when
// the title holds no size.
func ParseSize(title string) (Size, bool) {
	m := sizeRE.FindStringSubmatch(strings.TrimSpace(title))
	if m == nil || (m[1] == "" && m[2] == "") {
		return Size{}, false
	}
	return Size{Width: atoi(m[1]), Height: atoi(m[2])}, true
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// Points returns the width and height in points for an image of w by h
// CSS pixels: the requested size at 96 dpi, a missing side derived from the
// aspect ratio, the natural size when nothing was requested. The result is
// held inside TeX's range with the aspect kept. Clamping to the page is the
// Template's job, because only the Template knows the text area.
func Points(w, h float64, req Size) (float64, float64) {
	pw, ph := float64(req.Width), float64(req.Height)
	switch {
	case pw > 0 && ph > 0:
	case pw > 0:
		ph = pw * h / w
	case ph > 0:
		pw = ph * w / h
	default:
		pw, ph = w, h
	}
	pw, ph = pw*pointsPerPixel, ph*pointsPerPixel
	if big := max(pw, ph); big > maxPoints {
		pw, ph = pw*maxPoints/big, ph*maxPoints/big
	}
	return max(pw, minPoints), max(ph, minPoints)
}

// formatPoints writes a dimension for a Pandoc attribute.
func formatPoints(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64) + "pt"
}
