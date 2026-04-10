package whitelabel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	// Register decoders so image.DecodeConfig can sniff dimensions.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// BrandAssetStore persists versioned brand assets per tenant. Implementations
// are expected to be safe for concurrent use.
//
// Put always assigns a new monotonic version for the (tenantID, kind) pair
// and returns the resulting AssetRef. List enumerates every version of every
// kind for a tenant. Get fetches the bytes for a specific (kind, version).
type BrandAssetStore interface {
	Put(ctx context.Context, tenantID string, kind AssetKind, filename string, content io.Reader) (AssetRef, error)
	Get(ctx context.Context, tenantID string, kind AssetKind, version int) (AssetRef, io.ReadCloser, error)
	List(ctx context.Context, tenantID string) ([]AssetRef, error)
	Delete(ctx context.Context, tenantID string, kind AssetKind, version int) error
}

// Store-level errors. Callers should use errors.Is.
var (
	ErrAssetNotFound     = errors.New("whitelabel: asset not found")
	ErrAssetKindInvalid  = errors.New("whitelabel: invalid asset kind")
	ErrAssetTooLarge     = errors.New("whitelabel: asset exceeds max size")
	ErrAssetUnsafe       = errors.New("whitelabel: asset failed content sniff")
	ErrAssetDimensions   = errors.New("whitelabel: asset dimensions out of range")
	ErrAssetMIMEMismatch = errors.New("whitelabel: asset MIME type not allowed for kind")
)

// assetKindConstraints defines the per-kind validation limits. Dimension
// checks only apply to image kinds; fonts get byte-size and MIME validation.
type assetKindConstraints struct {
	MaxBytes        int64
	AllowedMIMEs    []string
	MinWidth        int
	MinHeight       int
	MaxWidth        int
	MaxHeight       int
	RequireSquare   bool
	RequireImage    bool
}

var constraints = map[AssetKind]assetKindConstraints{
	AssetKindLogo: {
		MaxBytes:     2 * 1024 * 1024,
		AllowedMIMEs: []string{"image/png", "image/jpeg", "image/gif"},
		MinWidth:     64, MinHeight: 64,
		MaxWidth: 4096, MaxHeight: 4096,
		RequireImage: true,
	},
	AssetKindSplash: {
		MaxBytes:     5 * 1024 * 1024,
		AllowedMIMEs: []string{"image/png", "image/jpeg"},
		MinWidth:     512, MinHeight: 512,
		MaxWidth: 4096, MaxHeight: 4096,
		RequireImage: true,
	},
	AssetKindIcon: {
		MaxBytes:     1 * 1024 * 1024,
		AllowedMIMEs: []string{"image/png"},
		MinWidth:     1024, MinHeight: 1024,
		MaxWidth: 1024, MaxHeight: 1024,
		RequireSquare: true,
		RequireImage:  true,
	},
	AssetKindFont: {
		MaxBytes: 4 * 1024 * 1024,
		// http.DetectContentType returns application/octet-stream for
		// most font binaries, so we allow it here and additionally
		// check the filename suffix in validateAsset.
		AllowedMIMEs: []string{
			"application/octet-stream",
			"font/ttf", "font/otf", "font/woff", "font/woff2",
		},
	},
}

// validateAsset runs size/MIME/dimension checks without committing to the
// store. Exported for handler-level pre-flight validation and for tests.
func validateAsset(kind AssetKind, filename string, data []byte) (contentType string, err error) {
	if !kind.Valid() {
		return "", fmt.Errorf("%w: %q", ErrAssetKindInvalid, kind)
	}
	c, ok := constraints[kind]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrAssetKindInvalid, kind)
	}
	if int64(len(data)) > c.MaxBytes {
		return "", fmt.Errorf("%w: kind=%s size=%d max=%d", ErrAssetTooLarge, kind, len(data), c.MaxBytes)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%w: empty body", ErrAssetUnsafe)
	}
	sniff := http.DetectContentType(data)
	// Normalise "image/png; charset=..." style content types.
	sniff = strings.SplitN(sniff, ";", 2)[0]

	mimeOK := false
	for _, allowed := range c.AllowedMIMEs {
		if allowed == sniff {
			mimeOK = true
			break
		}
	}
	if !mimeOK {
		return "", fmt.Errorf("%w: kind=%s sniffed=%s", ErrAssetMIMEMismatch, kind, sniff)
	}

	if kind == AssetKindFont {
		// Extra guard: refuse anything that sniffs as an executable
		// or script even if it slipped through the MIME allow-list.
		if strings.HasPrefix(sniff, "text/") || strings.Contains(sniff, "executable") {
			return "", fmt.Errorf("%w: font kind rejects %s", ErrAssetUnsafe, sniff)
		}
		lower := strings.ToLower(filename)
		okExt := false
		for _, ext := range []string{".ttf", ".otf", ".woff", ".woff2"} {
			if strings.HasSuffix(lower, ext) {
				okExt = true
				break
			}
		}
		if !okExt {
			return "", fmt.Errorf("%w: font filename must end in ttf/otf/woff/woff2, got %q", ErrAssetUnsafe, filename)
		}
		return sniff, nil
	}

	if c.RequireImage {
		cfg, _, derr := image.DecodeConfig(bytes.NewReader(data))
		if derr != nil {
			return "", fmt.Errorf("%w: decode: %v", ErrAssetUnsafe, derr)
		}
		if cfg.Width < c.MinWidth || cfg.Height < c.MinHeight ||
			cfg.Width > c.MaxWidth || cfg.Height > c.MaxHeight {
			return "", fmt.Errorf("%w: kind=%s %dx%d (min %dx%d max %dx%d)",
				ErrAssetDimensions, kind, cfg.Width, cfg.Height,
				c.MinWidth, c.MinHeight, c.MaxWidth, c.MaxHeight)
		}
		if c.RequireSquare && cfg.Width != cfg.Height {
			return "", fmt.Errorf("%w: icon must be square, got %dx%d", ErrAssetDimensions, cfg.Width, cfg.Height)
		}
	}

	return sniff, nil
}

// MemoryAssetStore is an in-memory fake used by tests and the KAI-353 API
// round-trip. It does not attempt crash safety. All validation happens at
// Put time so the handler layer can stay thin.
type MemoryAssetStore struct {
	mu      sync.RWMutex
	objects map[string]map[AssetKind][]memoryObject
	nowFn   func() time.Time
}

type memoryObject struct {
	ref  AssetRef
	data []byte
}

// NewMemoryAssetStore returns an empty store.
func NewMemoryAssetStore() *MemoryAssetStore {
	return &MemoryAssetStore{
		objects: make(map[string]map[AssetKind][]memoryObject),
		nowFn:   time.Now,
	}
}

// Put validates and stores content, assigning the next monotonic version for
// the (tenant, kind) pair.
func (s *MemoryAssetStore) Put(_ context.Context, tenantID string, kind AssetKind, filename string, content io.Reader) (AssetRef, error) {
	if strings.TrimSpace(tenantID) == "" {
		return AssetRef{}, ErrMissingTenantID
	}
	buf, err := io.ReadAll(content)
	if err != nil {
		return AssetRef{}, fmt.Errorf("whitelabel: read content: %w", err)
	}
	ct, err := validateAsset(kind, filename, buf)
	if err != nil {
		return AssetRef{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects[tenantID] == nil {
		s.objects[tenantID] = make(map[AssetKind][]memoryObject)
	}
	existing := s.objects[tenantID][kind]
	version := len(existing) + 1
	ref := AssetRef{
		Kind:        kind,
		StorageKey:  fmt.Sprintf("%s/brand/v%d/%s", tenantID, version, string(kind)),
		ContentType: ct,
		SizeBytes:   int64(len(buf)),
		Version:     version,
		UploadedAt:  s.nowFn().UTC(),
	}
	s.objects[tenantID][kind] = append(existing, memoryObject{ref: ref, data: buf})
	return ref, nil
}

// Get returns the AssetRef and a reader for the stored bytes.
func (s *MemoryAssetStore) Get(_ context.Context, tenantID string, kind AssetKind, version int) (AssetRef, io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := s.objects[tenantID][kind]
	for _, obj := range versions {
		if obj.ref.Version == version {
			return obj.ref, io.NopCloser(bytes.NewReader(obj.data)), nil
		}
	}
	return AssetRef{}, nil, ErrAssetNotFound
}

// List returns every asset ref for the tenant ordered by kind then version.
func (s *MemoryAssetStore) List(_ context.Context, tenantID string) ([]AssetRef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []AssetRef
	for _, perKind := range s.objects[tenantID] {
		for _, obj := range perKind {
			out = append(out, obj.ref)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// Delete removes a specific version. Remaining versions keep their original
// version numbers so historical brand configs still resolve.
func (s *MemoryAssetStore) Delete(_ context.Context, tenantID string, kind AssetKind, version int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.objects[tenantID][kind]
	for i, obj := range versions {
		if obj.ref.Version == version {
			s.objects[tenantID][kind] = append(versions[:i], versions[i+1:]...)
			return nil
		}
	}
	return ErrAssetNotFound
}

// S3AssetStore is a stub for the real backend.
//
// TODO(lead-cloud): KAI-355 will implement this against S3/R2 with KMS
// envelope encryption and signed URL issuance. Until then this type exists
// so callers can dependency-inject it without importing AWS SDKs.
type S3AssetStore struct {
	Bucket   string
	Endpoint string
	Region   string
}

// Put is not implemented. Callers must use MemoryAssetStore until KAI-355.
func (s *S3AssetStore) Put(context.Context, string, AssetKind, string, io.Reader) (AssetRef, error) {
	return AssetRef{}, errors.New("whitelabel: S3AssetStore not implemented (KAI-355)")
}

// Get is not implemented.
func (s *S3AssetStore) Get(context.Context, string, AssetKind, int) (AssetRef, io.ReadCloser, error) {
	return AssetRef{}, nil, errors.New("whitelabel: S3AssetStore not implemented (KAI-355)")
}

// List is not implemented.
func (s *S3AssetStore) List(context.Context, string) ([]AssetRef, error) {
	return nil, errors.New("whitelabel: S3AssetStore not implemented (KAI-355)")
}

// Delete is not implemented.
func (s *S3AssetStore) Delete(context.Context, string, AssetKind, int) error {
	return errors.New("whitelabel: S3AssetStore not implemented (KAI-355)")
}

// compile-time interface checks
var (
	_ BrandAssetStore = (*MemoryAssetStore)(nil)
	_ BrandAssetStore = (*S3AssetStore)(nil)
)
