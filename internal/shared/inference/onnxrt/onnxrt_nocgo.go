//go:build !cgo

// Package onnxrt implements inference.Inferencer backed by ONNX Runtime.
// This file is the no-cgo stub — every exported function returns
// ErrCGORequired so the binary compiles on platforms without the ONNX
// Runtime shared library.
package onnxrt

import (
	"context"
	"errors"
	"log/slog"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

// ErrCGORequired is returned when the binary is built without CGO.
var ErrCGORequired = errors.New("onnxrt: CGO required for ONNX Runtime backend")

// Inferencer is the no-cgo stub. All methods return ErrCGORequired.
type Inferencer struct{}

// Option is a no-op in the no-cgo build.
type Option func(*Inferencer)

// WithName is a no-op.
func WithName(string) Option { return func(*Inferencer) {} }

// WithRegistry is a no-op.
func WithRegistry(inference.ModelRegistry) Option { return func(*Inferencer) {} }

// WithLibraryPath is a no-op.
func WithLibraryPath(string) Option { return func(*Inferencer) {} }

// WithGPU is a no-op.
func WithGPU(bool) Option { return func(*Inferencer) {} }

// WithLogger is a no-op.
func WithLogger(*slog.Logger) Option { return func(*Inferencer) {} }

// New returns ErrCGORequired.
func New(_ ...Option) (*Inferencer, error) {
	return nil, ErrCGORequired
}

func (i *Inferencer) Name() string                        { return "onnxrt-nocgo" }
func (i *Inferencer) Backend() inference.BackendKind      { return inference.BackendONNXRuntime }
func (i *Inferencer) LoadModel(_ context.Context, _ string, _ []byte) (*inference.LoadedModel, error) {
	return nil, ErrCGORequired
}
func (i *Inferencer) Infer(_ context.Context, _ *inference.LoadedModel, _ inference.Tensor) (*inference.InferenceResult, error) {
	return nil, ErrCGORequired
}
func (i *Inferencer) Unload(_ context.Context, _ *inference.LoadedModel) error { return ErrCGORequired }
func (i *Inferencer) Stats() inference.Stats                                   { return inference.Stats{Backend: inference.BackendONNXRuntime} }
func (i *Inferencer) Close() error                                             { return nil }
