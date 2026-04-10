package diagnostics

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Uploader implements Uploader using AWS S3 with lifecycle-based TTL.
type S3Uploader struct {
	Client     S3API
	Bucket     string
	PathPrefix string // optional prefix for all keys
}

// S3API is the subset of the S3 client used by the uploader. This enables
// testing with a mock.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// NewS3Uploader creates an S3Uploader.
func NewS3Uploader(client S3API, bucket, prefix string) *S3Uploader {
	return &S3Uploader{
		Client:     client,
		Bucket:     bucket,
		PathPrefix: prefix,
	}
}

// Upload puts an object in S3 with a tagging-based expiration hint. The actual
// 7-day lifecycle rule must be configured on the bucket (tag: auto-delete=true).
func (u *S3Uploader) Upload(ctx context.Context, key string, data io.Reader, size int64, ttl time.Duration) (string, error) {
	fullKey := u.fullKey(key)

	expires := time.Now().UTC().Add(ttl)

	input := &s3.PutObjectInput{
		Bucket:        aws.String(u.Bucket),
		Key:           aws.String(fullKey),
		Body:          data,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String("application/octet-stream"),
		Tagging:       aws.String("auto-delete=true"),
		Expires:       &expires,
		StorageClass:  s3types.StorageClassStandard,
	}

	if _, err := u.Client.PutObject(ctx, input); err != nil {
		return "", fmt.Errorf("s3 put %s: %w", fullKey, err)
	}

	// Return a simple S3 URI. Callers who need a presigned URL can generate
	// one separately.
	downloadURL := fmt.Sprintf("s3://%s/%s", u.Bucket, fullKey)
	return downloadURL, nil
}

// Delete removes an object from S3.
func (u *S3Uploader) Delete(ctx context.Context, key string) error {
	fullKey := u.fullKey(key)
	_, err := u.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(u.Bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", fullKey, err)
	}
	return nil
}

func (u *S3Uploader) fullKey(key string) string {
	if u.PathPrefix == "" {
		return key
	}
	return u.PathPrefix + "/" + key
}
