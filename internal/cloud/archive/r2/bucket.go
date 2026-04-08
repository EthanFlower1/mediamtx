package r2

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// KeySchema represents a fully-validated, tenant-scoped R2 object key.
//
// Format: {tenant_id}/{directory_id}/{camera_id}/{yyyy}/{mm}/{dd}/{segment_uuid}.mp4
//
// The tenant_id prefix is the primary multi-tenant isolation mechanism at the
// key-schema level. Client-side isolation complements (but does not replace)
// per-tenant IAM token scoping (see KAI-231 IaC).
//
// KeySchema is immutable after construction. Use NewKeySchema or ParseKeySchema
// to obtain a value. Never construct from raw strings directly.
type KeySchema struct {
	TenantID    string
	DirectoryID string
	CameraID    string
	Date        time.Time
	SegmentUUID uuid.UUID
}

var (
	// ErrInvalidKey is returned when a raw key string fails validation.
	ErrInvalidKey = errors.New("r2: invalid key schema")

	// ErrCrossTenantKey is returned when a key's tenant prefix does not match
	// the expected tenant. This is the key-schema level cross-tenant guard.
	ErrCrossTenantKey = errors.New("r2: cross-tenant key rejected")

	// segmentIDRe validates UUID format without hyphens (our segment UUIDs are
	// formatted with hyphens by uuid.String()).
	segmentIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

	// componentRe validates that key components are safe path segments:
	// alphanumeric, hyphens, underscores only. This prevents path injection.
	componentRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// NewKeySchema constructs a validated KeySchema.
//
// All ID components must be non-empty and contain only [A-Za-z0-9_-].
// segmentUUID must be a valid UUID (use uuid.New()).
func NewKeySchema(tenantID, directoryID, cameraID string, when time.Time, segmentUUID uuid.UUID) (KeySchema, error) {
	if err := validateComponent("tenant_id", tenantID); err != nil {
		return KeySchema{}, err
	}
	if err := validateComponent("directory_id", directoryID); err != nil {
		return KeySchema{}, err
	}
	if err := validateComponent("camera_id", cameraID); err != nil {
		return KeySchema{}, err
	}
	if when.IsZero() {
		return KeySchema{}, fmt.Errorf("%w: segment date must not be zero", ErrInvalidKey)
	}
	if segmentUUID == (uuid.UUID{}) {
		return KeySchema{}, fmt.Errorf("%w: segment UUID must not be nil", ErrInvalidKey)
	}
	return KeySchema{
		TenantID:    tenantID,
		DirectoryID: directoryID,
		CameraID:    cameraID,
		Date:        when.UTC(),
		SegmentUUID: segmentUUID,
	}, nil
}

// String renders the full R2 object key.
// Format: {tenant_id}/{directory_id}/{camera_id}/{yyyy}/{mm}/{dd}/{segment_uuid}.mp4
func (k KeySchema) String() string {
	d := k.Date.UTC()
	return fmt.Sprintf("%s/%s/%s/%04d/%02d/%02d/%s.mp4",
		k.TenantID, k.DirectoryID, k.CameraID,
		d.Year(), int(d.Month()), d.Day(),
		k.SegmentUUID.String(),
	)
}

// ParseKeySchema parses a raw R2 object key string back into a KeySchema.
// Returns ErrInvalidKey if the key does not conform to the expected format.
// Use this to validate keys received from external sources (e.g., event
// notifications or segment index rows).
func ParseKeySchema(key string) (KeySchema, error) {
	// Expected: tenant/directory/camera/yyyy/mm/dd/uuid.mp4
	parts := strings.Split(key, "/")
	if len(parts) != 7 {
		return KeySchema{}, fmt.Errorf("%w: want 7 path segments, got %d in %q", ErrInvalidKey, len(parts), key)
	}
	tenantID := parts[0]
	directoryID := parts[1]
	cameraID := parts[2]
	yyyyStr := parts[3]
	mmStr := parts[4]
	ddStr := parts[5]
	fileStr := parts[6]

	for name, val := range map[string]string{
		"tenant_id": tenantID, "directory_id": directoryID, "camera_id": cameraID,
	} {
		if err := validateComponent(name, val); err != nil {
			return KeySchema{}, err
		}
	}

	// Validate date components individually to give clear errors.
	var yyyy, mm, dd int
	if _, err := fmt.Sscanf(yyyyStr, "%d", &yyyy); err != nil || len(yyyyStr) != 4 {
		return KeySchema{}, fmt.Errorf("%w: invalid year %q", ErrInvalidKey, yyyyStr)
	}
	if _, err := fmt.Sscanf(mmStr, "%d", &mm); err != nil || len(mmStr) != 2 || mm < 1 || mm > 12 {
		return KeySchema{}, fmt.Errorf("%w: invalid month %q", ErrInvalidKey, mmStr)
	}
	if _, err := fmt.Sscanf(ddStr, "%d", &dd); err != nil || len(ddStr) != 2 || dd < 1 || dd > 31 {
		return KeySchema{}, fmt.Errorf("%w: invalid day %q", ErrInvalidKey, ddStr)
	}

	// Validate and strip .mp4 suffix.
	if !strings.HasSuffix(fileStr, ".mp4") {
		return KeySchema{}, fmt.Errorf("%w: filename must end in .mp4, got %q", ErrInvalidKey, fileStr)
	}
	uuidStr := strings.TrimSuffix(fileStr, ".mp4")
	if !segmentIDRe.MatchString(uuidStr) {
		return KeySchema{}, fmt.Errorf("%w: invalid segment UUID %q", ErrInvalidKey, uuidStr)
	}
	segUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		return KeySchema{}, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}

	when := time.Date(yyyy, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	return KeySchema{
		TenantID:    tenantID,
		DirectoryID: directoryID,
		CameraID:    cameraID,
		Date:        when,
		SegmentUUID: segUUID,
	}, nil
}

// AssertTenant returns ErrCrossTenantKey if k.TenantID != expectedTenantID.
// Call this before any operation that uses an externally-supplied key.
// This is the key-schema level cross-tenant regression guard (seam #4).
func (k KeySchema) AssertTenant(expectedTenantID string) error {
	if k.TenantID != expectedTenantID {
		// Do not include the expected tenant ID in the error message to avoid
		// leaking tenant enumeration details in logs that may be multi-tenant.
		return fmt.Errorf("%w: key tenant does not match session tenant", ErrCrossTenantKey)
	}
	return nil
}

// validateComponent ensures an ID component is safe for use in an object key.
func validateComponent(name, val string) error {
	if val == "" {
		return fmt.Errorf("%w: %s must not be empty", ErrInvalidKey, name)
	}
	if !componentRe.MatchString(val) {
		return fmt.Errorf("%w: %s %q contains invalid characters (allowed: [A-Za-z0-9_-])", ErrInvalidKey, name, val)
	}
	return nil
}
