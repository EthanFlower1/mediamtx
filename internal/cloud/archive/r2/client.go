package r2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/bluenviron/mediamtx/internal/shared/cryptostore"
)

const (
	// DefaultPresignTTL is the default lifetime for presigned URLs.
	// Short TTL limits exposure window; callers can override via options.
	DefaultPresignTTL = 15 * time.Minute

	// r2EndpointTemplate is the S3-compatible endpoint format for Cloudflare R2.
	r2EndpointTemplate = "https://%s.r2.cloudflarestorage.com"
)

// ObjectMeta contains the metadata returned from a HeadObject call.
type ObjectMeta struct {
	ContentLength int64
	ContentType   string
	ETag          string
	LastModified  time.Time
	// UserMetadata contains custom headers stored with the object (e.g.,
	// x-amz-meta-tenant-id for belt-and-suspenders isolation checks).
	UserMetadata map[string]string
}

// PutOptions carries optional parameters for PutObject.
type PutOptions struct {
	// ContentType defaults to "video/mp4" if empty.
	ContentType string
	// Metadata is stored as object user-defined metadata.
	Metadata map[string]string
}

// CopyOptions carries optional parameters for CopyObject.
type CopyOptions struct {
	// DestMetadata overrides metadata on the destination object.
	// If nil, the source metadata is preserved.
	DestMetadata map[string]string
}

// Client is the R2 storage client. Wrap it — don't embed it — in higher-level
// services. It is safe for concurrent use from multiple goroutines.
//
// All methods that accept a KeySchema enforce cross-tenant isolation at the
// schema level (seam #4). Callers are responsible for also verifying tenant
// from the authenticated session before constructing KeySchema values.
type Client struct {
	cfg       Config
	s3        *s3.Client
	presigner *s3.PresignClient
	crypto    cryptostore.Cryptostore // nil when EncryptionMode != CSE-CMK
}

// NewClientWithEndpoint is identical to NewClient but overrides the R2 endpoint
// URL. Use this in tests to point the client at a local httptest.Server rather
// than the real Cloudflare R2 endpoint. Must not be called from production code.
func NewClientWithEndpoint(cfg Config, cs cryptostore.Cryptostore, endpointURL string) (*Client, error) {
	cfg.AccountID = "testoverride" // prevent real endpoint construction
	c, err := newClientWithEndpoint(cfg, cs, endpointURL)
	return c, err
}

// NewClient constructs and validates an R2 client from the given Config.
//
// For CSE-CMK mode, pass a non-nil cryptostore.Cryptostore derived from the
// tenant's master key (InfoCloudArchive context). The CSE-CMK Cryptostore is
// stored in the Client and used for every PutObject / GetObject call.
// Passing a nil cs when EncryptionMode is EncryptionModeCSECMK returns an error.
//
// Internally uses the AWS SDK v2 with a static-credentials provider pointed at
// the R2 S3-compatible endpoint. No AWS IAM is involved — R2 uses its own
// token system that is API-key compatible with the S3 auth scheme.
func NewClient(cfg Config, cs cryptostore.Cryptostore) (*Client, error) {
	endpoint := fmt.Sprintf(r2EndpointTemplate, cfg.AccountID)
	return newClientWithEndpoint(cfg, cs, endpoint)
}

// newClientWithEndpoint is the shared constructor used by both NewClient and
// NewClientWithEndpoint (test override).
func newClientWithEndpoint(cfg Config, cs cryptostore.Cryptostore, endpoint string) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	mode := cfg.EncryptionMode
	if mode == "" {
		mode = EncryptionModeStandard
	}
	if mode == EncryptionModeCSECMK && cs == nil {
		return nil, errors.New("r2: CSE-CMK encryption requires a non-nil cryptostore")
	}

	region := cfg.Region
	if region == "" {
		region = "auto"
	}

	s3Cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"", // session token — not used with R2
		)),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("r2: load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(s3Cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		// R2 is S3-compatible but uses path-style addressing.
		o.UsePathStyle = true
	})

	c := &Client{
		cfg:       cfg,
		s3:        s3Client,
		presigner: s3.NewPresignClient(s3Client),
		crypto:    cs,
	}
	// Normalise mode after validation.
	c.cfg.EncryptionMode = mode
	return c, nil
}

// PutObject uploads body to the given bucket and key, applying the client's
// configured encryption mode. For CSE-CMK the body is fully read, encrypted,
// and re-uploaded — callers should pass a bounded reader.
//
// Fail-closed: if encryption fails the upload is aborted and the error is
// returned without any bytes being written to R2.
func (c *Client) PutObject(ctx context.Context, bucket string, key KeySchema, body io.Reader, opts PutOptions) error {
	ct := opts.ContentType
	if ct == "" {
		ct = "video/mp4"
	}
	meta := mergeMetadata(opts.Metadata, map[string]string{
		"x-kaivue-tenant-id": key.TenantID,
	})

	switch c.cfg.EncryptionMode {
	case EncryptionModeStandard:
		return c.putStandard(ctx, bucket, key.String(), body, ct, meta)
	case EncryptionModeSSEKMS:
		return c.putSSEKMS(ctx, bucket, key.String(), body, ct, meta)
	case EncryptionModeCSECMK:
		return c.putCSECMK(ctx, bucket, key.String(), body, ct, meta)
	default:
		return fmt.Errorf("r2: unknown encryption mode %q", c.cfg.EncryptionMode)
	}
}

func (c *Client) putStandard(ctx context.Context, bucket, key string, body io.Reader, ct string, meta map[string]string) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(ct),
		Metadata:    meta,
	})
	if err != nil {
		return fmt.Errorf("r2: PutObject %s/%s: %w", bucket, key, err)
	}
	return nil
}

// putSSEKMS is a TODO stub. The interface is present; actual KMS header wiring
// requires the KMS key ARN resolution path (KAI-266 follow-up).
func (c *Client) putSSEKMS(ctx context.Context, bucket, key string, body io.Reader, ct string, meta map[string]string) error {
	// TODO(KAI-266): wire x-amz-server-side-encryption=aws:kms and
	// x-amz-server-side-encryption-aws-kms-key-id=c.cfg.KMSKeyARN
	return errors.New("r2: SSE-KMS mode not yet implemented (KAI-266 TODO)")
}

// putCSECMK reads body into memory, encrypts via the cryptostore (AES-256-GCM),
// then uploads the ciphertext. Fail-closed: any error aborts without upload.
func (c *Client) putCSECMK(ctx context.Context, bucket, key string, body io.Reader, ct string, meta map[string]string) error {
	plaintext, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("r2: CSE-CMK read plaintext: %w", err)
	}
	ciphertext, err := c.crypto.Encrypt(plaintext)
	if err != nil {
		// Fail-closed: never fall back to plaintext.
		return fmt.Errorf("r2: CSE-CMK encrypt: %w (upload aborted)", err)
	}
	// Mark the object as client-side encrypted so playback knows to decrypt.
	if meta == nil {
		meta = make(map[string]string)
	}
	meta["x-kaivue-encryption"] = "cse-cmk"

	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(ciphertext),
		ContentType:   aws.String(ct),
		ContentLength: aws.Int64(int64(len(ciphertext))),
		Metadata:      meta,
	})
	if err != nil {
		return fmt.Errorf("r2: CSE-CMK PutObject %s/%s: %w", bucket, key, err)
	}
	return nil
}

// GetObject downloads the object at key from bucket. For CSE-CMK mode, the
// returned ReadCloser contains plaintext after decryption. The caller must
// close the returned ReadCloser when done.
//
// Fail-closed: if decryption fails the error is returned without returning any
// plaintext bytes to the caller.
func (c *Client) GetObject(ctx context.Context, bucket string, key KeySchema) (io.ReadCloser, *ObjectMeta, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key.String()),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("r2: GetObject %s/%s: %w", bucket, key.String(), err)
	}
	meta := extractMeta(out.ContentLength, out.ContentType, out.ETag, out.LastModified, out.Metadata)

	if c.cfg.EncryptionMode == EncryptionModeCSECMK || out.Metadata["x-kaivue-encryption"] == "cse-cmk" {
		defer out.Body.Close()
		ciphertext, err := io.ReadAll(out.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("r2: CSE-CMK read ciphertext: %w", err)
		}
		plaintext, err := c.crypto.Decrypt(ciphertext)
		if err != nil {
			// Fail-closed: never return partial or undecrypted bytes.
			return nil, nil, fmt.Errorf("r2: CSE-CMK decrypt: %w (no plaintext returned)", err)
		}
		return io.NopCloser(bytesReader(plaintext)), meta, nil
	}
	return out.Body, meta, nil
}

// HeadObject retrieves metadata for the object at key without downloading the body.
func (c *Client) HeadObject(ctx context.Context, bucket string, key KeySchema) (*ObjectMeta, error) {
	out, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key.String()),
	})
	if err != nil {
		return nil, fmt.Errorf("r2: HeadObject %s/%s: %w", bucket, key.String(), err)
	}
	meta := extractMeta(out.ContentLength, out.ContentType, out.ETag, out.LastModified, out.Metadata)
	return meta, nil
}

// DeleteObject removes the object at key from bucket.
// Returns nil if the object does not exist (idempotent).
func (c *Client) DeleteObject(ctx context.Context, bucket string, key KeySchema) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key.String()),
	})
	if err != nil {
		return fmt.Errorf("r2: DeleteObject %s/%s: %w", bucket, key.String(), err)
	}
	return nil
}

// ListObjectsV2 lists objects under the given prefix. prefix must start with
// the tenant_id component to enforce tenant scoping. Returns at most maxKeys
// results per call; use the returned continuation token for pagination.
//
// The prefix is validated to start with the tenantID for cross-tenant protection.
func (c *Client) ListObjectsV2(ctx context.Context, bucket, tenantID, prefix string, maxKeys int32, continuationToken string) (keys []string, nextToken string, err error) {
	// Tenant-scope guard: the prefix must start with the tenant's own namespace.
	if !startsWith(prefix, tenantID+"/") && prefix != tenantID+"/" {
		return nil, "", fmt.Errorf("%w: list prefix %q does not start with tenant prefix %q",
			ErrCrossTenantKey, prefix, tenantID+"/")
	}

	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(maxKeys),
	}
	if continuationToken != "" {
		input.ContinuationToken = aws.String(continuationToken)
	}

	out, err := c.s3.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("r2: ListObjectsV2 %s/%s: %w", bucket, prefix, err)
	}

	for _, obj := range out.Contents {
		if obj.Key != nil {
			keys = append(keys, *obj.Key)
		}
	}
	if out.NextContinuationToken != nil {
		nextToken = *out.NextContinuationToken
	}
	return keys, nextToken, nil
}

// CopyObject copies an object within or across buckets without re-downloading.
// Typically used for tier transitions (hot→warm→cold) by KAI-267.
// The source key's tenant is verified to match the destination key's tenant.
func (c *Client) CopyObject(ctx context.Context, srcBucket string, srcKey KeySchema, dstBucket string, dstKey KeySchema, opts CopyOptions) error {
	if srcKey.TenantID != dstKey.TenantID {
		return fmt.Errorf("%w: cross-tenant copy rejected (src tenant %q != dst tenant %q)",
			ErrCrossTenantKey, srcKey.TenantID, dstKey.TenantID)
	}
	copySource := fmt.Sprintf("%s/%s", srcBucket, srcKey.String())
	input := &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucket),
		Key:        aws.String(dstKey.String()),
		CopySource: aws.String(copySource),
	}
	if opts.DestMetadata != nil {
		input.Metadata = opts.DestMetadata
		input.MetadataDirective = s3types.MetadataDirectiveReplace
	}
	_, err := c.s3.CopyObject(ctx, input)
	if err != nil {
		return fmt.Errorf("r2: CopyObject %s → %s/%s: %w", copySource, dstBucket, dstKey.String(), err)
	}
	return nil
}

// GeneratePresignedGetURL creates a time-limited GET URL for the object at key.
// The URL does not require R2 credentials to access — it carries a signed
// query string. Use this for playback: the Flutter client fetches segments
// directly from R2 without going through the cloud API.
//
// TTL must be > 0. For CSE-CMK segments the presigned URL returns ciphertext;
// the Flutter client is responsible for decryption using the tenant's local key.
func (c *Client) GeneratePresignedGetURL(ctx context.Context, bucket string, key KeySchema, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", errors.New("r2: presign TTL must be positive")
	}
	req, err := c.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key.String()),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("r2: PresignGetObject %s/%s: %w", bucket, key.String(), err)
	}
	return req.URL, nil
}

// GeneratePresignedPutURL creates a time-limited PUT URL for direct upload by
// a Recorder. The Recorder signs and uploads the segment directly to R2
// without routing the video bytes through the cloud API server.
//
// The cloud platform issues this URL after verifying the Recorder is authorized
// to upload on behalf of the given tenant (done by KAI-265 uploader).
// TTL must be > 0.
func (c *Client) GeneratePresignedPutURL(ctx context.Context, bucket string, key KeySchema, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", errors.New("r2: presign TTL must be positive")
	}
	req, err := c.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key.String()),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("r2: PresignPutObject %s/%s: %w", bucket, key.String(), err)
	}
	return req.URL, nil
}

// --- helpers -----------------------------------------------------------------

func extractMeta(contentLength *int64, contentType, eTag *string, lastModified *time.Time, raw map[string]string) *ObjectMeta {
	m := &ObjectMeta{
		UserMetadata: raw,
	}
	if contentLength != nil {
		m.ContentLength = *contentLength
	}
	if contentType != nil {
		m.ContentType = *contentType
	}
	if eTag != nil {
		m.ETag = *eTag
	}
	if lastModified != nil {
		m.LastModified = *lastModified
	}
	return m
}

func mergeMetadata(base, extra map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// bytesReader wraps a byte slice into an io.Reader compatible with the S3 SDK.
func bytesReader(b []byte) io.Reader {
	return &bytesReaderImpl{data: b}
}

type bytesReaderImpl struct {
	data []byte
	pos  int
}

func (r *bytesReaderImpl) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
