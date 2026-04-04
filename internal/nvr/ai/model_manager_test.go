package ai

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyModel(t *testing.T) {
	tests := []struct {
		filename string
		wantType ModelType
		wantCaps []string
	}{
		{"yolov8n.onnx", ModelTypeDetection, []string{"object_detection", "lightweight", "real_time"}},
		{"yolov8s.onnx", ModelTypeDetection, []string{"object_detection", "balanced", "real_time"}},
		{"yolov8m.onnx", ModelTypeDetection, []string{"object_detection", "medium_accuracy"}},
		{"yolov8x.onnx", ModelTypeDetection, []string{"object_detection", "high_accuracy"}},
		{"yolov8n-seg.onnx", ModelTypeDetection, []string{"object_detection", "lightweight", "real_time", "instance_segmentation"}},
		{"clip-visual.onnx", ModelTypeEmbedding, []string{"visual_embedding", "semantic_search"}},
		{"custom-model.onnx", ModelTypeDetection, []string{"object_detection"}},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			gotType, gotCaps := classifyModel(tt.filename)
			if gotType != tt.wantType {
				t.Errorf("classifyModel(%q) type = %v, want %v", tt.filename, gotType, tt.wantType)
			}
			if len(gotCaps) != len(tt.wantCaps) {
				t.Errorf("classifyModel(%q) caps = %v, want %v", tt.filename, gotCaps, tt.wantCaps)
				return
			}
			for i, cap := range gotCaps {
				if cap != tt.wantCaps[i] {
					t.Errorf("classifyModel(%q) caps[%d] = %q, want %q", tt.filename, i, cap, tt.wantCaps[i])
				}
			}
		})
	}
}

func TestListModels_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	mgr := NewModelManager(dir, nil, "")

	models, err := mgr.ListModels()
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 0 {
		t.Errorf("ListModels() returned %d models, want 0", len(models))
	}
}

func TestListModels_NonExistentDir(t *testing.T) {
	mgr := NewModelManager("/nonexistent/path/models", nil, "")

	models, err := mgr.ListModels()
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 0 {
		t.Errorf("ListModels() returned %d models, want 0", len(models))
	}
}

func TestListModels_ScansONNXFiles(t *testing.T) {
	dir := t.TempDir()

	// Create fake model files.
	for _, name := range []string{"yolov8n.onnx", "yolov8s.onnx", "readme.txt", "clip-visual.onnx"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Create a subdirectory that should be skipped.
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	mgr := NewModelManager(dir, nil, filepath.Join(dir, "yolov8n.onnx"))

	models, err := mgr.ListModels()
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("ListModels() returned %d models, want 3", len(models))
	}

	// Check that the active model is marked.
	var activeCount int
	for _, m := range models {
		if m.Active {
			activeCount++
			if m.Name != "yolov8n.onnx" {
				t.Errorf("expected yolov8n.onnx to be active, got %s", m.Name)
			}
		}
	}
	if activeCount != 1 {
		t.Errorf("expected 1 active model, got %d", activeCount)
	}
}

func TestModelManager_ActivateModel_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	mgr := NewModelManager(dir, nil, "")

	err := mgr.ActivateModel(filepath.Join(dir, "nonexistent.onnx"))
	if err == nil {
		t.Fatal("ActivateModel() should have returned an error for missing file")
	}
}

func TestModelManager_ActivateModel_WrongType(t *testing.T) {
	dir := t.TempDir()
	// Create a CLIP model file (embedding type).
	modelPath := filepath.Join(dir, "clip-visual.onnx")
	os.WriteFile(modelPath, []byte("fake"), 0644)

	mgr := NewModelManager(dir, nil, "")

	err := mgr.ActivateModel(modelPath)
	if err == nil {
		t.Fatal("ActivateModel() should have returned an error for non-detection model")
	}
}

func TestModelManager_Rollback_NoPrevious(t *testing.T) {
	dir := t.TempDir()
	mgr := NewModelManager(dir, nil, "")

	err := mgr.Rollback()
	if err == nil {
		t.Fatal("Rollback() should have returned an error when no previous model exists")
	}
}
