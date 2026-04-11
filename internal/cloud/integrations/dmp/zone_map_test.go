package dmp

import (
	"testing"
)

func TestZoneMapper_AddAndLookup(t *testing.T) {
	zm := NewZoneMapper()

	// Add a mapping.
	zm.Add(&ZoneMapping{
		AccountID: "1234",
		Zone:      1,
		Area:      1,
		CameraID:  "cam-front",
		Label:     "Front door",
	})

	zm.Add(&ZoneMapping{
		AccountID: "1234",
		Zone:      2,
		Area:      1,
		CameraID:  "cam-back",
		Label:     "Back door",
	})

	// Exact match.
	camID, ok := zm.Lookup("1234", 1, 1)
	if !ok || camID != "cam-front" {
		t.Errorf("Lookup(1234, 1, 1) = %q, %v; want cam-front, true", camID, ok)
	}

	camID, ok = zm.Lookup("1234", 2, 1)
	if !ok || camID != "cam-back" {
		t.Errorf("Lookup(1234, 2, 1) = %q, %v; want cam-back, true", camID, ok)
	}

	// No match.
	_, ok = zm.Lookup("9999", 1, 1)
	if ok {
		t.Error("Lookup(9999, 1, 1) should not match")
	}
}

func TestZoneMapper_FallbackLookup(t *testing.T) {
	zm := NewZoneMapper()

	// Add a catch-all mapping for the account.
	zm.Add(&ZoneMapping{
		AccountID: "5678",
		Zone:      0,
		Area:      0,
		CameraID:  "cam-overview",
	})

	// Add a zone-specific mapping without area.
	zm.Add(&ZoneMapping{
		AccountID: "5678",
		Zone:      3,
		Area:      0,
		CameraID:  "cam-garage",
	})

	// Zone 3, any area should match the zone-specific mapping.
	camID, ok := zm.Lookup("5678", 3, 2)
	if !ok || camID != "cam-garage" {
		t.Errorf("Lookup(5678, 3, 2) = %q, %v; want cam-garage, true", camID, ok)
	}

	// Unknown zone should fall back to catch-all.
	camID, ok = zm.Lookup("5678", 99, 1)
	if !ok || camID != "cam-overview" {
		t.Errorf("Lookup(5678, 99, 1) = %q, %v; want cam-overview, true", camID, ok)
	}
}

func TestZoneMapper_Remove(t *testing.T) {
	zm := NewZoneMapper()

	zm.Add(&ZoneMapping{
		AccountID: "1234",
		Zone:      1,
		Area:      1,
		CameraID:  "cam-1",
	})

	_, ok := zm.Lookup("1234", 1, 1)
	if !ok {
		t.Fatal("expected mapping to exist before removal")
	}

	zm.Remove("1234", 1, 1)

	_, ok = zm.Lookup("1234", 1, 1)
	if ok {
		t.Error("expected mapping to be removed")
	}
}

func TestZoneMapper_LoadMappings(t *testing.T) {
	zm := NewZoneMapper()

	zm.Add(&ZoneMapping{AccountID: "old", Zone: 1, Area: 1, CameraID: "old-cam"})

	zm.LoadMappings([]*ZoneMapping{
		{AccountID: "new", Zone: 1, Area: 1, CameraID: "new-cam"},
		{AccountID: "new", Zone: 2, Area: 1, CameraID: "new-cam-2"},
	})

	if zm.Count() != 2 {
		t.Errorf("Count() = %d, want 2", zm.Count())
	}

	// Old mapping should be gone.
	_, ok := zm.Lookup("old", 1, 1)
	if ok {
		t.Error("old mapping should be replaced")
	}

	// New mapping should exist.
	camID, ok := zm.Lookup("new", 1, 1)
	if !ok || camID != "new-cam" {
		t.Errorf("Lookup(new, 1, 1) = %q, %v; want new-cam, true", camID, ok)
	}
}

func TestZoneMapper_List(t *testing.T) {
	zm := NewZoneMapper()

	zm.Add(&ZoneMapping{AccountID: "1234", Zone: 1, Area: 1, CameraID: "cam-1"})
	zm.Add(&ZoneMapping{AccountID: "1234", Zone: 2, Area: 1, CameraID: "cam-2"})

	list := zm.List()
	if len(list) != 2 {
		t.Errorf("List() returned %d items, want 2", len(list))
	}
}

func TestZoneMapper_ConcurrentAccess(t *testing.T) {
	zm := NewZoneMapper()

	// Run concurrent reads and writes to verify thread safety.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			zm.Add(&ZoneMapping{
				AccountID: "1234",
				Zone:      i,
				Area:      1,
				CameraID:  "cam",
			})
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		zm.Lookup("1234", i, 1)
	}

	<-done

	if zm.Count() != 100 {
		t.Errorf("Count() = %d, want 100", zm.Count())
	}
}
