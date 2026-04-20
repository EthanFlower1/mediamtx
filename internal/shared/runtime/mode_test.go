package runtime

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModeIsValid(t *testing.T) {
	cases := []struct {
		mode Mode
		want bool
	}{
		{ModeLegacy, true},
		{ModeDirectory, true},
		{ModeRecorder, true},
		{ModeAllInOne, true},
		{Mode("bogus"), false},
		{Mode("DIRECTORY"), false}, // case-sensitive
		{Mode("all_in_one"), false},
	}
	for _, c := range cases {
		require.Equalf(t, c.want, c.mode.IsValid(), "mode=%q", c.mode)
	}
}

func TestModeValidate(t *testing.T) {
	require.NoError(t, ModeLegacy.Validate())
	require.NoError(t, ModeDirectory.Validate())
	require.NoError(t, ModeRecorder.Validate())
	require.NoError(t, ModeAllInOne.Validate())
	require.Error(t, Mode("junk").Validate())
}

func TestModeString(t *testing.T) {
	require.Equal(t, "legacy", ModeLegacy.String())
	require.Equal(t, "directory", ModeDirectory.String())
	require.Equal(t, "recorder", ModeRecorder.String())
	require.Equal(t, "all-in-one", ModeAllInOne.String())
}

func TestDispatchInvalidMode(t *testing.T) {
	err := Dispatch(Mode("wat"), Hooks{})
	require.Error(t, err)
}

// TestDispatchLegacyActsAsAllInOne verifies that ModeLegacy now routes to
// the all-in-one path (Phase 5). StartLegacy is never called; instead the
// Directory, Recorder, and AutoPair hooks run in order.
func TestDispatchLegacyActsAsAllInOne(t *testing.T) {
	var called []string
	hooks := Hooks{
		StartLegacy:    func() error { called = append(called, "legacy"); return nil },
		StartDirectory: func() error { called = append(called, "directory"); return nil },
		StartRecorder:  func() error { called = append(called, "recorder"); return nil },
		AutoPair:       func() error { called = append(called, "pair"); return nil },
	}
	require.NoError(t, Dispatch(ModeLegacy, hooks))
	require.Equal(t, []string{"directory", "recorder", "pair"}, called)
}

func TestDispatchDirectoryCallsOnlyDirectoryHook(t *testing.T) {
	var called []string
	hooks := Hooks{
		StartLegacy:    func() error { called = append(called, "legacy"); return nil },
		StartDirectory: func() error { called = append(called, "directory"); return nil },
		StartRecorder:  func() error { called = append(called, "recorder"); return nil },
		AutoPair:       func() error { called = append(called, "pair"); return nil },
	}
	require.NoError(t, Dispatch(ModeDirectory, hooks))
	require.Equal(t, []string{"directory"}, called)
}

func TestDispatchRecorderCallsOnlyRecorderHook(t *testing.T) {
	var called []string
	hooks := Hooks{
		StartLegacy:    func() error { called = append(called, "legacy"); return nil },
		StartDirectory: func() error { called = append(called, "directory"); return nil },
		StartRecorder:  func() error { called = append(called, "recorder"); return nil },
		AutoPair:       func() error { called = append(called, "pair"); return nil },
	}
	require.NoError(t, Dispatch(ModeRecorder, hooks))
	require.Equal(t, []string{"recorder"}, called)
}

func TestDispatchAllInOneCallsDirectoryRecorderThenPair(t *testing.T) {
	var called []string
	hooks := Hooks{
		StartLegacy:    func() error { called = append(called, "legacy"); return nil },
		StartDirectory: func() error { called = append(called, "directory"); return nil },
		StartRecorder:  func() error { called = append(called, "recorder"); return nil },
		AutoPair:       func() error { called = append(called, "pair"); return nil },
	}
	require.NoError(t, Dispatch(ModeAllInOne, hooks))
	require.Equal(t, []string{"directory", "recorder", "pair"}, called)
}

func TestDispatchAllInOnePropagatesDirectoryError(t *testing.T) {
	sentinel := errors.New("boom")
	hooks := Hooks{
		StartDirectory: func() error { return sentinel },
		StartRecorder:  func() error { t.Fatal("recorder should not start"); return nil },
		AutoPair:       func() error { t.Fatal("pair should not run"); return nil },
	}
	err := Dispatch(ModeAllInOne, hooks)
	require.ErrorIs(t, err, sentinel)
}

func TestDispatchAllInOnePropagatesRecorderError(t *testing.T) {
	sentinel := errors.New("nope")
	hooks := Hooks{
		StartDirectory: func() error { return nil },
		StartRecorder:  func() error { return sentinel },
		AutoPair:       func() error { t.Fatal("pair should not run"); return nil },
	}
	err := Dispatch(ModeAllInOne, hooks)
	require.ErrorIs(t, err, sentinel)
}

func TestDispatchNilHooksIsNoOp(t *testing.T) {
	require.NoError(t, Dispatch(ModeLegacy, Hooks{}))
	require.NoError(t, Dispatch(ModeDirectory, Hooks{}))
	require.NoError(t, Dispatch(ModeRecorder, Hooks{}))
	require.NoError(t, Dispatch(ModeAllInOne, Hooks{}))
}
