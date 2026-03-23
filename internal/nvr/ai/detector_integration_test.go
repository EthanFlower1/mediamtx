package ai

import (
    "image/jpeg"
    "os"
    "path/filepath"
    "testing"
)

func TestDetectorIntegration(t *testing.T) {
    home, _ := os.UserHomeDir()
    libPath := filepath.Join(home, "lib", "libonnxruntime.dylib")
    
    if _, err := os.Stat(libPath); err != nil {
        t.Skip("ONNX Runtime not installed, skipping integration test")
    }

    if err := InitONNXRuntime(); err != nil {
        t.Fatalf("ONNX Runtime init: %v", err)
    }
    t.Log("✅ ONNX Runtime initialized")

    // Use absolute path since test working directory varies
    projectRoot := filepath.Join(home, "personal_projects", "mediamtx")
    modelPath := filepath.Join(projectRoot, "models", "yolov8n.onnx")
    if _, err := os.Stat(modelPath); err != nil {
        t.Skipf("Model not found at %s", modelPath)
    }

    detector, err := NewDetector(modelPath)
    if err != nil {
        t.Fatalf("NewDetector: %v", err)
    }
    defer detector.Close()
    t.Log("✅ YOLOv8n loaded")

    // Find a thumbnail to test with
    thumbDir := filepath.Join(projectRoot, "thumbnails")
    entries, _ := os.ReadDir(thumbDir)
    if len(entries) == 0 {
        t.Skip("No thumbnails to test with")
    }

    imgPath := filepath.Join(thumbDir, entries[0].Name())
    f, err := os.Open(imgPath)
    if err != nil {
        t.Fatalf("Open: %v", err)
    }
    defer f.Close()

    img, err := jpeg.Decode(f)
    if err != nil {
        t.Fatalf("Decode: %v", err)
    }
    t.Logf("✅ Image: %dx%d (%s)", img.Bounds().Dx(), img.Bounds().Dy(), entries[0].Name())

    dets, err := detector.Detect(img, 0.3)
    if err != nil {
        t.Fatalf("Detect: %v", err)
    }

    t.Logf("🔍 Found %d detections:", len(dets))
    for i, d := range dets {
        t.Logf("  [%d] %s (%.1f%%) at (%.2f, %.2f, %.2f, %.2f)", 
            i+1, d.ClassName, d.Confidence*100, d.X, d.Y, d.W, d.H)
    }
}
