package devseed

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestSolidPNGIsOneColourAndRepeatable(t *testing.T) {
	first, err := solidPNG(workspaceAvatarColour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := solidPNG(workspaceAvatarColour)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("the same colour must give the same bytes")
	}
	img, err := png.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != avatarSize || b.Dy() != avatarSize {
		t.Errorf("size = %v, want %dx%d", b, avatarSize, avatarSize)
	}
	for _, p := range [][2]int{{0, 0}, {avatarSize - 1, avatarSize - 1}, {avatarSize / 2, avatarSize / 2}} {
		if got := color.RGBAModel.Convert(img.At(p[0], p[1])); got != workspaceAvatarColour {
			t.Errorf("pixel %v = %v, want %v", p, got, workspaceAvatarColour)
		}
	}
}
