package lpr

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	fakeinf "github.com/bluenviron/mediamtx/internal/shared/inference/fake"
)

// makeLocalisationTensor builds a fake localisation output tensor that encodes
// one plate box with the given confidence. The layout matches decodePlateBoxes:
// channel-major [5, 1] (4 bbox channels + 1 score), all in float32.
func makeLocalisationTensor(conf float32) InferenceTensor {
	// 5 channels × 1 anchor
	floats := []float32{
		320, // cx (pixel)
		240, // cy (pixel)
		100, // w (pixel)
		40,  // h (pixel)
		conf, // plate confidence
	}
	data := make([]byte, len(floats)*4)
	for i, f := range floats {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(f))
	}
	return InferenceTensor{
		Name:  "input",
		Shape: []int{1, 5, 1},
		DType: "float32",
		Data:  data,
	}
}

// makeReaderTensor builds a fake CRNN output tensor that encodes a confidence
// and plate text string. The decodeCRNNOutput function expects:
// [confidence float32 LE][plate text UTF-8 bytes]
func makeReaderTensor(conf float32, text string) InferenceTensor {
	textBytes := []byte(text)
	data := make([]byte, 4+len(textBytes))
	binary.LittleEndian.PutUint32(data[:4], math.Float32bits(conf))
	copy(data[4:], textBytes)
	return InferenceTensor{
		Name:  "input",
		Shape: []int{1, len(data)},
		DType: "float32",
		Data:  data,
	}
}

// fakeModelRegistry is a minimal ModelRegistry that returns preset bytes for
// known model IDs.
type fakeModelRegistry struct {
	models map[string][]byte
}

func (r *fakeModelRegistry) Resolve(_ context.Context, modelID string) ([]byte, string, error) {
	if b, ok := r.models[modelID]; ok {
		return b, "test-v1", nil
	}
	return nil, "", ErrInvalidConfig // stand-in for inference.ErrModelNotFound
}

func TestDetectorNew(t *testing.T) {
	inf := fakeinf.New(fakeinf.WithRegistry(&fakeModelRegistry{
		models: map[string][]byte{
			"lpr-loc":  {0x01},
			"lpr-read": {0x02},
		},
	}))

	cfg := Config{
		LocalisationModelID:       "lpr-loc",
		ReaderModelID:             "lpr-read",
		ConfidenceThreshold:       0.5,
		ReaderConfidenceThreshold: 0.6,
	}
	det, err := New(cfg, inf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := det.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDetectorNewNilInferencer(t *testing.T) {
	cfg := Config{
		LocalisationModelID: "lpr-loc",
		ReaderModelID:       "lpr-read",
	}
	_, err := New(cfg, nil)
	if err == nil {
		t.Fatal("expected error with nil inferencer; got nil")
	}
}

func TestDetectorClosedAfterClose(t *testing.T) {
	inf := fakeinf.New(fakeinf.WithRegistry(&fakeModelRegistry{
		models: map[string][]byte{
			"lpr-loc":  {0x01},
			"lpr-read": {0x02},
		},
	}))
	cfg := Config{LocalisationModelID: "lpr-loc", ReaderModelID: "lpr-read"}
	det, err := New(cfg, inf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = det.Close()

	_, err = det.ProcessFrame(context.Background(), Frame{
		TenantID: "t1", CameraID: "c1", Width: 640, Height: 480,
		CapturedAt: time.Now(), Tensor: InferenceTensor{Name: "x", Shape: []int{1}, DType: "float32", Data: make([]byte, 4)},
	}, CameraLPRConfig{Enabled: true})
	if err != ErrDetectorClosed {
		t.Errorf("expected ErrDetectorClosed; got %v", err)
	}
}

func TestDetectorDisabledCamera(t *testing.T) {
	inf := fakeinf.New(fakeinf.WithRegistry(&fakeModelRegistry{
		models: map[string][]byte{
			"lpr-loc":  {0x01},
			"lpr-read": {0x02},
		},
	}))
	cfg := Config{LocalisationModelID: "lpr-loc", ReaderModelID: "lpr-read"}
	det, _ := New(cfg, inf)
	defer det.Close()

	reads, err := det.ProcessFrame(context.Background(), Frame{
		Width: 640, Height: 480, Tensor: InferenceTensor{Name: "x", Shape: []int{1}, DType: "float32", Data: make([]byte, 4)},
	}, CameraLPRConfig{Enabled: false})
	if err != nil || reads != nil {
		t.Errorf("disabled camera: want nil,nil; got %v,%v", reads, err)
	}
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid", Config{LocalisationModelID: "a", ReaderModelID: "b"}, false},
		{"missing_loc", Config{ReaderModelID: "b"}, true},
		{"missing_read", Config{LocalisationModelID: "a"}, true},
		{"bad_conf", Config{LocalisationModelID: "a", ReaderModelID: "b", ConfidenceThreshold: 1.5}, true},
	}
	for _, tc := range cases {
		err := tc.cfg.Validate()
		if tc.wantErr && err == nil {
			t.Errorf("%s: expected error; got nil", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
	}
}
