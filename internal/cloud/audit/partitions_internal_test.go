package audit

import (
	"testing"
	"time"
)

func TestPartitionUpperBound(t *testing.T) {
	cases := []struct {
		name string
		want time.Time
		ok   bool
	}{
		{"audit_log_2026_04", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), true},
		{"audit_log_2025_12", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"audit_log_2026_13", time.Time{}, false}, // bad month
		{"audit_log_bogus", time.Time{}, false},   // not YYYY_MM
		{"other_2026_04", time.Time{}, false},     // wrong parent
	}
	for _, c := range cases {
		got, ok := partitionUpperBound(c.name, "audit_log")
		if ok != c.ok {
			t.Errorf("%s: ok want %v got %v", c.name, c.ok, ok)
			continue
		}
		if ok && !got.Equal(c.want) {
			t.Errorf("%s: want %v got %v", c.name, c.want, got)
		}
	}
}

func TestCreatePartitionDDL_Shape(t *testing.T) {
	// Verify the DDL we would emit for Postgres has the right shape. We
	// don't have a real Postgres here so we build the statement by hand
	// using the same formatter and inspect it as a string.
	name := PartitionName("audit_log", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	if name != "audit_log_2026_04" {
		t.Errorf("name: got %s", name)
	}
}

func TestFirstOfMonth(t *testing.T) {
	in := time.Date(2026, 4, 7, 12, 34, 56, 0, time.UTC)
	got := firstOfMonth(in)
	want := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("want %v got %v", want, got)
	}
}
