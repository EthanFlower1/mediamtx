package plans

import (
	"errors"
	"sort"
	"testing"
)

// TestCatalog_Wellformed is the belt-and-braces assertion that the package
// init validation succeeded and produced a complete catalog. If the init
// panic ever fires, this test will never run — so the fact that it runs at
// all is part of the contract.
func TestCatalog_Wellformed(t *testing.T) {
	plans := AllPlans()
	if got, want := len(plans), 4; got != want {
		t.Fatalf("AllPlans length = %d, want %d", got, want)
	}

	// Canonical tier order.
	wantOrder := []Tier{TierFree, TierStarter, TierPro, TierEnterprise}
	for i, p := range plans {
		if p.Tier != wantOrder[i] {
			t.Errorf("AllPlans[%d].Tier = %q, want %q", i, p.Tier, wantOrder[i])
		}
	}

	// Retail monotonic non-decreasing.
	var prev int64 = -1
	for _, p := range plans {
		if p.RetailPricePerCameraCents < prev {
			t.Errorf("retail price not monotonic: tier %q has %d (prev %d)",
				p.Tier, p.RetailPricePerCameraCents, prev)
		}
		prev = p.RetailPricePerCameraCents
	}

	// Wholesale <= retail for every tier.
	for _, p := range plans {
		if p.WholesalePricePerCameraCents > p.RetailPricePerCameraCents {
			t.Errorf("tier %q wholesale %d > retail %d",
				p.Tier, p.WholesalePricePerCameraCents, p.RetailPricePerCameraCents)
		}
	}
}

func TestLookupPlan(t *testing.T) {
	tests := []struct {
		name    string
		tier    Tier
		wantErr error
	}{
		{"free", TierFree, nil},
		{"starter", TierStarter, nil},
		{"professional", TierPro, nil},
		{"enterprise", TierEnterprise, nil},
		{"unknown", Tier("gold"), ErrUnknownTier},
		{"empty string", Tier(""), ErrUnknownTier},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := LookupPlan(tt.tier)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("LookupPlan(%q) err = %v, want nil", tt.tier, err)
				}
				if p.Tier != tt.tier {
					t.Errorf("LookupPlan(%q).Tier = %q, want %q", tt.tier, p.Tier, tt.tier)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("LookupPlan(%q) err = %v, want wrapping %v", tt.tier, err, tt.wantErr)
			}
		})
	}
}

func TestLookupAddOn(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{"face recognition", AddOnFaceRecognition, nil},
		{"lpr", AddOnLPR, nil},
		{"custom ai model", AddOnCustomAIModelUpload, nil},
		{"cloud archive extended", AddOnCloudArchiveExtended, nil},
		{"unknown id", "mystery_feature", ErrUnknownAddOn},
		{"empty id", "", ErrUnknownAddOn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := LookupAddOn(tt.id)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("LookupAddOn(%q) err = %v, want nil", tt.id, err)
				}
				if a.ID != tt.id {
					t.Errorf("LookupAddOn(%q).ID = %q, want %q", tt.id, a.ID, tt.id)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("LookupAddOn(%q) err = %v, want wrapping %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

// TestAllAddOns_StableOrder locks in the determinism guarantee documented on
// the package: AllAddOns returns entries sorted by ID so downstream snapshot
// tests and billing exports see reproducible output.
func TestAllAddOns_StableOrder(t *testing.T) {
	addons := AllAddOns()
	if len(addons) == 0 {
		t.Fatal("AllAddOns returned empty slice")
	}
	ids := make([]string, len(addons))
	for i, a := range addons {
		ids[i] = a.ID
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	for i := range ids {
		if ids[i] != sorted[i] {
			t.Errorf("AllAddOns not sorted: index %d id = %q, want %q", i, ids[i], sorted[i])
		}
	}

	// Belt-and-braces: AllAddOns is a fresh allocation; mutating it must not
	// affect a subsequent call.
	addons[0].ID = "MUTATED"
	again := AllAddOns()
	if again[0].ID == "MUTATED" {
		t.Error("AllAddOns returned a slice that aliases internal state")
	}
}

func TestTierRank(t *testing.T) {
	tests := []struct {
		tier Tier
		want int
	}{
		{TierFree, 0},
		{TierStarter, 1},
		{TierPro, 2},
		{TierEnterprise, 3},
		{Tier("unknown"), -1},
	}
	for _, tt := range tests {
		if got := tt.tier.Rank(); got != tt.want {
			t.Errorf("%q.Rank() = %d, want %d", tt.tier, got, tt.want)
		}
	}
}
