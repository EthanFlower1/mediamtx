//go:build !cgo

package onnxrt

import (
	"context"
	"errors"
	"testing"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

func TestNewReturnsErrCGORequired(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrCGORequired) {
		t.Fatalf("expected ErrCGORequired, got %v", err)
	}
}

func TestStubMethodsReturnErrors(t *testing.T) {
	// The stub Inferencer cannot be constructed via New() (returns error),
	// but we can still exercise the methods on a zero value to ensure they
	// compile and return the expected errors.
	var i Inferencer

	if i.Name() != "onnxrt-nocgo" {
		t.Errorf("Name() = %q, want onnxrt-nocgo", i.Name())
	}
	if i.Backend() != inference.BackendONNXRuntime {
		t.Errorf("Backend() = %v, want BackendONNXRuntime", i.Backend())
	}

	ctx := context.Background()
	_, err := i.LoadModel(ctx, "test", nil)
	if !errors.Is(err, ErrCGORequired) {
		t.Errorf("LoadModel err = %v, want ErrCGORequired", err)
	}

	_, err = i.Infer(ctx, nil, inference.Tensor{})
	if !errors.Is(err, ErrCGORequired) {
		t.Errorf("Infer err = %v, want ErrCGORequired", err)
	}

	err = i.Unload(ctx, nil)
	if !errors.Is(err, ErrCGORequired) {
		t.Errorf("Unload err = %v, want ErrCGORequired", err)
	}

	stats := i.Stats()
	if stats.Backend != inference.BackendONNXRuntime {
		t.Errorf("Stats.Backend = %v, want BackendONNXRuntime", stats.Backend)
	}

	if err := i.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}
