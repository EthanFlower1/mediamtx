package streamclaims

import (
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// StreamKind is a bitfield describing which streaming capabilities a token
// grants. A single token can grant more than one kind (e.g. LIVE | AUDIO_TALKBACK).
//
// Verifiers MUST check that the requested kind bit is set; any unset bit is
// an insufficient-scope condition and MUST be rejected.
type StreamKind uint32

const (
	// StreamKindLive grants access to a real-time live stream.
	StreamKindLive StreamKind = 1 << iota
	// StreamKindPlayback grants access to recorded-video playback.
	// A PlaybackRange MUST accompany this kind.
	StreamKindPlayback
	// StreamKindSnapshot grants access to a single MJPEG snapshot frame.
	StreamKindSnapshot
	// StreamKindAudioTalkback grants the ability to push audio to the camera
	// speaker via the WebRTC data channel (requires StreamKindLive too).
	StreamKindAudioTalkback
)

// Has reports whether all bits in mask are set on k.
func (k StreamKind) Has(mask StreamKind) bool { return k&mask == mask }

// Protocol identifies the streaming protocol a token is scoped to.
// A token is valid only for the protocol it was minted for.
type Protocol string

const (
	// ProtocolWebRTC selects the WebRTC transport (default for live/talkback).
	ProtocolWebRTC Protocol = "webrtc"
	// ProtocolLLHLS selects Low-Latency HLS (live fallback).
	ProtocolLLHLS Protocol = "ll-hls"
	// ProtocolHLS selects standard HLS (default for playback).
	ProtocolHLS Protocol = "hls"
	// ProtocolMJPEG selects MJPEG (snapshots and grid thumbnails).
	ProtocolMJPEG Protocol = "mjpeg"
	// ProtocolRTSPTLS selects RTSP-over-TLS (power users and video wall).
	ProtocolRTSPTLS Protocol = "rtsp-tls"
)

// CameraID is the stable, tenant-scoped camera identifier.
type CameraID = string

// DirectoryID is the stable identifier for the Directory service instance
// that issued the token.
type DirectoryID = string

// RecorderID is re-exported from auth for convenience; callers can use either.
type RecorderID = auth.RecorderID

// UserID is re-exported from auth for convenience.
type UserID = auth.UserID

// TenantRef is re-exported from auth for convenience.
type TenantRef = auth.TenantRef

// TimeRange is a closed time interval used for PLAYBACK tokens.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MaxTTL is the hard upper bound on how far in the future ExpiresAt may be
// set relative to the issuance time. Issuers MUST NOT mint tokens with a
// longer TTL. Verifiers enforce this on presentation.
const MaxTTL = 5 * time.Minute

// StreamClaims carries the application claims embedded in a stream token.
// It is separate from the JWT standard claims (iss, aud, iat, nbf, jti, exp)
// which are handled by the [Issuer] and [Verifier] wrappers.
//
// All fields are required unless marked optional. A [Verifier] will reject any
// token that is missing a required field.
type StreamClaims struct {
	// UserID is the authenticated principal that requested the stream.
	// Derived from the session — never trusted from the request body.
	UserID UserID `json:"uid"`

	// TenantRef identifies the tenant whose camera is being accessed.
	// Every camera lookup and policy check is scoped to this tenant.
	TenantRef TenantRef `json:"tnt"`

	// CameraID is the stable camera identifier within the tenant.
	CameraID CameraID `json:"cam"`

	// RecorderID is the Recorder instance that holds this camera's stream.
	RecorderID RecorderID `json:"rec"`

	// DirectoryID identifies the Directory that minted the token.
	DirectoryID DirectoryID `json:"dir"`

	// Kind is a bitfield of [StreamKind] values authorised by this token.
	// At least one bit MUST be set.
	Kind StreamKind `json:"kind"`

	// Protocol is the transport protocol this token is scoped to.
	Protocol Protocol `json:"proto"`

	// PlaybackRange is the closed time interval for PLAYBACK tokens.
	// MUST be non-nil when Kind has [StreamKindPlayback]; MUST be nil for
	// all other kinds.
	PlaybackRange *TimeRange `json:"pbr,omitempty"`

	// ExpiresAt is when the token expires. The [Issuer] sets this; the
	// [Verifier] rejects tokens where time.Now() >= ExpiresAt.
	ExpiresAt time.Time `json:"exp_at"`

	// Nonce is a cryptographically random, base64url-encoded value used to
	// detect replays. KAI-257 (nonce bloom filter) checks uniqueness;
	// this package only generates and validates its presence.
	Nonce string `json:"nonce"`
}
