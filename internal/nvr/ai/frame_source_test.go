// internal/nvr/ai/frame_source_test.go
package ai

import (
	"image/color"
	"testing"
)

func TestRgbToImage(t *testing.T) {
	// 2x2 image: red, green, blue, white
	data := []byte{
		255, 0, 0, 0, 255, 0,
		0, 0, 255, 255, 255, 255,
	}
	img := rgbToImage(data, 2, 2)

	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 2 {
		t.Fatalf("expected 2x2, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	tests := []struct {
		x, y int
		want color.NRGBA
	}{
		{0, 0, color.NRGBA{255, 0, 0, 255}},
		{1, 0, color.NRGBA{0, 255, 0, 255}},
		{0, 1, color.NRGBA{0, 0, 255, 255}},
		{1, 1, color.NRGBA{255, 255, 255, 255}},
	}
	for _, tt := range tests {
		got := img.NRGBAAt(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("pixel(%d,%d) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}
