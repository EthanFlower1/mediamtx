// cmd/backfill/main.go — one-shot tool to backfill CLIP embeddings for detections.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/ai"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func main() {
	if err := ai.InitONNXRuntime(); err != nil {
		log.Fatalf("init ONNX runtime: %v", err)
	}
	defer ai.ShutdownONNXRuntime()

	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".mediamtx", "nvr.db")

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	embedder, err := ai.NewEmbedder(
		"models/clip-vit-b32-visual.onnx",
		"models/clip-vit-b32-text.onnx",
		"models/clip-vocab.json",
		"models/clip-visual-projection.bin",
	)
	if err != nil {
		log.Fatalf("load embedder: %v", err)
	}
	log.Println("CLIP embedder loaded")

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)

	dets, err := database.ListDetectionsNeedingEmbedding(start, end)
	if err != nil {
		log.Fatalf("list detections: %v", err)
	}
	log.Printf("Found %d detections needing embeddings", len(dets))

	if len(dets) == 0 {
		return
	}

	succeeded, failed := 0, 0
	for i, det := range dets {
		frameTime, err := time.Parse("2006-01-02T15:04:05.000Z", det.FrameTime)
		if err != nil {
			failed++
			continue
		}

		recs, err := database.QueryRecordings(det.CameraID, frameTime.Add(-1*time.Second), frameTime.Add(1*time.Second))
		if err != nil || len(recs) == 0 {
			recs, _ = database.QueryRecordings(det.CameraID, frameTime.Add(-60*time.Second), frameTime)
		}
		if len(recs) == 0 {
			failed++
			continue
		}

		rec := recs[len(recs)-1]
		recStart, _ := time.Parse("2006-01-02T15:04:05.000Z", rec.StartTime)
		offset := frameTime.Sub(recStart)

		img, err := extractFrame(rec.FilePath, offset)
		if err != nil {
			failed++
			continue
		}

		bounds := img.Bounds()
		x := int(det.BoxX * float64(bounds.Dx()))
		y := int(det.BoxY * float64(bounds.Dy()))
		w := int(det.BoxW * float64(bounds.Dx()))
		h := int(det.BoxH * float64(bounds.Dy()))
		if w <= 0 || h <= 0 {
			failed++
			continue
		}
		cropRect := image.Rect(x, y, x+w, y+h).Intersect(bounds)
		if cropRect.Empty() {
			failed++
			continue
		}
		cropped := cropToNRGBA(img, cropRect)

		embedding, err := embedder.EncodeImage(cropped)
		if err != nil {
			failed++
			continue
		}

		embBytes := float32ToBytes(embedding)
		if err := database.UpdateDetectionEmbedding(det.ID, embBytes); err != nil {
			failed++
			continue
		}

		succeeded++
		if (i+1)%50 == 0 || i+1 == len(dets) {
			log.Printf("progress: %d/%d (ok=%d, fail=%d)", i+1, len(dets), succeeded, failed)
		}
	}

	log.Printf("Done: %d succeeded, %d failed out of %d", succeeded, failed, len(dets))
}

func extractFrame(filePath string, offset time.Duration) (image.Image, error) {
	cmd := exec.Command("ffmpeg",
		"-ss", fmt.Sprintf("%.3f", offset.Seconds()),
		"-i", filePath,
		"-frames:v", "1",
		"-f", "image2",
		"-c:v", "mjpeg",
		"-an",
		"pipe:1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	img, _, err := image.Decode(&stdout)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return img, nil
}

func cropToNRGBA(src image.Image, rect image.Rectangle) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			dst.SetNRGBA(x-rect.Min.X, y-rect.Min.Y, color.NRGBA{
				R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8),
			})
		}
	}
	return dst
}

func float32ToBytes(fs []float32) []byte {
	buf := make([]byte, len(fs)*4)
	for i, f := range fs {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}
