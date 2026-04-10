// Package models is the cloud AI model registry for the Kaivue multi-tenant
// control plane (KAI-279). It stores model metadata (framework, version,
// approval state, file reference) but not the model bytes themselves — those
// live in S3/R2 and are referenced via file_ref.
//
// Package boundary: this package imports internal/cloud/db only. It never
// imports apiserver or any other cloud package that would introduce a cycle.
//
// Multi-tenant invariant: every exported method accepts a tenant_id parameter
// and includes it in every SQL predicate. Cross-tenant reads are impossible
// by construction.
package models

import (
	"encoding/json"
	"errors"
	"time"
)

// PlatformBuiltinTenantID is the sentinel tenant_id used for models shipped
// with the platform itself (e.g. YOLO base, CLIP base). Tenant-specific
// Resolve calls fall back to this tenant when no tenant-scoped model exists.
const PlatformBuiltinTenantID = "platform-builtin"

// -----------------------------------------------------------------------
// Framework
// -----------------------------------------------------------------------

// Framework identifies the inference framework a model targets.
type Framework string

const (
	FrameworkONNX     Framework = "onnx"
	FrameworkTensorRT Framework = "tensorrt"
	FrameworkCoreML   Framework = "coreml"
	FrameworkPyTorch  Framework = "pytorch"
	FrameworkTFLite   Framework = "tflite"
)

// -----------------------------------------------------------------------
// ApprovalState + state machine
// -----------------------------------------------------------------------

// ApprovalState is the lifecycle state of a model in the approval pipeline.
type ApprovalState string

const (
	StateDraft      ApprovalState = "draft"
	StateInReview   ApprovalState = "in_review"
	StateApproved   ApprovalState = "approved"
	StateRejected   ApprovalState = "rejected"
	StateDeprecated ApprovalState = "deprecated"
)

// ValidTransitions defines the allowed state machine edges. The key is the
// current state; the value is the set of states reachable from it.
var ValidTransitions = map[ApprovalState][]ApprovalState{
	StateDraft:    {StateInReview},
	StateInReview: {StateApproved, StateRejected},
	StateRejected: {StateDraft},
	StateApproved: {StateDeprecated},
}

// -----------------------------------------------------------------------
// Domain types
// -----------------------------------------------------------------------

// Model is a row from the models table.
type Model struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Framework     Framework       `json:"framework"`
	FileRef       string          `json:"file_ref"`
	FileSHA256    string          `json:"file_sha256"`
	SizeBytes     int64           `json:"size_bytes"`
	Metrics       json.RawMessage `json:"metrics"`
	ApprovalState ApprovalState   `json:"approval_state"`
	ApprovedBy    *string         `json:"approved_by,omitempty"`
	ApprovedAt    *time.Time      `json:"approved_at,omitempty"`
	OwnerUserID   string          `json:"owner_user_id"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// CreateModelInput holds the fields required to insert a new model.
type CreateModelInput struct {
	TenantID    string
	Name        string
	Version     string
	Framework   Framework
	FileRef     string
	FileSHA256  string
	SizeBytes   int64
	OwnerUserID string
}

// UpdateApprovalInput holds the fields required to transition a model's
// approval state.
type UpdateApprovalInput struct {
	ModelID    string
	TenantID   string
	NewState   ApprovalState
	ApprovedBy string
}

// ListFilter constrains a List query. TenantID is always required.
type ListFilter struct {
	TenantID      string
	ApprovalState *ApprovalState // optional filter
	OwnerUserID   *string        // optional filter
}

// -----------------------------------------------------------------------
// Sentinel errors
// -----------------------------------------------------------------------

var (
	// ErrNotFound is returned when a model row does not exist within the
	// queried tenant.
	ErrNotFound = errors.New("models: not found")

	// ErrInvalidTenantID is returned when the tenant_id parameter is empty.
	ErrInvalidTenantID = errors.New("models: tenant_id is required")

	// ErrInvalidID is returned when the model id is empty.
	ErrInvalidID = errors.New("models: id is required")

	// ErrInvalidTransition is returned when an approval state change violates
	// the state machine defined by ValidTransitions.
	ErrInvalidTransition = errors.New("models: invalid state transition")

	// ErrDuplicateVersion is returned when a (tenant_id, name, version) tuple
	// already exists.
	ErrDuplicateVersion = errors.New("models: duplicate version")
)
