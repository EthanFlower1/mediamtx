package cameras

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// -----------------------------------------------------------------------
// cameraStore — implements CameraRegistry
// -----------------------------------------------------------------------

// cameraStore is the SQL-backed CameraRegistry. It works against both the
// Postgres (production) and SQLite (unit test) dialects via the clouddb.DB
// dialect-aware placeholder helper.
type cameraStore struct {
	db *clouddb.DB
}

// NewCameraRegistry constructs a SQL-backed CameraRegistry.
func NewCameraRegistry(db *clouddb.DB) CameraRegistry {
	return &cameraStore{db: db}
}

func (s *cameraStore) ph(i int) string {
	return s.db.Placeholder(i)
}

func (s *cameraStore) List(ctx context.Context, tenantID string) ([]Camera, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	q := fmt.Sprintf(
		`SELECT id, tenant_id, directory_id, display_name, location_label,
		        manufacturer, model, onvif_endpoint, rtsp_url,
		        rtsp_credentials_encrypted, assigned_recorder_id,
		        schedule_id, retention_policy_id, ai_features, lpr_enabled,
		        status, region, created_at, updated_at
		 FROM cameras
		 WHERE tenant_id = %s
		 ORDER BY created_at ASC`,
		s.ph(1),
	)
	rows, err := s.db.QueryContext(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("cameras.List: %w", err)
	}
	defer rows.Close()
	return scanCameras(rows)
}

func (s *cameraStore) Get(ctx context.Context, tenantID, id string) (Camera, error) {
	if tenantID == "" {
		return Camera{}, ErrInvalidTenantID
	}
	if id == "" {
		return Camera{}, ErrInvalidID
	}
	q := fmt.Sprintf(
		`SELECT id, tenant_id, directory_id, display_name, location_label,
		        manufacturer, model, onvif_endpoint, rtsp_url,
		        rtsp_credentials_encrypted, assigned_recorder_id,
		        schedule_id, retention_policy_id, ai_features, lpr_enabled,
		        status, region, created_at, updated_at
		 FROM cameras
		 WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2),
	)
	row := s.db.QueryRowContext(ctx, q, tenantID, id)
	cam, err := scanCamera(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Camera{}, ErrNotFound
	}
	return cam, err
}

// GetCamera satisfies the streams.CameraRegistry interface.
func (s *cameraStore) GetCamera(ctx context.Context, tenantID, cameraID string) (Camera, error) {
	return s.Get(ctx, tenantID, cameraID)
}

func (s *cameraStore) Create(ctx context.Context, cam Camera) error {
	if cam.TenantID == "" {
		return ErrInvalidTenantID
	}
	if cam.ID == "" {
		return ErrInvalidID
	}
	if cam.Region == "" {
		cam.Region = clouddb.DefaultRegion
	}
	if cam.AIFeatures == "" {
		cam.AIFeatures = "{}"
	}
	if cam.Status == "" {
		cam.Status = "unconfigured"
	}

	q := fmt.Sprintf(
		`INSERT INTO cameras
		    (id, tenant_id, directory_id, display_name, location_label,
		     manufacturer, model, onvif_endpoint, rtsp_url,
		     rtsp_credentials_encrypted, assigned_recorder_id,
		     schedule_id, retention_policy_id, ai_features, lpr_enabled,
		     status, region)
		 VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
		s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10),
		s.ph(11), s.ph(12), s.ph(13), s.ph(14), s.ph(15),
		s.ph(16), s.ph(17),
	)

	recorderID := nullString(cam.AssignedRecorderID)
	scheduleID := nullString(cam.ScheduleID)
	retentionID := nullString(cam.RetentionPolicyID)

	_, err := s.db.ExecContext(ctx, q,
		cam.ID, cam.TenantID, cam.DirectoryID, cam.DisplayName, nullString(cam.LocationLabel),
		nullString(cam.Manufacturer), nullString(cam.Model), nullString(cam.ONVIFEndpoint),
		nullString(cam.RTSPUrl), nullBytes(cam.RTSPCredentialsEncrypted),
		recorderID, scheduleID, retentionID,
		cam.AIFeatures, cam.LPREnabled, cam.Status, cam.Region,
	)
	if err != nil {
		return fmt.Errorf("cameras.Create: %w", err)
	}
	return nil
}

func (s *cameraStore) Update(ctx context.Context, cam Camera) error {
	if cam.TenantID == "" {
		return ErrInvalidTenantID
	}
	if cam.ID == "" {
		return ErrInvalidID
	}
	q := fmt.Sprintf(
		`UPDATE cameras
		 SET display_name = %s, location_label = %s, manufacturer = %s,
		     model = %s, onvif_endpoint = %s, rtsp_url = %s,
		     rtsp_credentials_encrypted = %s, assigned_recorder_id = %s,
		     schedule_id = %s, retention_policy_id = %s, ai_features = %s,
		     lpr_enabled = %s, status = %s, updated_at = %s
		 WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7),
		s.ph(8), s.ph(9), s.ph(10), s.ph(11), s.ph(12), s.ph(13), s.ph(14),
		s.ph(15), s.ph(16),
	)
	res, err := s.db.ExecContext(ctx, q,
		cam.DisplayName, nullString(cam.LocationLabel), nullString(cam.Manufacturer),
		nullString(cam.Model), nullString(cam.ONVIFEndpoint), nullString(cam.RTSPUrl),
		nullBytes(cam.RTSPCredentialsEncrypted), nullString(cam.AssignedRecorderID),
		nullString(cam.ScheduleID), nullString(cam.RetentionPolicyID),
		cam.AIFeatures, cam.LPREnabled, cam.Status, time.Now().UTC(),
		cam.TenantID, cam.ID,
	)
	if err != nil {
		return fmt.Errorf("cameras.Update: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *cameraStore) Delete(ctx context.Context, tenantID, id string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if id == "" {
		return ErrInvalidID
	}
	q := fmt.Sprintf(
		`DELETE FROM cameras WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2),
	)
	res, err := s.db.ExecContext(ctx, q, tenantID, id)
	if err != nil {
		return fmt.Errorf("cameras.Delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *cameraStore) ListByRecorder(ctx context.Context, tenantID, recorderID string) ([]Camera, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if recorderID == "" {
		return nil, fmt.Errorf("cameras.ListByRecorder: recorder_id is required")
	}
	q := fmt.Sprintf(
		`SELECT id, tenant_id, directory_id, display_name, location_label,
		        manufacturer, model, onvif_endpoint, rtsp_url,
		        rtsp_credentials_encrypted, assigned_recorder_id,
		        schedule_id, retention_policy_id, ai_features, lpr_enabled,
		        status, region, created_at, updated_at
		 FROM cameras
		 WHERE tenant_id = %s AND assigned_recorder_id = %s
		 ORDER BY created_at ASC`,
		s.ph(1), s.ph(2),
	)
	rows, err := s.db.QueryContext(ctx, q, tenantID, recorderID)
	if err != nil {
		return nil, fmt.Errorf("cameras.ListByRecorder: %w", err)
	}
	defer rows.Close()
	return scanCameras(rows)
}

// -----------------------------------------------------------------------
// scanCamera helpers
// -----------------------------------------------------------------------

type cameraScanner interface {
	Scan(dest ...any) error
}

func scanCamera(row cameraScanner) (Camera, error) {
	var cam Camera
	var locationLabel, manufacturer, model, onvifEndpoint, rtspURL sql.NullString
	var rtspCreds []byte
	var assignedRecorderID, scheduleID, retentionPolicyID sql.NullString
	var lprEnabled int // SQLite stores BOOLEAN as INTEGER

	err := row.Scan(
		&cam.ID, &cam.TenantID, &cam.DirectoryID, &cam.DisplayName, &locationLabel,
		&manufacturer, &model, &onvifEndpoint, &rtspURL,
		&rtspCreds, &assignedRecorderID, &scheduleID, &retentionPolicyID,
		&cam.AIFeatures, &lprEnabled,
		&cam.Status, &cam.Region, &cam.CreatedAt, &cam.UpdatedAt,
	)
	if err != nil {
		return Camera{}, err
	}
	cam.LocationLabel = locationLabel.String
	cam.Manufacturer = manufacturer.String
	cam.Model = model.String
	cam.ONVIFEndpoint = onvifEndpoint.String
	cam.RTSPUrl = rtspURL.String
	cam.RTSPCredentialsEncrypted = rtspCreds
	cam.AssignedRecorderID = assignedRecorderID.String
	cam.ScheduleID = scheduleID.String
	cam.RetentionPolicyID = retentionPolicyID.String
	cam.LPREnabled = lprEnabled != 0
	return cam, nil
}

func scanCameras(rows *sql.Rows) ([]Camera, error) {
	var out []Camera
	for rows.Next() {
		cam, err := scanCamera(rows)
		if err != nil {
			return nil, fmt.Errorf("cameras scan: %w", err)
		}
		out = append(out, cam)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cameras rows: %w", err)
	}
	if out == nil {
		out = []Camera{}
	}
	return out, nil
}

// -----------------------------------------------------------------------
// recorderStore — implements RecorderRegistry
// -----------------------------------------------------------------------

type recorderStore struct {
	db *clouddb.DB
}

// NewRecorderRegistry constructs a SQL-backed RecorderRegistry.
func NewRecorderRegistry(db *clouddb.DB) RecorderRegistry {
	return &recorderStore{db: db}
}

func (s *recorderStore) ph(i int) string {
	return s.db.Placeholder(i)
}

func (s *recorderStore) List(ctx context.Context, tenantID string) ([]Recorder, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	q := fmt.Sprintf(
		`SELECT id, tenant_id, directory_id, display_name, hardware_summary,
		        status, last_checkin_at, assigned_camera_count, storage_used_bytes,
		        sidecar_status, lan_base_url, relay_base_url, lan_subnets,
		        region, created_at, updated_at
		 FROM recorders
		 WHERE tenant_id = %s
		 ORDER BY created_at ASC`,
		s.ph(1),
	)
	rows, err := s.db.QueryContext(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("recorders.List: %w", err)
	}
	defer rows.Close()
	return scanRecorders(rows)
}

func (s *recorderStore) Get(ctx context.Context, tenantID, id string) (Recorder, error) {
	if tenantID == "" {
		return Recorder{}, ErrInvalidTenantID
	}
	if id == "" {
		return Recorder{}, ErrInvalidID
	}
	q := fmt.Sprintf(
		`SELECT id, tenant_id, directory_id, display_name, hardware_summary,
		        status, last_checkin_at, assigned_camera_count, storage_used_bytes,
		        sidecar_status, lan_base_url, relay_base_url, lan_subnets,
		        region, created_at, updated_at
		 FROM recorders
		 WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2),
	)
	row := s.db.QueryRowContext(ctx, q, tenantID, id)
	rec, err := scanRecorder(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Recorder{}, ErrNotFound
	}
	return rec, err
}

func (s *recorderStore) Create(ctx context.Context, rec Recorder) error {
	if rec.TenantID == "" {
		return ErrInvalidTenantID
	}
	if rec.ID == "" {
		return ErrInvalidID
	}
	if rec.Region == "" {
		rec.Region = clouddb.DefaultRegion
	}
	if rec.HardwareSummary == "" {
		rec.HardwareSummary = "{}"
	}
	if rec.SidecarStatus == "" {
		rec.SidecarStatus = "{}"
	}
	if rec.LANSubnets == "" {
		rec.LANSubnets = "[]"
	}
	if rec.Status == "" {
		rec.Status = "offline"
	}
	q := fmt.Sprintf(
		`INSERT INTO recorders
		    (id, tenant_id, directory_id, display_name, hardware_summary,
		     status, assigned_camera_count, storage_used_bytes,
		     sidecar_status, lan_base_url, relay_base_url, lan_subnets, region)
		 VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
		s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10),
		s.ph(11), s.ph(12), s.ph(13),
	)
	_, err := s.db.ExecContext(ctx, q,
		rec.ID, rec.TenantID, rec.DirectoryID, rec.DisplayName, rec.HardwareSummary,
		rec.Status, rec.AssignedCameraCount, rec.StorageUsedBytes,
		rec.SidecarStatus, nullString(rec.LANBaseURL), nullString(rec.RelayBaseURL),
		rec.LANSubnets, rec.Region,
	)
	if err != nil {
		return fmt.Errorf("recorders.Create: %w", err)
	}
	return nil
}

func (s *recorderStore) UpdateStatus(ctx context.Context, tenantID, id, status string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if id == "" {
		return ErrInvalidID
	}
	q := fmt.Sprintf(
		`UPDATE recorders
		 SET status = %s, last_checkin_at = %s, updated_at = %s
		 WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
	)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, q, status, now, now, tenantID, id)
	if err != nil {
		return fmt.Errorf("recorders.UpdateStatus: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// GetTenantID returns the owning tenantID for a recorder without tenant-scoping
// the query. This is the one intentional exception to the "always tenant-scope"
// rule: the caller (recordercontrol.Handler) needs the tenant BEFORE it can
// scope the subsequent operations. The result is immediately verified by the
// handler against the session's tenantID.
func (s *recorderStore) GetTenantID(ctx context.Context, recorderID string) (string, error) {
	if recorderID == "" {
		return "", ErrInvalidID
	}
	q := fmt.Sprintf(
		`SELECT tenant_id FROM recorders WHERE id = %s`,
		s.ph(1),
	)
	var tid string
	err := s.db.QueryRowContext(ctx, q, recorderID).Scan(&tid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("recorders.GetTenantID: %w", err)
	}
	return tid, nil
}

func (s *recorderStore) Delete(ctx context.Context, tenantID, id string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if id == "" {
		return ErrInvalidID
	}
	q := fmt.Sprintf(
		`DELETE FROM recorders WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2),
	)
	res, err := s.db.ExecContext(ctx, q, tenantID, id)
	if err != nil {
		return fmt.Errorf("recorders.Delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func scanRecorder(row cameraScanner) (Recorder, error) {
	var rec Recorder
	var lanBaseURL, relayBaseURL sql.NullString
	err := row.Scan(
		&rec.ID, &rec.TenantID, &rec.DirectoryID, &rec.DisplayName, &rec.HardwareSummary,
		&rec.Status, &rec.LastCheckinAt, &rec.AssignedCameraCount, &rec.StorageUsedBytes,
		&rec.SidecarStatus, &lanBaseURL, &relayBaseURL, &rec.LANSubnets,
		&rec.Region, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		return Recorder{}, err
	}
	rec.LANBaseURL = lanBaseURL.String
	rec.RelayBaseURL = relayBaseURL.String
	return rec, nil
}

func scanRecorders(rows *sql.Rows) ([]Recorder, error) {
	var out []Recorder
	for rows.Next() {
		rec, err := scanRecorder(rows)
		if err != nil {
			return nil, fmt.Errorf("recorders scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recorders rows: %w", err)
	}
	if out == nil {
		out = []Recorder{}
	}
	return out, nil
}

// -----------------------------------------------------------------------
// segmentStore — implements SegmentIndex
// -----------------------------------------------------------------------

type segmentStore struct {
	db *clouddb.DB
}

// NewSegmentIndex constructs a SQL-backed SegmentIndex.
// In Postgres it queries camera_segment_index (partitioned).
// In SQLite it queries camera_segment_index_sqlite (unit test fallback).
func NewSegmentIndex(db *clouddb.DB) SegmentIndex {
	return &segmentStore{db: db}
}

func (s *segmentStore) ph(i int) string {
	return s.db.Placeholder(i)
}

func (s *segmentStore) tableName() string {
	if s.db.Dialect() == clouddb.DialectPostgres {
		return "camera_segment_index"
	}
	return "camera_segment_index_sqlite"
}

func (s *segmentStore) Append(ctx context.Context, seg Segment) error {
	if seg.TenantID == "" {
		return ErrInvalidTenantID
	}
	if seg.CameraID == "" {
		return fmt.Errorf("cameras.Append: camera_id is required")
	}
	if seg.Region == "" {
		seg.Region = clouddb.DefaultRegion
	}

	var q string
	if s.db.Dialect() == clouddb.DialectPostgres {
		q = fmt.Sprintf(
			`INSERT INTO %s
			    (camera_id, recorder_id, tenant_id, start_ts, end_ts,
			     file_path, file_size_bytes, storage_tier, checksum_sha256, region)
			 VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
			 ON CONFLICT (camera_id, start_ts) DO NOTHING`,
			s.tableName(),
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
			s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10),
		)
	} else {
		q = fmt.Sprintf(
			`INSERT OR IGNORE INTO %s
			    (camera_id, recorder_id, tenant_id, start_ts, end_ts,
			     file_path, file_size_bytes, storage_tier, checksum_sha256, region)
			 VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
			s.tableName(),
			s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5),
			s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10),
		)
	}

	_, err := s.db.ExecContext(ctx, q,
		seg.CameraID, seg.RecorderID, seg.TenantID,
		seg.StartTS, seg.EndTS,
		seg.FilePath, seg.FileSizeBytes, seg.StorageTier,
		nullString(seg.ChecksumSHA256), seg.Region,
	)
	if err != nil {
		return fmt.Errorf("segments.Append: %w", err)
	}
	return nil
}

func (s *segmentStore) QueryByTimeRange(
	ctx context.Context,
	tenantID, cameraID string,
	from, to time.Time,
) ([]Segment, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if cameraID == "" {
		return nil, fmt.Errorf("cameras.QueryByTimeRange: camera_id is required")
	}
	q := fmt.Sprintf(
		`SELECT camera_id, recorder_id, tenant_id, start_ts, end_ts,
		        file_path, file_size_bytes, storage_tier, checksum_sha256, region
		 FROM %s
		 WHERE tenant_id = %s AND camera_id = %s
		   AND start_ts >= %s AND end_ts <= %s
		 ORDER BY start_ts ASC`,
		s.tableName(),
		s.ph(1), s.ph(2), s.ph(3), s.ph(4),
	)
	rows, err := s.db.QueryContext(ctx, q, tenantID, cameraID, from, to)
	if err != nil {
		return nil, fmt.Errorf("segments.QueryByTimeRange: %w", err)
	}
	defer rows.Close()

	var out []Segment
	for rows.Next() {
		var seg Segment
		var checksum sql.NullString
		if err := rows.Scan(
			&seg.CameraID, &seg.RecorderID, &seg.TenantID,
			&seg.StartTS, &seg.EndTS,
			&seg.FilePath, &seg.FileSizeBytes, &seg.StorageTier,
			&checksum, &seg.Region,
		); err != nil {
			return nil, fmt.Errorf("segments scan: %w", err)
		}
		seg.ChecksumSHA256 = checksum.String
		out = append(out, seg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("segments rows: %w", err)
	}
	if out == nil {
		out = []Segment{}
	}
	return out, nil
}

// -----------------------------------------------------------------------
// null-coercion helpers
// -----------------------------------------------------------------------

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
