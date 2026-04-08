package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// fakeCrypto is a deterministic stub Cryptostore used in tests. It
// prepends a fixed tag + SHA-256(plaintext) + plaintext so round-trips
// are verifiable and tampering would surface as a decode error. This
// stands in for KAI-251's real cryptostore while it is being built.
type fakeCrypto struct {
	encCalls int
	decCalls int
}

const fakeTag = "FAKE:"

func (f *fakeCrypto) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	f.encCalls++
	sum := sha256.Sum256(plaintext)
	out := append([]byte(fakeTag), []byte(hex.EncodeToString(sum[:]))...)
	out = append(out, ':')
	out = append(out, plaintext...)
	return out, nil
}

func (f *fakeCrypto) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	f.decCalls++
	if len(ciphertext) < len(fakeTag) || string(ciphertext[:len(fakeTag)]) != fakeTag {
		return nil, errors.New("fakeCrypto: bad tag")
	}
	rest := ciphertext[len(fakeTag):]
	// rest = <hex sha256>:<plaintext>
	if len(rest) < 65 || rest[64] != ':' {
		return nil, errors.New("fakeCrypto: bad frame")
	}
	plaintext := rest[65:]
	sum := sha256.Sum256(plaintext)
	want := hex.EncodeToString(sum[:])
	if string(rest[:64]) != want {
		return nil, errors.New("fakeCrypto: auth mismatch")
	}
	return plaintext, nil
}

func newTestStore(t *testing.T) (*Store, *fakeCrypto) {
	t.Helper()
	dir := t.TempDir()
	crypto := &fakeCrypto{}
	s, err := Open(filepath.Join(dir, "state.db"), Options{Cryptostore: crypto})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, crypto
}

func sampleCamera(id string) AssignedCamera {
	return AssignedCamera{
		CameraID:      id,
		ConfigVersion: 1,
		RTSPPassword:  "secret-" + id,
		Config: CameraConfig{
			ID:                id,
			Name:              "Camera " + id,
			RTSPURL:           "rtsp://cam/" + id,
			RTSPUsername:      "user",
			ONVIFEndpoint:     "http://cam/onvif",
			ONVIFProfileToken: "Profile_1",
			PTZCapable:        true,
			RetentionDays:     7,
			Tags:              []string{"front", "outdoor"},
		},
	}
}

func TestMigrationsApplyCleanly(t *testing.T) {
	s, _ := newTestStore(t)
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v != len(migrations) {
		t.Fatalf("schema version = %d, want %d", v, len(migrations))
	}

	// Reopening an already-migrated DB must be a no-op.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	s2, err := Open(s.Path(), Options{Cryptostore: &fakeCrypto{}})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	v2, _ := s2.SchemaVersion()
	if v2 != v {
		t.Fatalf("schema version after reopen = %d, want %d", v2, v)
	}

	// All three tables must exist.
	for _, tbl := range []string{"assigned_cameras", "local_state", "segment_index"} {
		var n int
		if err := s2.DB().QueryRow(
			fmt.Sprintf(`SELECT COUNT(*) FROM %s`, tbl),
		).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}

func TestUpsertGetListRoundTrip(t *testing.T) {
	s, crypto := newTestStore(t)
	ctx := context.Background()

	cam := sampleCamera("cam-a")
	if err := s.UpsertCamera(ctx, cam); err != nil {
		t.Fatalf("UpsertCamera: %v", err)
	}
	if crypto.encCalls != 1 {
		t.Fatalf("Encrypt calls = %d, want 1", crypto.encCalls)
	}

	got, err := s.GetCamera(ctx, "cam-a")
	if err != nil {
		t.Fatalf("GetCamera: %v", err)
	}
	if got.CameraID != cam.CameraID ||
		got.Config.Name != cam.Config.Name ||
		got.RTSPPassword != cam.RTSPPassword ||
		got.ConfigVersion != cam.ConfigVersion ||
		got.Config.RTSPURL != cam.Config.RTSPURL ||
		got.Config.PTZCapable != cam.Config.PTZCapable ||
		got.Config.RetentionDays != cam.Config.RetentionDays ||
		len(got.Config.Tags) != 2 {
		t.Fatalf("GetCamera round-trip mismatch: %+v", got)
	}
	if got.AssignedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", got)
	}

	// Ensure the raw column is ciphertext (not plaintext).
	var cipher []byte
	if err := s.DB().QueryRow(
		`SELECT rtsp_credentials FROM assigned_cameras WHERE camera_id = ?`,
		"cam-a",
	).Scan(&cipher); err != nil {
		t.Fatalf("scan cipher: %v", err)
	}
	if string(cipher) == cam.RTSPPassword || len(cipher) == 0 {
		t.Fatalf("password not encrypted on disk: %q", cipher)
	}

	// Upsert again with a bumped version.
	cam.ConfigVersion = 2
	cam.Config.Name = "Renamed"
	if err := s.UpsertCamera(ctx, cam); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got2, _ := s.GetCamera(ctx, "cam-a")
	if got2.ConfigVersion != 2 || got2.Config.Name != "Renamed" {
		t.Fatalf("update not applied: %+v", got2)
	}

	// List two cameras.
	if err := s.UpsertCamera(ctx, sampleCamera("cam-b")); err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	list, err := s.ListAssigned(ctx)
	if err != nil {
		t.Fatalf("ListAssigned: %v", err)
	}
	if len(list) != 2 || list[0].CameraID != "cam-a" || list[1].CameraID != "cam-b" {
		t.Fatalf("ListAssigned = %+v", list)
	}

	// Remove.
	if err := s.RemoveCamera(ctx, "cam-a"); err != nil {
		t.Fatalf("RemoveCamera: %v", err)
	}
	if _, err := s.GetCamera(ctx, "cam-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	// Remove again must be a no-op.
	if err := s.RemoveCamera(ctx, "cam-a"); err != nil {
		t.Fatalf("RemoveCamera idempotent: %v", err)
	}
}

func TestMarkStatePushed(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	if err := s.UpsertCamera(ctx, sampleCamera("cam-a")); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	when := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	if err := s.MarkStatePushed(ctx, "cam-a", when); err != nil {
		t.Fatalf("MarkStatePushed: %v", err)
	}
	got, _ := s.GetCamera(ctx, "cam-a")
	if got.LastStatePushAt == nil || !got.LastStatePushAt.Equal(when) {
		t.Fatalf("LastStatePushAt = %v, want %v", got.LastStatePushAt, when)
	}
	if err := s.MarkStatePushed(ctx, "missing", when); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReconcileAssignments(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()

	// Seed cache with cam-a and cam-b.
	if err := s.UpsertCamera(ctx, sampleCamera("cam-a")); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := s.UpsertCamera(ctx, sampleCamera("cam-b")); err != nil {
		t.Fatalf("seed b: %v", err)
	}

	// Snapshot: keep cam-a unchanged, update cam-b (bumped version),
	// drop cam-a is not dropped... actually: unchanged cam-a, updated cam-b,
	// new cam-c, and cam-b in the cache stays, cam-a... wait let me rework.
	//
	// Simpler scenario:
	//   cam-a: unchanged
	//   cam-b: updated (version 1 -> 2, name changed)
	//   cam-c: added (new)
	//   cam-x: removed (was never in snapshot — but we need one present
	//          in the cache to remove, so add cam-x to cache first)
	camX := sampleCamera("cam-x")
	if err := s.UpsertCamera(ctx, camX); err != nil {
		t.Fatalf("seed x: %v", err)
	}
	// Attach a segment to cam-x to prove cascade on remove.
	if err := s.AppendSegment(ctx, Segment{
		CameraID: "cam-x",
		StartTS:  time.Unix(1000, 0).UTC(),
		EndTS:    time.Unix(1060, 0).UTC(),
		Path:     "/tmp/x1.mp4",
	}); err != nil {
		t.Fatalf("append x seg: %v", err)
	}

	camAUnchanged := sampleCamera("cam-a")
	camBUpdated := sampleCamera("cam-b")
	camBUpdated.ConfigVersion = 2
	camBUpdated.Config.Name = "Cam B renamed"
	camCNew := sampleCamera("cam-c")

	snapshot := []AssignedCamera{camAUnchanged, camBUpdated, camCNew}
	diff, err := s.ReconcileAssignments(ctx, snapshot)
	if err != nil {
		t.Fatalf("ReconcileAssignments: %v", err)
	}

	assertStrs := func(label string, got, want []string) {
		t.Helper()
		gotCopy := append([]string(nil), got...)
		wantCopy := append([]string(nil), want...)
		sort.Strings(gotCopy)
		sort.Strings(wantCopy)
		if len(gotCopy) != len(wantCopy) {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
		for i := range gotCopy {
			if gotCopy[i] != wantCopy[i] {
				t.Fatalf("%s = %v, want %v", label, got, want)
			}
		}
	}
	assertStrs("Added", diff.Added, []string{"cam-c"})
	assertStrs("Updated", diff.Updated, []string{"cam-b"})
	assertStrs("Removed", diff.Removed, []string{"cam-x"})
	assertStrs("Unchanged", diff.Unchanged, []string{"cam-a"})

	// Post-conditions:
	list, _ := s.ListAssigned(ctx)
	if len(list) != 3 {
		t.Fatalf("post-reconcile list len = %d, want 3", len(list))
	}
	gotB, _ := s.GetCamera(ctx, "cam-b")
	if gotB.ConfigVersion != 2 || gotB.Config.Name != "Cam B renamed" {
		t.Fatalf("cam-b not updated: %+v", gotB)
	}
	if _, err := s.GetCamera(ctx, "cam-x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cam-x should be removed: %v", err)
	}
	// cam-x's segments must have been cascade-deleted.
	segs, _ := s.QuerySegments(ctx, "cam-x",
		time.Unix(0, 0), time.Unix(9999, 0))
	if len(segs) != 0 {
		t.Fatalf("cam-x segments not cleaned: %+v", segs)
	}

	// Reconciling the same snapshot again must be a full no-op.
	diff2, err := s.ReconcileAssignments(ctx, snapshot)
	if err != nil {
		t.Fatalf("reconcile idempotent: %v", err)
	}
	if len(diff2.Added)+len(diff2.Updated)+len(diff2.Removed) != 0 {
		t.Fatalf("expected no-op reconcile, got %+v", diff2)
	}
	assertStrs("Unchanged2", diff2.Unchanged, []string{"cam-a", "cam-b", "cam-c"})
}

func TestLocalStateKV(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()

	type syncStatus struct {
		LastSync time.Time `json:"last_sync"`
		Token    string    `json:"token"`
	}
	in := syncStatus{LastSync: time.Unix(1700000000, 0).UTC(), Token: "abc"}
	if err := s.SetState(ctx, "sync", in); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	var out syncStatus
	if err := s.GetState(ctx, "sync", &out); err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if !out.LastSync.Equal(in.LastSync) || out.Token != in.Token {
		t.Fatalf("KV round-trip mismatch: %+v vs %+v", out, in)
	}

	// Overwrite.
	in.Token = "xyz"
	if err := s.SetState(ctx, "sync", in); err != nil {
		t.Fatalf("re-SetState: %v", err)
	}
	_ = s.GetState(ctx, "sync", &out)
	if out.Token != "xyz" {
		t.Fatalf("overwrite failed: %+v", out)
	}

	// Missing key.
	var tmp syncStatus
	if err := s.GetState(ctx, "missing", &tmp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Delete.
	if err := s.DeleteState(ctx, "sync"); err != nil {
		t.Fatalf("DeleteState: %v", err)
	}
	if err := s.GetState(ctx, "sync", &tmp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete didn't stick: %v", err)
	}
}

func TestSegmentIndexAppendAndQuery(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()

	base := time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC)
	segs := []Segment{
		{CameraID: "cam-a", StartTS: base, EndTS: base.Add(1 * time.Minute), Path: "/a/1.mp4", SizeBytes: 1000},
		{CameraID: "cam-a", StartTS: base.Add(1 * time.Minute), EndTS: base.Add(2 * time.Minute), Path: "/a/2.mp4", SizeBytes: 2000},
		{CameraID: "cam-a", StartTS: base.Add(2 * time.Minute), EndTS: base.Add(3 * time.Minute), Path: "/a/3.mp4", SizeBytes: 3000},
		{CameraID: "cam-b", StartTS: base, EndTS: base.Add(1 * time.Minute), Path: "/b/1.mp4", SizeBytes: 500},
	}
	for _, seg := range segs {
		if err := s.AppendSegment(ctx, seg); err != nil {
			t.Fatalf("AppendSegment: %v", err)
		}
	}

	// Idempotent re-append with updated fields.
	updated := segs[0]
	updated.SizeBytes = 1234
	if err := s.AppendSegment(ctx, updated); err != nil {
		t.Fatalf("re-append: %v", err)
	}

	got, err := s.QuerySegments(ctx, "cam-a",
		base, base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("QuerySegments: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d segments, want 3: %+v", len(got), got)
	}
	if got[0].SizeBytes != 1234 {
		t.Fatalf("idempotent update didn't apply: %d", got[0].SizeBytes)
	}
	// Check ordering.
	for i := 1; i < len(got); i++ {
		if got[i].StartTS.Before(got[i-1].StartTS) {
			t.Fatalf("unordered segments: %+v", got)
		}
	}

	// Partial overlap — range covers seg 2 middle only.
	mid, _ := s.QuerySegments(ctx, "cam-a",
		base.Add(90*time.Second), base.Add(100*time.Second))
	if len(mid) != 1 || mid[0].Path != "/a/2.mp4" {
		t.Fatalf("partial overlap = %+v", mid)
	}

	// Query before any segments.
	before, _ := s.QuerySegments(ctx, "cam-a",
		base.Add(-1*time.Hour), base.Add(-30*time.Minute))
	if len(before) != 0 {
		t.Fatalf("expected no segments, got %+v", before)
	}

	// Camera isolation.
	bSegs, _ := s.QuerySegments(ctx, "cam-b",
		base, base.Add(1*time.Minute+time.Second))
	if len(bSegs) != 1 || bSegs[0].CameraID != "cam-b" {
		t.Fatalf("camera isolation broken: %+v", bSegs)
	}

	// Upload flag round-trip.
	if err := s.MarkSegmentUploaded(ctx, "cam-a", base); err != nil {
		t.Fatalf("MarkSegmentUploaded: %v", err)
	}
	got2, _ := s.QuerySegments(ctx, "cam-a", base, base.Add(10*time.Second))
	if len(got2) != 1 || !got2[0].UploadedToCloudArchive {
		t.Fatalf("upload flag not set: %+v", got2)
	}
}

func TestConfigJSONStable(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()

	cam := sampleCamera("cam-stable")
	cam.Config.Extra = map[string]any{
		"future_field": "hello",
		"future_int":   float64(42),
	}
	if err := s.UpsertCamera(ctx, cam); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Raw JSON in the DB must deserialize back to exactly the same
	// CameraConfig — including Extra — across as many round-trips as
	// we want.
	var raw1 string
	if err := s.DB().QueryRow(
		`SELECT config FROM assigned_cameras WHERE camera_id = ?`, "cam-stable",
	).Scan(&raw1); err != nil {
		t.Fatalf("scan raw1: %v", err)
	}

	got, _ := s.GetCamera(ctx, "cam-stable")
	if err := s.UpsertCamera(ctx, got); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	var raw2 string
	if err := s.DB().QueryRow(
		`SELECT config FROM assigned_cameras WHERE camera_id = ?`, "cam-stable",
	).Scan(&raw2); err != nil {
		t.Fatalf("scan raw2: %v", err)
	}
	if raw1 != raw2 {
		t.Fatalf("config JSON unstable:\n%s\n---\n%s", raw1, raw2)
	}

	// Extra must survive the round-trip.
	if got.Config.Extra["future_field"] != "hello" {
		t.Fatalf("Extra lost: %+v", got.Config.Extra)
	}
}

func TestReconcileRemovesAllWhenSnapshotEmpty(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		if err := s.UpsertCamera(ctx, sampleCamera(id)); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	diff, err := s.ReconcileAssignments(ctx, nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(diff.Removed) != 3 {
		t.Fatalf("Removed = %v, want 3", diff.Removed)
	}
	list, _ := s.ListAssigned(ctx)
	if len(list) != 0 {
		t.Fatalf("post-empty-reconcile list = %+v", list)
	}
}

func TestNoopCryptostoreDefaultWhenNil(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "noop.db"), Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	cam := sampleCamera("cam-noop")
	if err := s.UpsertCamera(ctx, cam); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := s.GetCamera(ctx, "cam-noop")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RTSPPassword != cam.RTSPPassword {
		t.Fatalf("noop crypto round-trip failed: %q vs %q", got.RTSPPassword, cam.RTSPPassword)
	}
}
