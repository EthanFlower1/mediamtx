package ai

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
)

// DecodeFrame decodes an encoded frame to an image.Image.
// For v1, only MJPEG/JPEG is supported. H.264 frame decoding can be added
// later using pion/mediadevices or similar.
func DecodeFrame(data []byte, codec string) (image.Image, error) {
	switch codec {
	case "jpeg", "mjpeg":
		return jpeg.Decode(bytes.NewReader(data))
	default:
		return nil, fmt.Errorf("unsupported codec for AI: %s (use MJPEG sub-stream for best results)", codec)
	}
}
