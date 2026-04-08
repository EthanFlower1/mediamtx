package r2_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/cloud/archive/r2"
	"github.com/bluenviron/mediamtx/internal/shared/cryptostore"
)

// ----------------------------------------------------------------------------
// Fake S3-compatible server (in-process, no Docker / localstack required)
// ----------------------------------------------------------------------------

// fakeS3 is a minimal in-memory S3-compatible server that speaks just enough
// of the S3 HTTP wire protocol to exercise the r2.Client.
type fakeS3 struct {
	mu      sync.RWMutex
	objects map[string]fakeObject // key: "bucket/object-key"
}

type fakeObject struct {
	body     []byte
	metadata map[string]string
	ct       string
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: make(map[string]fakeObject)}
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Path-style: /{bucket}/{key...}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(parts) < 2 {
		http.Error(w, "need bucket/key", http.StatusBadRequest)
		return
	}
	bucket, key := parts[0], parts[1]
	storeKey := bucket + "/" + key

	switch r.Method {
	case http.MethodPut:
		// Handle CopyObject (has x-amz-copy-source header).
		if copySource := r.Header.Get("x-amz-copy-source"); copySource != "" {
			f.mu.RLock()
			src, ok := f.objects[copySource]
			f.mu.RUnlock()
			if !ok {
				http.Error(w, "NoSuchKey", http.StatusNotFound)
				return
			}
			f.mu.Lock()
			f.objects[storeKey] = src
			f.mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprintf(w, `<CopyObjectResult><ETag>"copied"</ETag></CopyObjectResult>`)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		meta := make(map[string]string)
		for k, vs := range r.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") {
				meta[strings.ToLower(k[len("x-amz-meta-"):])] = vs[0]
			}
		}
		f.mu.Lock()
		f.objects[storeKey] = fakeObject{body: body, metadata: meta, ct: r.Header.Get("Content-Type")}
		f.mu.Unlock()
		w.Header().Set("ETag", `"fakeetag"`)
		w.WriteHeader(http.StatusOK)

	case http.MethodGet:
		// Presigned URL check: no auth check in the fake (just serve the object).
		f.mu.RLock()
		obj, ok := f.objects[storeKey]
		f.mu.RUnlock()
		if !ok {
			http.Error(w, "NoSuchKey", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", obj.ct)
		w.Header().Set("ETag", `"fakeetag"`)
		for k, v := range obj.metadata {
			w.Header().Set("x-amz-meta-"+k, v)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(obj.body) //nolint:errcheck

	case http.MethodHead:
		f.mu.RLock()
		obj, ok := f.objects[storeKey]
		f.mu.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", obj.ct)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(obj.body)))
		w.Header().Set("ETag", `"fakeetag"`)
		w.WriteHeader(http.StatusOK)

	case http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, storeKey)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ----------------------------------------------------------------------------
// Test helpers
// ----------------------------------------------------------------------------

// clientForServer builds an r2.Client pointed at the fake httptest server.
// For SSE-KMS mode a stub KMSKeyARN is automatically populated so Validate passes.
func clientForServer(t *testing.T, srv *httptest.Server, encMode r2.EncryptionMode, cs cryptostore.Cryptostore) *r2.Client {
	t.Helper()
	cfg := r2.Config{
		AccountID:       "testaccount",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		Env:             "test",
		Region:          "auto",
		EncryptionMode:  encMode,
	}
	// SSE-KMS requires a KMS key ARN to pass validation. The stub test
	// exercises PutObject which returns "not implemented" before using the ARN.
	if encMode == r2.EncryptionModeSSEKMS {
		cfg.KMSKeyARN = "arn:aws:kms:us-east-1:000000000000:key/test-stub"
	}
	client, err := r2.NewClientWithEndpoint(cfg, cs, srv.URL)
	require.NoError(t, err)
	return client
}

func makeKey(t *testing.T, tenantID string) r2.KeySchema {
	t.Helper()
	k, err := r2.NewKeySchema(tenantID, "dir-1", "cam-1", time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC), uuid.New())
	require.NoError(t, err)
	return k
}

func makeCryptostore(t *testing.T) cryptostore.Cryptostore {
	t.Helper()
	// 32-byte key — deterministic for test reproducibility.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	cs, err := cryptostore.New(key)
	require.NoError(t, err)
	return cs
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

// TestRoundTrip_Standard exercises put → head → get → delete.
func TestRoundTrip_Standard(t *testing.T) {
	srv := httptest.NewServer(newFakeS3())
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeStandard, nil)
	ctx := context.Background()
	bucket := "kaivue-test-hot"
	key := makeKey(t, "tenant-a")
	payload := []byte("fake-mp4-payload")

	// PutObject
	err := client.PutObject(ctx, bucket, key, bytes.NewReader(payload), r2.PutOptions{})
	require.NoError(t, err)

	// HeadObject
	meta, err := client.HeadObject(ctx, bucket, key)
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), meta.ContentLength)

	// GetObject
	rc, _, err := client.GetObject(ctx, bucket, key)
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	// DeleteObject
	err = client.DeleteObject(ctx, bucket, key)
	require.NoError(t, err)

	// Head after delete → not found.
	_, err = client.HeadObject(ctx, bucket, key)
	require.Error(t, err)
}

// TestRoundTrip_CSECMK exercises put → get with client-side encryption.
// The bytes stored in the fake S3 must be ciphertext (not plaintext).
func TestRoundTrip_CSECMK(t *testing.T) {
	fake := newFakeS3()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	cs := makeCryptostore(t)
	client := clientForServer(t, srv, r2.EncryptionModeCSECMK, cs)
	ctx := context.Background()
	bucket := "kaivue-test-hot"
	key := makeKey(t, "tenant-b")
	payload := []byte("sensitive-video-data")

	err := client.PutObject(ctx, bucket, key, bytes.NewReader(payload), r2.PutOptions{})
	require.NoError(t, err)

	// Verify the stored bytes are NOT plaintext.
	storeKey := bucket + "/" + key.String()
	fake.mu.RLock()
	stored := fake.objects[storeKey].body
	fake.mu.RUnlock()
	assert.NotEqual(t, payload, stored, "stored bytes should be ciphertext, not plaintext")
	assert.Greater(t, len(stored), len(payload), "ciphertext should be longer than plaintext due to nonce+tag")

	// GetObject must decrypt and return the original plaintext.
	rc, _, err := client.GetObject(ctx, bucket, key)
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, payload, got, "decrypted plaintext must match original")
}

// TestRoundTrip_SSEKMSSTUB verifies that SSE-KMS mode returns a not-implemented error.
func TestRoundTrip_SSEKMSStub(t *testing.T) {
	srv := httptest.NewServer(newFakeS3())
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeSSEKMS, nil)
	ctx := context.Background()
	key := makeKey(t, "tenant-c")

	err := client.PutObject(ctx, "kaivue-test-hot", key, strings.NewReader("data"), r2.PutOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SSE-KMS", "error should identify the unimplemented mode")
}

// TestCSECMK_FailClosed verifies that a broken cryptostore aborts the upload.
func TestCSECMK_FailClosed(t *testing.T) {
	srv := httptest.NewServer(newFakeS3())
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeCSECMK, &brokenCryptostore{})
	ctx := context.Background()
	key := makeKey(t, "tenant-d")

	err := client.PutObject(ctx, "kaivue-test-hot", key, strings.NewReader("data"), r2.PutOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CSE-CMK encrypt")
	assert.Contains(t, err.Error(), "upload aborted")
}

// brokenCryptostore always returns an error from Encrypt.
type brokenCryptostore struct{}

func (b *brokenCryptostore) Encrypt([]byte) ([]byte, error) {
	return nil, fmt.Errorf("simulated KMS unavailable")
}
func (b *brokenCryptostore) Decrypt([]byte) ([]byte, error) {
	return nil, fmt.Errorf("simulated KMS unavailable")
}
func (b *brokenCryptostore) RotateKey(_, _ []byte) error { return nil }

// TestPresignedURLGeneration exercises both PUT and GET presigned URL paths.
func TestPresignedURLGeneration(t *testing.T) {
	srv := httptest.NewServer(newFakeS3())
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeStandard, nil)
	ctx := context.Background()
	key := makeKey(t, "tenant-e")

	putURL, err := client.GeneratePresignedPutURL(ctx, "kaivue-test-hot", key, 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, putURL)
	assert.Contains(t, putURL, key.TenantID, "URL should contain the tenant prefix")

	getURL, err := client.GeneratePresignedGetURL(ctx, "kaivue-test-hot", key, 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, getURL)
}

// TestPresignedURL_ZeroTTL checks that presign rejects zero or negative TTL.
func TestPresignedURL_ZeroTTL(t *testing.T) {
	srv := httptest.NewServer(newFakeS3())
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeStandard, nil)
	ctx := context.Background()
	key := makeKey(t, "tenant-f")

	_, err := client.GeneratePresignedGetURL(ctx, "kaivue-test-hot", key, 0)
	require.Error(t, err)

	_, err = client.GeneratePresignedPutURL(ctx, "kaivue-test-hot", key, -1*time.Second)
	require.Error(t, err)
}

// TestCopyBetweenTiers verifies that content and metadata survive a tier copy.
func TestCopyBetweenTiers(t *testing.T) {
	fake := newFakeS3()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeStandard, nil)
	ctx := context.Background()
	key := makeKey(t, "tenant-g")
	payload := []byte("segment-bytes")

	err := client.PutObject(ctx, "kaivue-test-hot", key, bytes.NewReader(payload), r2.PutOptions{
		Metadata: map[string]string{"camera": "cam-1"},
	})
	require.NoError(t, err)

	err = client.CopyBetweenTiers(ctx, "kaivue-test-hot", "kaivue-test-warm", key)
	require.NoError(t, err)

	// Verify destination has the content.
	rc, _, err := client.GetObject(ctx, "kaivue-test-warm", key)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	rc.Close()
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

// TestDeleteFromTier verifies idempotent delete after copy.
func TestDeleteFromTier(t *testing.T) {
	srv := httptest.NewServer(newFakeS3())
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeStandard, nil)
	ctx := context.Background()
	key := makeKey(t, "tenant-h")

	// Delete a non-existent key — must be idempotent (no error).
	err := client.DeleteFromTier(ctx, "kaivue-test-hot", key)
	require.NoError(t, err)
}

// TestCrossTenantIsolation_ListPrefix verifies that listing a prefix outside
// the caller's tenant_id namespace is rejected before any network call.
func TestCrossTenantIsolation_ListPrefix(t *testing.T) {
	srv := httptest.NewServer(newFakeS3())
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeStandard, nil)
	ctx := context.Background()

	// Tenant A trying to list tenant B's prefix → must be rejected.
	_, _, err := client.ListObjectsV2(ctx, "kaivue-test-hot", "tenant-a", "tenant-b/dir-1/", 100, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, r2.ErrCrossTenantKey)
}

// TestCrossTenantCopyRejected verifies that copying from one tenant's key to
// another tenant's key is rejected by CopyObject.
func TestCrossTenantCopyRejected(t *testing.T) {
	srv := httptest.NewServer(newFakeS3())
	defer srv.Close()

	client := clientForServer(t, srv, r2.EncryptionModeStandard, nil)
	ctx := context.Background()

	srcKey := makeKey(t, "tenant-a")
	dstKey := makeKey(t, "tenant-b")

	err := client.CopyObject(ctx, "kaivue-test-hot", srcKey, "kaivue-test-warm", dstKey, r2.CopyOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, r2.ErrCrossTenantKey)
}

// ----------------------------------------------------------------------------
// KeySchema tests
// ----------------------------------------------------------------------------

// TestKeySchema_String validates the canonical key format.
func TestKeySchema_String(t *testing.T) {
	segID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	k, err := r2.NewKeySchema("tenant-x", "dir-001", "cam-001",
		time.Date(2026, time.April, 8, 15, 30, 0, 0, time.UTC), segID)
	require.NoError(t, err)
	assert.Equal(t, "tenant-x/dir-001/cam-001/2026/04/08/01234567-89ab-cdef-0123-456789abcdef.mp4", k.String())
}

// TestKeySchema_ParseRoundTrip validates parse → string → parse identity.
func TestKeySchema_ParseRoundTrip(t *testing.T) {
	segID := uuid.New()
	original, err := r2.NewKeySchema("tenant-y", "dir-002", "cam-002",
		time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC), segID)
	require.NoError(t, err)

	parsed, err := r2.ParseKeySchema(original.String())
	require.NoError(t, err)
	assert.Equal(t, original.TenantID, parsed.TenantID)
	assert.Equal(t, original.DirectoryID, parsed.DirectoryID)
	assert.Equal(t, original.CameraID, parsed.CameraID)
	assert.Equal(t, original.SegmentUUID, parsed.SegmentUUID)
	assert.Equal(t, original.Date.UTC(), parsed.Date.UTC())
}

// TestKeySchema_RejectMalformed ensures path-injection attempts are rejected.
func TestKeySchema_RejectMalformed(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"missing segments", "tenant-a/dir/cam/2026/04"},
		{"dotdot injection", "../../../etc/passwd/2026/04/08/" + uuid.New().String() + ".mp4"},
		{"no mp4 suffix", "tenant-a/dir/cam/2026/04/08/" + uuid.New().String() + ".ts"},
		{"invalid uuid", "tenant-a/dir/cam/2026/04/08/not-a-uuid.mp4"},
		{"invalid month", "tenant-a/dir/cam/2026/99/08/" + uuid.New().String() + ".mp4"},
		{"empty tenant", "/dir/cam/2026/04/08/" + uuid.New().String() + ".mp4"},
		{"slash in component", "tenant/a/dir/cam/2026/04/08/" + uuid.New().String() + ".mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r2.ParseKeySchema(tc.key)
			require.Error(t, err, "expected parse to fail for %q", tc.key)
			assert.ErrorIs(t, err, r2.ErrInvalidKey)
		})
	}
}

// TestKeySchema_RejectBadComponents ensures NewKeySchema rejects invalid ID chars.
func TestKeySchema_RejectBadComponents(t *testing.T) {
	seg := uuid.New()
	when := time.Now()

	// Path traversal in tenant_id.
	_, err := r2.NewKeySchema("../evil", "dir", "cam", when, seg)
	require.Error(t, err)
	assert.ErrorIs(t, err, r2.ErrInvalidKey)

	// Slash in directory_id.
	_, err = r2.NewKeySchema("tenant", "dir/evil", "cam", when, seg)
	require.Error(t, err)
	assert.ErrorIs(t, err, r2.ErrInvalidKey)

	// Empty camera_id.
	_, err = r2.NewKeySchema("tenant", "dir", "", when, seg)
	require.Error(t, err)
	assert.ErrorIs(t, err, r2.ErrInvalidKey)
}

// TestKeySchema_AssertTenant is the regression test for cross-tenant key
// injection (seam #4). This must never be deleted.
func TestKeySchema_AssertTenant(t *testing.T) {
	segID := uuid.New()
	k, err := r2.NewKeySchema("tenant-a", "dir", "cam", time.Now(), segID)
	require.NoError(t, err)

	// Correct tenant passes.
	require.NoError(t, k.AssertTenant("tenant-a"))

	// Wrong tenant returns ErrCrossTenantKey.
	err = k.AssertTenant("tenant-b")
	require.Error(t, err)
	assert.ErrorIs(t, err, r2.ErrCrossTenantKey)
}

// TestBucketName verifies the naming convention.
func TestBucketName(t *testing.T) {
	cfg := r2.Config{Env: "prod"}
	assert.Equal(t, "kaivue-prod-hot", r2.BucketName(cfg, r2.TierHot))
	assert.Equal(t, "kaivue-prod-warm", r2.BucketName(cfg, r2.TierWarm))
	assert.Equal(t, "kaivue-prod-cold", r2.BucketName(cfg, r2.TierCold))
}

// TestConfig_Validate covers required field enforcement.
func TestConfig_Validate(t *testing.T) {
	good := r2.Config{
		AccountID:      "acct",
		AccessKeyID:    "key",
		SecretAccessKey: "secret",
		Env:            "prod",
		EncryptionMode: r2.EncryptionModeStandard,
	}
	require.NoError(t, good.Validate())

	bad := good
	bad.AccountID = ""
	require.Error(t, bad.Validate())

	bad2 := good
	bad2.EncryptionMode = r2.EncryptionModeSSEKMS // KMSKeyARN missing
	require.Error(t, bad2.Validate())

	bad3 := good
	bad3.EncryptionMode = "unknown-mode"
	require.Error(t, bad3.Validate())
}
