package ai

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModelManagerListModels(t *testing.T) {
	// Create a temp directory with fake model files.
	dir := t.TempDir()

	// Create some fake model files.
	files := map[string]string{
		"yolov8n.onnx":             "fake-yolo-model",
		"yolov8s.onnx":             "fake-yolo-small-model",
		"clip-vit-b32-visual.onnx": "fake-clip-visual",
		"clip-vocab.json":          `{"hello": 1}`,
		"clip-visual-projection.bin": "fake-projection",
		"README.txt":               "not a model",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mgr := NewModelManager(dir, nil, "")

	models, err := mgr.ListModels()
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	// Should find 5 model files (3 .onnx + 1 .json + 1 .bin), not the .txt
	if len(models) != 5 {
		t.Errorf("expected 5 models, got %d", len(models))
		for _, m := range models {
			t.Logf("  %s (%s)", m.Name, m.Type)
		}
	}

	// Check classification.
	typeMap := make(map[string]ModelType)
	for _, m := range models {
		typeMap[m.Name] = m.Type
	}

	if typeMap["yolov8n.onnx"] != ModelTypeDetector {
		t.Errorf("yolov8n.onnx should be detector, got %s", typeMap["yolov8n.onnx"])
	}
	if typeMap["clip-vit-b32-visual.onnx"] != ModelTypeEmbedder {
		t.Errorf("clip-vit-b32-visual.onnx should be embedder, got %s", typeMap["clip-vit-b32-visual.onnx"])
	}
}

func TestModelManagerListModels_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	mgr := NewModelManager(dir, nil, "")

	models, err := mgr.ListModels()
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

func TestModelManagerListModels_NonexistentDir(t *testing.T) {
	mgr := NewModelManager("/nonexistent/dir", nil, "")

	models, err := mgr.ListModels()
	if err != nil {
		t.Fatalf("ListModels should not error for nonexistent dir: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

func TestModelManagerActiveModel(t *testing.T) {
	mgr := NewModelManager("/tmp", nil, "/tmp/yolov8n.onnx")

	if got := mgr.ActiveModel(); got != "/tmp/yolov8n.onnx" {
		t.Errorf("expected /tmp/yolov8n.onnx, got %s", got)
	}
}

func TestModelManagerActiveModel_NoneLoaded(t *testing.T) {
	mgr := NewModelManager("/tmp", nil, "")

	if got := mgr.ActiveModel(); got != "" {
		t.Errorf("expected empty active model, got %s", got)
	}
}

func TestModelManagerVerifyModel(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "test.onnx")
	if err := os.WriteFile(modelPath, []byte("test model content"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewModelManager(dir, nil, "")

	// Test with absolute path.
	hash, err := mgr.VerifyModel(modelPath)
	if err != nil {
		t.Fatalf("VerifyModel: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}

	// Test with relative name.
	hash2, err := mgr.VerifyModel("test.onnx")
	if err != nil {
		t.Fatalf("VerifyModel relative: %v", err)
	}
	if hash != hash2 {
		t.Errorf("hashes should match: %s vs %s", hash, hash2)
	}
}

func TestModelManagerVerifyModel_NotFound(t *testing.T) {
	mgr := NewModelManager(t.TempDir(), nil, "")

	_, err := mgr.VerifyModel("nonexistent.onnx")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestModelManagerActivate_NotFound(t *testing.T) {
	mgr := NewModelManager(t.TempDir(), nil, "")

	err := mgr.Activate("nonexistent.onnx")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestModelManagerRollback_NoPrevious(t *testing.T) {
	mgr := NewModelManager(t.TempDir(), nil, "")

	err := mgr.Rollback()
	if err == nil {
		t.Error("expected error when no previous model")
	}
}

func TestModelManagerListModels_ActiveFlag(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "yolov8n.onnx")
	if err := os.WriteFile(modelPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewModelManager(dir, nil, modelPath)

	models, err := mgr.ListModels()
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if !models[0].Active {
		t.Error("expected model to be marked active")
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{10485760, "10.0 MiB"},
	}
	for _, tt := range tests {
		got := humanSize(tt.bytes)
		if got != tt.want {
			t.Errorf("humanSize(%d) = %s, want %s", tt.bytes, got, tt.want)
		}
	}
}

func TestClassifyModel(t *testing.T) {
	tests := []struct {
		name string
		want ModelType
	}{
		{"yolov8n.onnx", ModelTypeDetector},
		{"yolov8s.onnx", ModelTypeDetector},
		{"clip-vit-b32-visual.onnx", ModelTypeEmbedder},
		{"clip-vocab.json", ModelTypeEmbedder},
		{"clip-visual-projection.bin", ModelTypeEmbedder},
		{"custom-model.onnx", ModelTypeUnknown},
	}
	for _, tt := range tests {
		got := classifyModel(tt.name)
		if got != tt.want {
			t.Errorf("classifyModel(%q) = %s, want %s", tt.name, got, tt.want)
		}
	}
}
