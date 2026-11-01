package images

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/taskmedia/outline-latex-pdf/internal/limits"
)

var (
	numberPattern = `[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?`
	lengthRE      = regexp.MustCompile(`^\s*(` + numberPattern + `)\s*(px|pt|pc|mm|cm|in)?\s*$`)
	numberRE      = regexp.MustCompile(`^` + numberPattern + `$`)
)

// pixelsPerUnit converts the absolute CSS units to CSS pixels at 96 dpi.
var pixelsPerUnit = map[string]float64{
	"": 1, "px": 1, "pt": 96.0 / 72, "pc": 16, "mm": 96 / 25.4, "cm": 96 / 2.54, "in": 96,
}

// parseLength reads an absolute length in CSS pixels. A relative one (a
// percentage, em, ex) or anything that is not a length reports false: such
// a value leaves the size to the viewBox.
func parseLength(s string) (float64, bool) {
	m := lengthRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v * pixelsPerUnit[m[2]], true
}

// parseViewBox reads "minx miny width height". ok is false when the
// attribute is absent; a rejection is returned when it is malformed or
// absurd.
func parseViewBox(s string) (w, h float64, present bool, rej *Rejection) {
	if strings.TrimSpace(s) == "" {
		return 0, 0, false, nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	nums := make([]float64, 0, 4)
	for _, f := range fields {
		if !numberRE.MatchString(f) {
			break
		}
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			break
		}
		nums = append(nums, v)
	}
	if len(nums) != 4 || len(fields) != 4 {
		return 0, 0, true, reject(KindSVGRejected, "the SVG viewBox is not four numbers")
	}
	w, h = nums[2], nums[3]
	if w <= 0 || h <= 0 || w > limits.MaxSVGViewBox || h > limits.MaxSVGViewBox {
		return 0, 0, true, reject(KindSVGRejected, "the SVG viewBox must have a width and height above 0 and at most %d", limits.MaxSVGViewBox)
	}
	return w, h, true, nil
}

// naturalSize is the size the converter will give the page: the width and
// height of the root element, a missing side taken from the viewBox aspect
// ratio, or the viewBox itself, in CSS pixels.
func (w *svgWalker) naturalSize() (float64, float64, *Rejection) {
	vbW, vbH, hasViewBox, rej := parseViewBox(w.viewBox)
	if rej != nil {
		return 0, 0, rej
	}
	width, hasW := parseLength(w.width)
	height, hasH := parseLength(w.height)
	switch {
	case hasW && hasH:
	case hasW && hasViewBox:
		height = width * vbH / vbW
	case hasH && hasViewBox:
		width = height * vbW / vbH
	case hasViewBox:
		width, height = vbW, vbH
	default:
		return 0, 0, reject(KindSVGRejected, "the SVG has no usable size; give the root element a width and height, or a viewBox")
	}
	if width <= 0 || height <= 0 || width > limits.MaxSVGSizePx || height > limits.MaxSVGSizePx {
		return 0, 0, reject(KindSVGRejected, "the SVG size must be above 0 and at most %d pixels (about %d cm) on each side",
			limits.MaxSVGSizePx, limits.MaxSVGSizePx*254/96/100)
	}
	return width, height, nil
}
