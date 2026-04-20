// Package runtime defines the Kaivue Recording Server runtime mode
// constants and the boot-mode dispatch used by cmd/mediamtx/main.go.
//
// Kaivue v1 splits what was previously a single "NVR" binary into
// three cooperating roles that are selected at boot via the top-level
// "mode:" field in mediamtx.yml:
//
//   - ModeDirectory  - the Directory server (admin UI, sidecar
//     supervisor, cloud Directory client). No capture pipeline.
//   - ModeRecorder   - the Recorder (capture pipeline, Raikada sidecar,
//     Directory client). No admin UI.
//   - ModeAllInOne   - both subsystems in-process, with automatic
//     pairing on first boot.
//
// ModeLegacy (the empty string) is the default and preserves the
// pre-KAI-237 single-NVR behavior bit-for-bit. Existing mediamtx.yml
// files that do not specify a mode MUST continue to work unchanged.
package runtime

import "fmt"

// Mode selects which Kaivue subsystems boot.
type Mode string

// Runtime modes.
const (
	// ModeLegacy is the implicit default when no mode is configured.
	// It boots the current single-binary NVR exactly as before so
	// that upgrading a running deployment is a no-op.
	ModeLegacy Mode = ""

	// ModeDirectory boots the Directory subsystem only.
	ModeDirectory Mode = "directory"

	// ModeRecorder boots the Recorder subsystem only.
	ModeRecorder Mode = "recorder"

	// ModeAllInOne boots both subsystems in a single process and
	// auto-pairs them on first boot.
	ModeAllInOne Mode = "all-in-one"
)

// IsValid reports whether the given mode is one of the known modes
// (including the empty legacy default).
func (m Mode) IsValid() bool {
	switch m {
	case ModeLegacy, ModeDirectory, ModeRecorder, ModeAllInOne:
		return true
	}
	return false
}

// String returns a human-friendly name for the mode, mapping the
// empty legacy mode to "legacy" for log output.
func (m Mode) String() string {
	if m == ModeLegacy {
		return "legacy"
	}
	return string(m)
}

// Validate returns an error if the mode is not one of the known values.
func (m Mode) Validate() error {
	if !m.IsValid() {
		return fmt.Errorf("invalid runtime mode %q: must be one of %q, %q, %q (or empty for legacy)",
			string(m), ModeDirectory, ModeRecorder, ModeAllInOne)
	}
	return nil
}

// Hooks holds the boot-time callbacks invoked by Dispatch. Each
// callback corresponds to one subsystem; callers inject their own
// implementations. Callbacks may be nil, in which case Dispatch is a
// no-op for that role (useful for tests).
type Hooks struct {
	// StartDirectory boots the Directory subsystem. Called for
	// ModeDirectory and ModeAllInOne.
	StartDirectory func() error

	// StartRecorder boots the Recorder subsystem. Called for
	// ModeRecorder and ModeAllInOne.
	StartRecorder func() error

	// AutoPair performs in-process pairing of a Recorder to a
	// Directory. Called once for ModeAllInOne after both subsystems
	// have started. Tracked in KAI-243 / KAI-244.
	AutoPair func() error

	// StartLegacy boots the pre-KAI-237 single-NVR code path. Called
	// for ModeLegacy to preserve existing behavior exactly.
	StartLegacy func() error
}

// Dispatch invokes the correct subsystem hooks for the given mode.
// It returns an error if the mode is invalid or any hook fails.
//
// This is an additive shim: today the only populated hook is
// StartLegacy, which forwards to the existing Raikada boot path.
// The Directory, Recorder, and AutoPair hooks are stubs that real
// wiring will fill in as KAI-246 (sidecar), KAI-226 (directory client),
// and KAI-243/244 (pairing) land.
func Dispatch(mode Mode, hooks Hooks) error {
	if err := mode.Validate(); err != nil {
		return err
	}

	switch mode {
	case ModeLegacy:
		if hooks.StartLegacy != nil {
			return hooks.StartLegacy()
		}
		return nil

	case ModeDirectory:
		// TODO(KAI-246, KAI-226): wire real Directory subsystem boot
		// (admin UI + sidecar supervisor + cloud Directory client).
		if hooks.StartDirectory != nil {
			return hooks.StartDirectory()
		}
		return nil

	case ModeRecorder:
		// TODO(KAI-246, KAI-226, KAI-250): wire real Recorder boot
		// (capture pipeline + Raikada sidecar + Directory client +
		// recorder-state SQLite).
		if hooks.StartRecorder != nil {
			return hooks.StartRecorder()
		}
		return nil

	case ModeAllInOne:
		// TODO(KAI-246): start Directory subsystem in-process.
		if hooks.StartDirectory != nil {
			if err := hooks.StartDirectory(); err != nil {
				return fmt.Errorf("all-in-one: start directory: %w", err)
			}
		}
		// TODO(KAI-246): start Recorder subsystem in-process.
		if hooks.StartRecorder != nil {
			if err := hooks.StartRecorder(); err != nil {
				return fmt.Errorf("all-in-one: start recorder: %w", err)
			}
		}
		// TODO(KAI-243, KAI-244): perform in-process auto-pairing on
		// first boot. No-op on subsequent boots once recorder-state
		// reports an existing pairing.
		if hooks.AutoPair != nil {
			if err := hooks.AutoPair(); err != nil {
				return fmt.Errorf("all-in-one: auto-pair: %w", err)
			}
		}
		return nil
	}

	// Unreachable because Validate rejects unknown modes above.
	return fmt.Errorf("runtime: unhandled mode %q", string(mode))
}
