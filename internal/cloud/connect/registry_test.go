package connect

import (
	"testing"
	"time"
)

func TestRegistryAddAndLookup(t *testing.T) {
	r := NewRegistry()

	s := Session{
		SiteID:    "site-1",
		TenantID:  "tenant-1",
		SiteAlias: "warehouse",
		PublicIP:   "1.2.3.4",
		LANCIDRs:  []string{"192.168.1.0/24"},
		Capabilities: map[string]bool{"onvif": true},
		Status:     StatusOnline,
		CameraCount: 5,
	}
	r.Add(s)

	got, ok := r.LookupByAlias("tenant-1", "warehouse")
	if !ok {
		t.Fatal("expected LookupByAlias to return true")
	}
	if got.SiteID != "site-1" {
		t.Errorf("SiteID = %q, want %q", got.SiteID, "site-1")
	}
	if got.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want %q", got.TenantID, "tenant-1")
	}
	if got.SiteAlias != "warehouse" {
		t.Errorf("SiteAlias = %q, want %q", got.SiteAlias, "warehouse")
	}
	if got.PublicIP != "1.2.3.4" {
		t.Errorf("PublicIP = %q, want %q", got.PublicIP, "1.2.3.4")
	}
	if got.CameraCount != 5 {
		t.Errorf("CameraCount = %d, want %d", got.CameraCount, 5)
	}
}

func TestRegistryRemove(t *testing.T) {
	r := NewRegistry()

	r.Add(Session{
		SiteID:    "site-1",
		TenantID:  "tenant-1",
		SiteAlias: "warehouse",
		PublicIP:   "1.2.3.4",
		Status:     StatusOnline,
	})

	r.Remove("site-1")

	_, ok := r.LookupByAlias("tenant-1", "warehouse")
	if ok {
		t.Fatal("expected LookupByAlias to return false after Remove")
	}
}

func TestRegistryUpdateHeartbeat(t *testing.T) {
	r := NewRegistry()

	r.Add(Session{
		SiteID:      "site-1",
		TenantID:    "tenant-1",
		SiteAlias:   "warehouse",
		PublicIP:     "1.2.3.4",
		Status:       StatusOnline,
		CameraCount: 2,
	})

	beforeUpdate := time.Now()

	r.UpdateHeartbeat("site-1", HeartbeatUpdate{
		CameraCount:   10,
		RecorderCount: 3,
		DiskUsedPct:   42.5,
		PublicIP:       "5.6.7.8",
	})

	got, ok := r.LookupByAlias("tenant-1", "warehouse")
	if !ok {
		t.Fatal("expected session to still exist after heartbeat")
	}
	if got.CameraCount != 10 {
		t.Errorf("CameraCount = %d, want 10", got.CameraCount)
	}
	if got.RecorderCount != 3 {
		t.Errorf("RecorderCount = %d, want 3", got.RecorderCount)
	}
	if got.DiskUsedPct != 42.5 {
		t.Errorf("DiskUsedPct = %f, want 42.5", got.DiskUsedPct)
	}
	if got.PublicIP != "5.6.7.8" {
		t.Errorf("PublicIP = %q, want %q", got.PublicIP, "5.6.7.8")
	}
	if got.LastSeen.Before(beforeUpdate) {
		t.Error("expected LastSeen to be updated")
	}

	// Verify empty PublicIP does not overwrite.
	r.UpdateHeartbeat("site-1", HeartbeatUpdate{
		CameraCount: 11,
		PublicIP:     "",
	})

	got2, _ := r.LookupByAlias("tenant-1", "warehouse")
	if got2.PublicIP != "5.6.7.8" {
		t.Errorf("PublicIP should remain %q when heartbeat PublicIP is empty, got %q", "5.6.7.8", got2.PublicIP)
	}
	if got2.CameraCount != 11 {
		t.Errorf("CameraCount = %d, want 11", got2.CameraCount)
	}
}

func TestRegistryListByTenant(t *testing.T) {
	r := NewRegistry()

	r.Add(Session{SiteID: "s1", TenantID: "t1", SiteAlias: "a1", Status: StatusOnline})
	r.Add(Session{SiteID: "s2", TenantID: "t1", SiteAlias: "a2", Status: StatusOnline})
	r.Add(Session{SiteID: "s3", TenantID: "t2", SiteAlias: "a3", Status: StatusOnline})

	list := r.ListByTenant("t1")
	if len(list) != 2 {
		t.Fatalf("ListByTenant(t1) returned %d sessions, want 2", len(list))
	}

	list2 := r.ListByTenant("t2")
	if len(list2) != 1 {
		t.Fatalf("ListByTenant(t2) returned %d sessions, want 1", len(list2))
	}

	list3 := r.ListByTenant("t999")
	if len(list3) != 0 {
		t.Fatalf("ListByTenant(t999) returned %d sessions, want 0", len(list3))
	}
}
