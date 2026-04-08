package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/bluenviron/mediamtx/internal/shared/runtime"
)

// RuntimeMode selects which Kaivue subsystems boot. It mirrors
// runtime.Mode in internal/shared/runtime and delegates validation so
// there is a single source of truth for the set of legal values.
//
// The empty string is treated as the legacy (pre-KAI-237) single-NVR
// mode so that existing mediamtx.yml files keep working unchanged.
type RuntimeMode string

// Runtime mode constants re-exported for use in the conf package.
const (
	RuntimeModeLegacy    RuntimeMode = RuntimeMode(runtime.ModeLegacy)
	RuntimeModeDirectory RuntimeMode = RuntimeMode(runtime.ModeDirectory)
	RuntimeModeRecorder  RuntimeMode = RuntimeMode(runtime.ModeRecorder)
	RuntimeModeAllInOne  RuntimeMode = RuntimeMode(runtime.ModeAllInOne)
)

// Runtime returns the shared runtime.Mode equivalent of this value.
func (m RuntimeMode) Runtime() runtime.Mode {
	return runtime.Mode(m)
}

// UnmarshalJSON implements json.Unmarshaler and rejects unknown modes.
func (m *RuntimeMode) UnmarshalJSON(b []byte) error {
	type alias RuntimeMode
	if err := jsonwrapper.Unmarshal(b, (*alias)(m)); err != nil {
		return err
	}
	if err := runtime.Mode(*m).Validate(); err != nil {
		return fmt.Errorf("invalid mode: %w", err)
	}
	return nil
}

// UnmarshalEnv implements env.Unmarshaler so MTX_MODE=recorder etc.
// works the same as the YAML field.
func (m *RuntimeMode) UnmarshalEnv(_ string, v string) error {
	return m.UnmarshalJSON([]byte(`"` + v + `"`))
}
