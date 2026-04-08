package behavioral

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// Store is the persistence interface for behavioral_config rows.
// All methods scope every predicate by tenantID — cross-tenant access is
// impossible through this interface.
type Store interface {
	// Get retrieves a single (tenant, camera, detector) config.
	// Returns ErrNotFound if the row does not exist.
	Get(ctx context.Context, tenantID, cameraID string, dt DetectorType) (Config, error)

	// List returns all detector configs for a (tenant, camera) pair.
	// Returns an empty slice (not nil) when nothing is configured.
	List(ctx context.Context, tenantID, cameraID string) ([]Config, error)

	// Upsert creates or fully replaces the config row. The validator MUST be
	// called before Upsert — this method does not validate params.
	Upsert(ctx context.Context, cfg Config) error

	// Delete removes a single (tenant, camera, detector) config.
	// Returns ErrNotFound if the row does not exist.
	Delete(ctx context.Context, tenantID, cameraID string, dt DetectorType) error
}

// -----------------------------------------------------------------------
// pgStore — Postgres + SQLite-portable implementation
// -----------------------------------------------------------------------

type pgStore struct {
	db *clouddb.DB
}

// NewStore constructs a SQL-backed Store backed by the provided DB handle.
// Works with both Postgres (production) and SQLite (unit tests).
func NewStore(db *clouddb.DB) Store {
	return &pgStore{db: db}
}

func (s *pgStore) ph(i int) string {
	return s.db.Placeholder(i)
}

func (s *pgStore) Get(ctx context.Context, tenantID, cameraID string, dt DetectorType) (Config, error) {
	if tenantID == "" {
		return Config{}, ErrInvalidTenantID
	}
	if cameraID == "" {
		return Config{}, ErrInvalidCameraID
	}
	if !dt.IsValid() {
		return Config{}, ErrInvalidDetectorType
	}

	q := fmt.Sprintf(
		`SELECT tenant_id, camera_id, detector_type, params, enabled, created_at, updated_at
		 FROM behavioral_config
		 WHERE tenant_id = %s AND camera_id = %s AND detector_type = %s`,
		s.ph(1), s.ph(2), s.ph(3),
	)
	row := s.db.QueryRowContext(ctx, q, tenantID, cameraID, string(dt))
	cfg, err := scanConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{}, ErrNotFound
	}
	if err != nil {
		return Config{}, fmt.Errorf("behavioral.Get: %w", err)
	}
	return cfg, nil
}

func (s *pgStore) List(ctx context.Context, tenantID, cameraID string) ([]Config, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if cameraID == "" {
		return nil, ErrInvalidCameraID
	}

	q := fmt.Sprintf(
		`SELECT tenant_id, camera_id, detector_type, params, enabled, created_at, updated_at
		 FROM behavioral_config
		 WHERE tenant_id = %s AND camera_id = %s
		 ORDER BY detector_type ASC`,
		s.ph(1), s.ph(2),
	)
	rows, err := s.db.QueryContext(ctx, q, tenantID, cameraID)
	if err != nil {
		return nil, fmt.Errorf("behavioral.List: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Config
	for rows.Next() {
		var cfg Config
		cfg, err = scanConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("behavioral.List scan: %w", err)
		}
		out = append(out, cfg)
	}
	if rowErr := rows.Err(); rowErr != nil {
		return nil, fmt.Errorf("behavioral.List rows: %w", rowErr)
	}
	if out == nil {
		out = []Config{}
	}
	return out, nil
}

func (s *pgStore) Upsert(ctx context.Context, cfg Config) error {
	if cfg.TenantID == "" {
		return ErrInvalidTenantID
	}
	if cfg.CameraID == "" {
		return ErrInvalidCameraID
	}
	if !cfg.DetectorType.IsValid() {
		return ErrInvalidDetectorType
	}
	if cfg.Params == "" {
		cfg.Params = "{}"
	}

	now := time.Now().UTC()

	var q string
	if s.db.Dialect() == clouddb.DialectPostgres {
		q = fmt.Sprintf(
			`INSERT INTO behavioral_config
			    (tenant_id, camera_id, detector_type, params, enabled, created_at, updated_at)
			 VALUES (%s, %s, %s, %s, %s, %s, %s)
			 ON CONFLICT (tenant_id, camera_id, detector_type)
			 DO UPDATE SET params = EXCLUDED.params,
			               enabled = EXCLUDED.enabled,
			               updated_at = EXCLUDED.updated_at`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7),
		)
	} else {
		// SQLite uses INSERT OR REPLACE which resets created_at — acceptable for
		// unit tests where we don't assert on created_at stability.
		q = fmt.Sprintf(
			`INSERT OR REPLACE INTO behavioral_config
			    (tenant_id, camera_id, detector_type, params, enabled, created_at, updated_at)
			 VALUES (%s, %s, %s, %s, %s, COALESCE(
			    (SELECT created_at FROM behavioral_config
			     WHERE tenant_id = %s AND camera_id = %s AND detector_type = %s),
			    %s
			 ), %s)`,
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
			s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10),
		)
		_, err := s.db.ExecContext(ctx, q,
			cfg.TenantID, cfg.CameraID, string(cfg.DetectorType), cfg.Params, boolToInt(cfg.Enabled),
			cfg.TenantID, cfg.CameraID, string(cfg.DetectorType), now, now,
		)
		if err != nil {
			return fmt.Errorf("behavioral.Upsert: %w", err)
		}
		return nil
	}

	_, err := s.db.ExecContext(ctx, q,
		cfg.TenantID, cfg.CameraID, string(cfg.DetectorType), cfg.Params,
		boolToInt(cfg.Enabled), now, now,
	)
	if err != nil {
		return fmt.Errorf("behavioral.Upsert: %w", err)
	}
	return nil
}

func (s *pgStore) Delete(ctx context.Context, tenantID, cameraID string, dt DetectorType) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if cameraID == "" {
		return ErrInvalidCameraID
	}
	if !dt.IsValid() {
		return ErrInvalidDetectorType
	}

	q := fmt.Sprintf(
		`DELETE FROM behavioral_config
		 WHERE tenant_id = %s AND camera_id = %s AND detector_type = %s`,
		s.ph(1), s.ph(2), s.ph(3),
	)
	res, err := s.db.ExecContext(ctx, q, tenantID, cameraID, string(dt))
	if err != nil {
		return fmt.Errorf("behavioral.Delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// -----------------------------------------------------------------------
// scan helpers
// -----------------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanConfig(row rowScanner) (Config, error) {
	var cfg Config
	var dtStr string
	var enabledInt int // SQLite stores BOOLEAN as INTEGER

	err := row.Scan(
		&cfg.TenantID, &cfg.CameraID, &dtStr,
		&cfg.Params, &enabledInt,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return Config{}, err
	}
	cfg.DetectorType = DetectorType(dtStr)
	cfg.Enabled = enabledInt != 0
	return cfg, nil
}

// boolToInt converts a bool to 0/1 for SQLite compatibility. Postgres
// accepts both native bool and integer so this is safe for both dialects.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
