package r2

import (
	"context"
	"fmt"
)

// CopyBetweenTiers copies a segment from one tier bucket to another without
// downloading the bytes to the cloud API server. This is a pure server-side
// S3 CopyObject operation.
//
// Typical usage (called by KAI-267 River jobs):
//
//	CopyBetweenTiers(ctx, "kaivue-prod-hot", "kaivue-prod-warm", key)
//
// The tenant_id embedded in key is verified to be consistent between src and
// dst (they share the same KeySchema so this is always true).
//
// Metadata is preserved from the source object (MetadataDirective=COPY).
// Content-Type is also preserved by default on R2 CopyObject.
func (c *Client) CopyBetweenTiers(ctx context.Context, srcBucket, dstBucket string, key KeySchema) error {
	if srcBucket == "" {
		return fmt.Errorf("r2: CopyBetweenTiers: srcBucket must not be empty")
	}
	if dstBucket == "" {
		return fmt.Errorf("r2: CopyBetweenTiers: dstBucket must not be empty")
	}
	if srcBucket == dstBucket {
		return fmt.Errorf("r2: CopyBetweenTiers: src and dst buckets are the same (%q)", srcBucket)
	}

	// CopyObject with nil DestMetadata → MetadataDirective=COPY (source preserved).
	return c.CopyObject(ctx, srcBucket, key, dstBucket, key, CopyOptions{})
}

// DeleteFromTier removes a segment from the given tier bucket. Call this after
// a successful CopyBetweenTiers to avoid paying for storage in both tiers.
//
// This is idempotent — deleting a non-existent object returns nil.
// KAI-267 River jobs should call CopyBetweenTiers first, verify the destination
// HeadObject confirms the copy, then call DeleteFromTier on the source.
func (c *Client) DeleteFromTier(ctx context.Context, bucket string, key KeySchema) error {
	if bucket == "" {
		return fmt.Errorf("r2: DeleteFromTier: bucket must not be empty")
	}
	return c.DeleteObject(ctx, bucket, key)
}
