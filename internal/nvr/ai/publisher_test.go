// internal/nvr/ai/publisher_test.go
package ai

import (
	"image"
	"testing"
)

func TestCropRegion(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	box := BoundingBox{0.1, 0.2, 0.5, 0.5}
	crop := cropRegion(img, box)
	if crop == nil {
		t.Fatal("expected non-nil crop")
	}
	bounds := crop.Bounds()
	if bounds.Dx() != 50 || bounds.Dy() != 50 {
		t.Errorf("crop size = %dx%d, want 50x50", bounds.Dx(), bounds.Dy())
	}
}

func TestCropRegionNilImage(t *testing.T) {
	crop := cropRegion(nil, BoundingBox{0.1, 0.1, 0.5, 0.5})
	if crop != nil {
		t.Error("expected nil crop for nil image")
	}
}

func TestCropRegionZeroSize(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	crop := cropRegion(img, BoundingBox{0.1, 0.1, 0, 0})
	if crop != nil {
		t.Error("expected nil crop for zero-size box")
	}
}
