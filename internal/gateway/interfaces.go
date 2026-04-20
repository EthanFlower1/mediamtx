package gateway

import (
	"context"
	"errors"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/streamclaims"
)

// ErrRecorderNotFound is returned by a [RecorderResolver] when the
// requested Recorder is unknown to the Directory or has no mesh address.
// Callers MUST treat this as a 404, never as a 5xx.
var ErrRecorderNotFound = errors.New("gateway: recorder not found")

// ErrCameraNotFound is returned by a [RecorderResolver] when the
// requested camera does not exist in the tenant.
var ErrCameraNotFound = errors.New("gateway: camera not found")

// RecorderEndpoint is the resolved upstream coordinate for a single
// Recorder on the mesh. The [Service] uses it to mint a `source:` URL
// inside the Raikada sidecar config and to mint per-request playback
// URLs for clients.
//
// Host is the MagicDNS hostname under the recorder- prefix (matching
// internal/recorder/mesh.RoleHostnamePrefix), e.g. "recorder-abc123".
// MediaPort is the loopback-or-mesh port the Recorder's Raikada
// instance listens on for the requested protocol.
//
// Scheme is one of "rtsp", "rtsps", "http", "https" — picked by the
// resolver based on the camera's stream config and the requested
// streamclaims.Protocol.
type RecorderEndpoint struct {
	RecorderID streamclaims.RecorderID
	Host       string
	MediaPort  int
	Scheme     string
	// PathName is the Raikada path name on the upstream Recorder.
	// Typically equal to the camera id, but the resolver is free to
	// rename (e.g. tenant prefix) so the Gateway's Raikada never
	// has to know about the upstream layout.
	PathName string
}

// RecorderResolver maps an authenticated stream-claim to a concrete
// upstream RecorderEndpoint on the mesh.
//
// In v1 the production implementation is wired up by the composition
// root in cmd/mediamtx using internal/directory/db + internal/shared/mesh.
// This package only depends on the interface so the directory and gateway
// packages remain decoupled (boundary linter: seam #1).
//
// Implementations MUST be tenant-safe: a token issued for tenant A must
// never resolve to a Recorder owned by tenant B. The supplied claims
// already carry the verified tenant — the resolver is the authority on
// camera ownership.
type RecorderResolver interface {
	// Resolve looks up the Recorder that owns claims.CameraID within
	// claims.TenantRef and returns the upstream endpoint over the mesh.
	//
	// On success returns a non-nil endpoint and nil error.
	// On a missing camera returns ErrCameraNotFound.
	// On a missing or offline Recorder returns ErrRecorderNotFound.
	// All other errors are treated by the Service as 5xx.
	Resolve(ctx context.Context, claims *streamclaims.StreamClaims) (*RecorderEndpoint, error)

	// ListRecorders returns the full set of Recorder endpoints the
	// Gateway should pre-render into its Raikada sidecar config at
	// startup. The Gateway's Raikada has a `paths:` block populated
	// with one entry per RecorderEndpoint so that on-demand sources
	// can be pulled without restarting the sidecar.
	//
	// In v1 this is called once at boot. Future iterations may wire
	// it up to a Directory subscription (KAI-258) for live updates.
	ListRecorders(ctx context.Context) ([]RecorderEndpoint, error)
}

// StreamVerifier is the seam between the Gateway HTTP handler and the
// streamclaims package. The production implementation is a
// *streamclaims.Verifier; tests substitute a fake.
//
// Defining this interface (rather than taking *streamclaims.Verifier
// directly) keeps the Service unit tests free of JWT/JWKS plumbing.
type StreamVerifier interface {
	Verify(token string) (*streamclaims.StreamClaims, error)
}

// NonceChecker is the seam between the Gateway and KAI-257 (single-use
// nonce bloom filter). The streamclaims package deliberately does NOT
// enforce uniqueness; the Gateway must call CheckAndConsume after a
// successful Verify and BEFORE proxying any media bytes.
//
// Implementations MUST be safe for concurrent use.
type NonceChecker interface {
	// CheckAndConsume returns nil if the nonce has not been seen before
	// (and atomically marks it as consumed), or a non-nil error if the
	// nonce was already used. The error wraps ErrReplay so callers can
	// errors.Is for the replay path specifically.
	CheckAndConsume(ctx context.Context, nonce string, expiresAt int64) error
}

// ErrReplay is returned by a NonceChecker when a previously-seen nonce
// is presented. KAI-257 produces wrapped versions of this sentinel.
var ErrReplay = errors.New("gateway: stream nonce replay")

// Compile-time assertion that streamclaims.RecorderID and
// auth.RecorderID are the same underlying type. If this stops compiling
// the gateway HTTP handler must be updated.
var _ auth.RecorderID = streamclaims.RecorderID("")
