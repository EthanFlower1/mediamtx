package inference_test

import (
	"testing"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

func TestTensorValidate(t *testing.T) {
	tests := []struct {
		name    string
		tensor  inference.Tensor
		wantErr bool
	}{
		{
			name: "valid float32",
			tensor: inference.Tensor{
				Shape: []int{2, 3},
				DType: inference.DTypeFloat32,
				Data:  make([]byte, 2*3*4),
			},
		},
		{
			name: "missing dtype",
			tensor: inference.Tensor{
				Shape: []int{1},
				Data:  []byte{0, 0, 0, 0},
			},
			wantErr: true,
		},
		{
			name: "byte length mismatch",
			tensor: inference.Tensor{
				Shape: []int{4},
				DType: inference.DTypeFloat32,
				Data:  []byte{0, 0},
			},
			wantErr: true,
		},
		{
			name: "scalar",
			tensor: inference.Tensor{
				Shape: []int{},
				DType: inference.DTypeInt64,
				Data:  make([]byte, 8),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tensor.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestBackendKindIsReal(t *testing.T) {
	cases := map[inference.BackendKind]bool{
		inference.BackendONNXRuntime: true,
		inference.BackendTensorRT:    true,
		inference.BackendCoreML:      true,
		inference.BackendDirectML:    true,
		inference.BackendFake:        false,
	}
	for b, want := range cases {
		if got := b.IsReal(); got != want {
			t.Errorf("%s.IsReal() = %v, want %v", b, got, want)
		}
	}
}

func TestDTypeElementSize(t *testing.T) {
	cases := map[inference.DType]int{
		inference.DTypeFloat32: 4,
		inference.DTypeFloat16: 2,
		inference.DTypeInt8:    1,
		inference.DTypeUint8:   1,
		inference.DTypeInt32:   4,
		inference.DTypeInt64:   8,
		inference.DTypeBool:    1,
	}
	for d, want := range cases {
		if got := d.ElementSize(); got != want {
			t.Errorf("%s.ElementSize() = %d, want %d", d, got, want)
		}
	}
}
