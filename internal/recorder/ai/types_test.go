package ai

import "testing"

func TestObjectStateString(t *testing.T) {
	tests := []struct {
		state ObjectState
		want  string
	}{
		{ObjectEntered, "entered"},
		{ObjectActive, "active"},
		{ObjectLeft, "left"},
		{ObjectState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("ObjectState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
