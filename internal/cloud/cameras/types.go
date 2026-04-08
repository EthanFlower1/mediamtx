// Package cameras is the cloud camera registry for the Kaivue multi-tenant
// control plane (KAI-249). It provides the authoritative record of cameras,
// recorders, recording schedules, retention policies, and the segment index.
//
// Package boundary: this package imports internal/shared/* and
// internal/cloud/db/* only. It never imports apiserver or any other cloud
// package that would introduce a cycle.
//
// Multi-tenant invariant: every exported method accepts a tenant_id parameter
// and includes it in every SQL predicate. Cross-tenant reads are impossible
// by construction — the SQL never executes without a tenant scope.
package cameras

import (
	"errors"
	"time"
)

// Sentinel errors. Callers should use errors.Is.
var (
	// ErrNotFound is returned when a row does not exist within the queried tenant.
	// Using a single sentinel for all entity types prevents callers from leaking
	// cross-tenant existence via different error types.
	ErrNotFound = errors.New("cameras: not found")

	// ErrInvalidTenantID is returned when the tenant_id parameter is empty.
	ErrInvalidTenantID = errors.New("cameras: tenant_id is required")

	// ErrInvalidID is returned when the entity id is empty.
	ErrInvalidID = errors.New("cameras: id is required")
)

// -----------------------------------------------------------------------
// Recorder
// -----------------------------------------------------------------------

// Recorder is a row from the recorders table.
type Recorder struct {
	ID                  string
	TenantID            string
	DirectoryID         string
	DisplayName         string
	HardwareSummary     string // raw JSON
	Status              string
	LastCheckinAt       *time.Time
	AssignedCameraCount int
	StorageUsedBytes    int64
	SidecarStatus       string // raw JSON
	LANBaseURL          string
	RelayBaseURL        string
	LANSubnets          string // raw JSON array of CIDR strings
	Region              string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// -----------------------------------------------------------------------
// Camera
// -----------------------------------------------------------------------

// Camera is a row from the cameras table.
// RTSPCredentialsEncrypted is the raw cryptostore envelope; the plaintext
// password is NEVER stored or returned from this package.
type Camera struct {
	ID                         string
	TenantID                   string
	DirectoryID                string
	DisplayName                string
	LocationLabel              string
	Manufacturer               string
	Model                      string
	ONVIFEndpoint              string
	RTSPUrl                    string
	RTSPCredentialsEncrypted   []byte // cryptostore v1 AES-256-GCM blob; nil = not configured
	AssignedRecorderID         string // empty = not assigned
	ScheduleID                 string // empty = no schedule
	RetentionPolicyID          string // empty = platform default
	AIFeatures                 string // raw JSON
	LPREnabled                 bool
	Status                     string
	Region                     string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

// -----------------------------------------------------------------------
// RecordingSchedule
// -----------------------------------------------------------------------

// RecordingSchedule is a row from the recording_schedules table.
type RecordingSchedule struct {
	ID           string
	TenantID     string
	Name         string
	ScheduleType string // "continuous" | "motion" | "event"
	WeeklyGrid   string // raw JSON
	Region       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// -----------------------------------------------------------------------
// RetentionPolicy
// -----------------------------------------------------------------------

// RetentionPolicy is a row from the retention_policies table.
type RetentionPolicy struct {
	ID             string
	TenantID       string
	Name           string
	HotDays        int
	WarmDays       int
	ColdDays       int
	ArchiveDays    int
	EncryptionMode string // "standard" | "sse_kms" | "cse_cmk"
	Region         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// -----------------------------------------------------------------------
// Segment
// -----------------------------------------------------------------------

// Segment is one row in camera_segment_index (or camera_segment_index_sqlite).
type Segment struct {
	CameraID       string
	RecorderID     string
	TenantID       string
	StartTS        time.Time
	EndTS          time.Time
	FilePath       string
	FileSizeBytes  int64
	StorageTier    string // "hot" | "warm" | "cold" | "archive"
	ChecksumSHA256 string
	Region         string
}
