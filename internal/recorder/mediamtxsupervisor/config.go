package mediamtxsupervisor

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/bluenviron/mediamtx/internal/recorder/state"
)

// PathConfig is the Recorder-side representation of a single Raikada
// path entry. It is intentionally a small subset of Raikada's full
// path schema — only the fields the Recorder actually drives.
//
// The field names match the Raikada HTTP API JSON shape so a
// PathConfig can be marshalled directly into a
// `/v3/config/paths/replace/{name}` request body. Fields the Recorder
// doesn't manage (encryption, fallbacks, custom hooks, ...) are simply
// absent and will be left at Raikada's defaults.
type PathConfig struct {
	// Name is the Raikada path name. It is stored separately
	// (not in the JSON body) because Raikada uses it as the URL
	// segment when applying a single path.
	Name string `json:"-"`

	// Source is the upstream the path pulls from. For ONVIF/RTSP
	// cameras this is the rtsp:// URL with credentials embedded.
	Source string `json:"source"`

	// SourceOnDemand asks Raikada to only dial the upstream when
	// at least one reader is connected. KAI-259 sets this to true
	// for every camera so an idle Recorder doesn't hammer cameras.
	SourceOnDemand bool `json:"sourceOnDemand"`

	// SourceOnDemandStartTimeout bounds how long Raikada waits
	// for the first frame after dialing on demand.
	SourceOnDemandStartTimeout string `json:"sourceOnDemandStartTimeout,omitempty"`

	// SourceOnDemandCloseAfter is how long Raikada keeps the
	// upstream open after the last reader disconnects.
	SourceOnDemandCloseAfter string `json:"sourceOnDemandCloseAfter,omitempty"`

	// Record enables Raikada's built-in recorder for this path.
	Record bool `json:"record"`

	// RecordPath, RecordFormat, RecordSegmentDuration, RecordPartDuration
	// are the recording knobs the Recorder exposes today. Empty string /
	// zero means "fall back to the Raikada-side default".
	RecordPath            string `json:"recordPath,omitempty"`
	RecordFormat          string `json:"recordFormat,omitempty"`
	RecordSegmentDuration string `json:"recordSegmentDuration,omitempty"`
	RecordPartDuration    string `json:"recordPartDuration,omitempty"`
	RecordDeleteAfter     string `json:"recordDeleteAfter,omitempty"`
}

// PathConfigSet is the full set of paths the supervisor wants Raikada
// to be running, keyed by path name and sorted for deterministic diffs.
type PathConfigSet struct {
	Paths []PathConfig
}

// Names returns the sorted list of path names in the set.
func (p PathConfigSet) Names() []string {
	out := make([]string, 0, len(p.Paths))
	for _, pc := range p.Paths {
		out = append(out, pc.Name)
	}
	sort.Strings(out)
	return out
}

// RenderOptions controls how cameras are turned into Raikada path
// entries. All fields are optional; sensible Recorder defaults are
// applied via withDefaults.
type RenderOptions struct {
	// PathPrefix is prepended to every camera_id to form the
	// Raikada path name. Default: "cam_".
	PathPrefix string

	// RecordPathTemplate is the value written to each path's
	// recordPath. Raikada expands its own %path / %Y / %m / %d
	// placeholders. Default:
	//   "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f"
	RecordPathTemplate string

	// RecordFormat is the segment container. Default: "fmp4".
	RecordFormat string

	// SegmentDuration is the target rotation interval. Default: "1h".
	SegmentDuration string

	// PartDuration controls fmp4 part flushing. Default: "1s".
	PartDuration string

	// OnDemandStartTimeout / OnDemandCloseAfter set the on-demand
	// dial / idle thresholds. Defaults: "10s" / "10s".
	OnDemandStartTimeout string
	OnDemandCloseAfter   string

	// RecordDeleteAfter, when non-empty, sets per-path automatic
	// deletion. The supervisor leaves it empty by default and lets
	// the retention service handle deletes; tests can pin it.
	RecordDeleteAfter string
}

func (o *RenderOptions) withDefaults() {
	if o.PathPrefix == "" {
		o.PathPrefix = "cam_"
	}
	if o.RecordPathTemplate == "" {
		o.RecordPathTemplate = "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f"
	}
	if o.RecordFormat == "" {
		o.RecordFormat = "fmp4"
	}
	if o.SegmentDuration == "" {
		o.SegmentDuration = "1h"
	}
	if o.PartDuration == "" {
		o.PartDuration = "1s"
	}
	if o.OnDemandStartTimeout == "" {
		o.OnDemandStartTimeout = "10s"
	}
	if o.OnDemandCloseAfter == "" {
		o.OnDemandCloseAfter = "10s"
	}
}

// RenderPaths converts a snapshot of assigned cameras into a
// PathConfigSet. Cameras with empty IDs or empty RTSP URLs are
// skipped (and an error is returned listing them) — the supervisor
// treats those as "drop this camera, keep the rest" so one bad row
// can't take down recording for the whole Recorder.
func RenderPaths(cams []state.AssignedCamera, opts RenderOptions) (PathConfigSet, error) {
	opts.withDefaults()

	var (
		out     PathConfigSet
		skipped []string
	)
	for _, cam := range cams {
		id := cam.CameraID
		if id == "" {
			id = cam.Config.ID
		}
		if id == "" {
			skipped = append(skipped, "<empty-id>")
			continue
		}
		if cam.Config.RTSPURL == "" {
			skipped = append(skipped, id+":no-rtsp-url")
			continue
		}
		src, err := buildSourceURL(cam)
		if err != nil {
			skipped = append(skipped, id+":"+err.Error())
			continue
		}
		out.Paths = append(out.Paths, PathConfig{
			Name:                       opts.PathPrefix + id,
			Source:                     src,
			SourceOnDemand:             true,
			SourceOnDemandStartTimeout: opts.OnDemandStartTimeout,
			SourceOnDemandCloseAfter:   opts.OnDemandCloseAfter,
			Record:                     true,
			RecordPath:                 opts.RecordPathTemplate,
			RecordFormat:               opts.RecordFormat,
			RecordSegmentDuration:      opts.SegmentDuration,
			RecordPartDuration:         opts.PartDuration,
			RecordDeleteAfter:          opts.RecordDeleteAfter,
		})
	}

	sort.Slice(out.Paths, func(i, j int) bool {
		return out.Paths[i].Name < out.Paths[j].Name
	})

	if len(skipped) > 0 {
		return out, &RenderError{Skipped: skipped}
	}
	return out, nil
}

// RenderError is returned by RenderPaths when one or more cameras had
// to be skipped. It is non-fatal — callers should still apply the
// returned (partial) PathConfigSet, then surface the error for logs.
type RenderError struct {
	Skipped []string
}

func (e *RenderError) Error() string {
	return fmt.Sprintf("mediamtxsupervisor: %d camera(s) skipped: %s",
		len(e.Skipped), strings.Join(e.Skipped, ", "))
}

// buildSourceURL takes an AssignedCamera and returns the rtsp:// URL
// to put in PathConfig.Source. RTSP credentials, when present, are
// embedded in the userinfo segment.
//
// We do *not* attempt to log or persist the formed URL: callers should
// treat it as a secret and rely on the standard slog redaction layer.
func buildSourceURL(cam state.AssignedCamera) (string, error) {
	raw := cam.Config.RTSPURL
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse rtsp url: %w", err)
	}
	if u.Scheme == "" {
		// Allow shorthand "host:554/stream" by re-parsing.
		u, err = url.Parse("rtsp://" + raw)
		if err != nil {
			return "", fmt.Errorf("parse rtsp url: %w", err)
		}
	}
	if cam.Config.RTSPUsername != "" {
		if cam.RTSPPassword != "" {
			u.User = url.UserPassword(cam.Config.RTSPUsername, cam.RTSPPassword)
		} else {
			u.User = url.User(cam.Config.RTSPUsername)
		}
	}
	return u.String(), nil
}
