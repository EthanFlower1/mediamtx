package plans

import (
	"errors"
	"testing"
)

func TestResolve_EachTierAlone(t *testing.T) {
	tests := []struct {
		tier                    Tier
		wantMaxCameras          int
		wantRetentionDays       int
		wantHasBasicDetection   bool
		wantHasFaceRecognition  bool
		wantHasFedRAMP          bool
		wantActiveAddOnsCount   int
	}{
		{TierFree, 4, 7, true, false, false, 0},
		{TierStarter, 32, 30, true, false, false, 0},
		{TierPro, 256, 90, true, true, false, 0},
		{TierEnterprise, Unlimited, 365, true, true, true, 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			e, err := Resolve(tt.tier, nil)
			if err != nil {
				t.Fatalf("Resolve(%q, nil) err = %v", tt.tier, err)
			}
			if got := e.MaxCameras(); got != tt.wantMaxCameras {
				t.Errorf("MaxCameras = %d, want %d", got, tt.wantMaxCameras)
			}
			if got := e.RetentionDays(); got != tt.wantRetentionDays {
				t.Errorf("RetentionDays = %d, want %d", got, tt.wantRetentionDays)
			}
			if got := e.HasFeature(FeatureBasicDetection); got != tt.wantHasBasicDetection {
				t.Errorf("HasFeature(basic_detection) = %v, want %v", got, tt.wantHasBasicDetection)
			}
			if got := e.HasFeature(FeatureFaceRecognition); got != tt.wantHasFaceRecognition {
				t.Errorf("HasFeature(face_recognition) = %v, want %v", got, tt.wantHasFaceRecognition)
			}
			if got := e.HasFeature(FeatureFedRAMP); got != tt.wantHasFedRAMP {
				t.Errorf("HasFeature(fedramp) = %v, want %v", got, tt.wantHasFedRAMP)
			}
			if got := len(e.ActiveAddOns); got != tt.wantActiveAddOnsCount {
				t.Errorf("len(ActiveAddOns) = %d, want %d", got, tt.wantActiveAddOnsCount)
			}
		})
	}
}

func TestResolve_AddOnGrantsFeature(t *testing.T) {
	// Face recognition is NOT included in Starter by default.
	e, err := Resolve(TierStarter, nil)
	if err != nil {
		t.Fatalf("Resolve(starter, nil) err = %v", err)
	}
	if e.HasFeature(FeatureFaceRecognition) {
		t.Fatal("Starter tier unexpectedly includes face recognition by default")
	}

	// Activating the face_recognition add-on grants the feature.
	e2, err := Resolve(TierStarter, []string{AddOnFaceRecognition})
	if err != nil {
		t.Fatalf("Resolve(starter, [face_recognition]) err = %v", err)
	}
	if !e2.HasFeature(FeatureFaceRecognition) {
		t.Error("Starter + face_recognition add-on should unlock face recognition")
	}
	if !e2.HasAddOn(AddOnFaceRecognition) {
		t.Error("HasAddOn(face_recognition) = false, want true")
	}
}

func TestResolve_ExtendsRetention(t *testing.T) {
	// Starter default is 30 days; cloud_archive_extended adds 365.
	e, err := Resolve(TierStarter, []string{AddOnCloudArchiveExtended})
	if err != nil {
		t.Fatalf("Resolve err = %v", err)
	}
	wantDays := 30 + 365
	if got := e.RetentionDays(); got != wantDays {
		t.Errorf("RetentionDays = %d, want %d", got, wantDays)
	}
	if !e.HasFeature(FeatureCloudArchiveExtended) {
		t.Error("cloud_archive_extended add-on should grant the feature flag")
	}
}

func TestResolve_RejectsUnknownAddOn(t *testing.T) {
	_, err := Resolve(TierStarter, []string{"mystery_addon"})
	if err == nil {
		t.Fatal("Resolve with unknown add-on should error")
	}
	if !errors.Is(err, ErrUnknownAddOn) {
		t.Errorf("err = %v, want wrapping ErrUnknownAddOn", err)
	}
}

func TestResolve_RejectsUnknownTier(t *testing.T) {
	_, err := Resolve(Tier("gold_deluxe"), nil)
	if err == nil {
		t.Fatal("Resolve with unknown tier should error")
	}
	if !errors.Is(err, ErrUnknownTier) {
		t.Errorf("err = %v, want wrapping ErrUnknownTier", err)
	}
}

func TestResolve_AddOnRequiresHigherTier(t *testing.T) {
	// custom_ai_model_upload has MinTier=TierPro; Starter must be rejected.
	_, err := Resolve(TierStarter, []string{AddOnCustomAIModelUpload})
	if err == nil {
		t.Fatal("Starter + custom_ai_model_upload should error")
	}
	if !errors.Is(err, ErrAddOnRequiresHigherTier) {
		t.Errorf("err = %v, want wrapping ErrAddOnRequiresHigherTier", err)
	}

	// And same add-on on Pro succeeds.
	e, err := Resolve(TierPro, []string{AddOnCustomAIModelUpload})
	if err != nil {
		t.Fatalf("Resolve(pro, [custom_ai_model_upload]) err = %v", err)
	}
	if !e.HasFeature(FeatureCustomAIModelUpload) {
		t.Error("Pro + custom_ai_model_upload should unlock the feature")
	}
}

func TestResolve_FreeTierRejectsPaidAddOns(t *testing.T) {
	// Every paid add-on has MinTier >= Starter, so Free must reject all of
	// them. Walk the whole catalog so a newly added add-on that forgets to
	// set MinTier gets caught by this test.
	for _, a := range AllAddOns() {
		_, err := Resolve(TierFree, []string{a.ID})
		if err == nil {
			t.Errorf("Free + %q should error (MinTier = %q)", a.ID, a.MinTier)
			continue
		}
		if !errors.Is(err, ErrAddOnRequiresHigherTier) {
			t.Errorf("Free + %q err = %v, want wrapping ErrAddOnRequiresHigherTier",
				a.ID, err)
		}
	}
}

func TestResolve_DuplicateAddOnsCoalesced(t *testing.T) {
	// Passing the same add-on ID twice must be a no-op, not an error.
	e, err := Resolve(TierPro, []string{AddOnFaceRecognition, AddOnFaceRecognition})
	if err != nil {
		t.Fatalf("Resolve with duplicate ids err = %v", err)
	}
	if got := len(e.ActiveAddOns); got != 1 {
		t.Errorf("len(ActiveAddOns) = %d, want 1 after dedup", got)
	}
}

func TestResolve_ActiveAddOnsIDSorted(t *testing.T) {
	e, err := Resolve(TierPro, []string{
		AddOnLPR,
		AddOnFaceRecognition,
		AddOnBehavioralAnalytics,
	})
	if err != nil {
		t.Fatalf("Resolve err = %v", err)
	}
	ids := make([]string, len(e.ActiveAddOns))
	for i, a := range e.ActiveAddOns {
		ids[i] = a.ID
	}
	want := []string{AddOnBehavioralAnalytics, AddOnFaceRecognition, AddOnLPR}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ActiveAddOns[%d].ID = %q, want %q", i, ids[i], want[i])
		}
	}
}

func TestEntitlements_NilSafe(t *testing.T) {
	var e *Entitlements
	if e.HasFeature(FeatureBasicDetection) {
		t.Error("nil.HasFeature should return false")
	}
	if got := e.MaxCameras(); got != 0 {
		t.Errorf("nil.MaxCameras = %d, want 0", got)
	}
	if got := e.RetentionDays(); got != 0 {
		t.Errorf("nil.RetentionDays = %d, want 0", got)
	}
	if e.HasAddOn(AddOnFaceRecognition) {
		t.Error("nil.HasAddOn should return false")
	}
	if got := e.Features(); got != nil {
		t.Errorf("nil.Features = %v, want nil", got)
	}
}

func TestEntitlements_FeaturesDeterministic(t *testing.T) {
	e, err := Resolve(TierPro, []string{AddOnCustomAIModelUpload})
	if err != nil {
		t.Fatalf("Resolve err = %v", err)
	}
	f1 := e.Features()
	f2 := e.Features()
	if len(f1) != len(f2) {
		t.Fatalf("Features length mismatch: %d vs %d", len(f1), len(f2))
	}
	for i := range f1 {
		if f1[i] != f2[i] {
			t.Errorf("Features[%d] differs: %q vs %q", i, f1[i], f2[i])
		}
	}
	// And calling Features twice and mutating the first result must not
	// affect the second.
	f1[0] = "MUTATED"
	f3 := e.Features()
	if f3[0] == "MUTATED" {
		t.Error("Features returned slice aliasing internal state")
	}
}
