package whitelabel

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testTenantID = "11111111-2222-3333-8444-555555555555"

func validConfig() BrandConfig {
	return BrandConfig{
		TenantID: testTenantID,
		Version:  1,
		AppName:  "Acme Security",
		Colors: ColorPalette{
			Primary:   "#112233",
			Secondary: "#445566",
			Accent:    "#778899",
		},
		Typography: Typography{
			HeadingFamily: "Inter",
			BodyFamily:    "Inter",
		},
		BundleIDs: BundleIDs{
			IOS:     "com.acme.security",
			Android: "com.acme.security",
		},
		SenderDomain: "mail.acme.example",
		ToSURL:       "https://acme.example/tos",
		PrivacyURL:   "https://acme.example/privacy",
	}
}

func TestBrandConfig_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*BrandConfig)
		wantErr bool
	}{
		{"baseline valid", func(*BrandConfig) {}, false},
		{"missing tenant", func(c *BrandConfig) { c.TenantID = "" }, true},
		{"bad uuid", func(c *BrandConfig) { c.TenantID = "not-a-uuid" }, true},
		{"zero version", func(c *BrandConfig) { c.Version = 0 }, true},
		{"empty app name", func(c *BrandConfig) { c.AppName = "" }, true},
		{"oversize app name", func(c *BrandConfig) { c.AppName = strings.Repeat("x", 65) }, true},
		{"bad primary color", func(c *BrandConfig) { c.Colors.Primary = "red" }, true},
		{"short hex allowed", func(c *BrandConfig) { c.Colors.Primary = "#abc" }, false},
		{"bad background allowed empty", func(c *BrandConfig) { c.Colors.Background = "" }, false},
		{"bad background non-empty", func(c *BrandConfig) { c.Colors.Background = "blue" }, true},
		{"missing heading font", func(c *BrandConfig) { c.Typography.HeadingFamily = "" }, true},
		{"bad ios bundle id", func(c *BrandConfig) { c.BundleIDs.IOS = "acme" }, true},
		{"bad android bundle id", func(c *BrandConfig) { c.BundleIDs.Android = "-acme.app" }, true},
		{"bad sender domain", func(c *BrandConfig) { c.SenderDomain = "not a domain" }, true},
		{"http tos url rejected", func(c *BrandConfig) { c.ToSURL = "http://acme.example/tos" }, true},
		{"invalid privacy url", func(c *BrandConfig) { c.PrivacyURL = "::bad" }, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// pngBytes generates an in-memory PNG of the requested size.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestValidateAsset(t *testing.T) {
	t.Parallel()

	validLogo := pngBytes(t, 256, 256)
	validSplash := pngBytes(t, 1024, 1024)
	validIcon := pngBytes(t, 1024, 1024)
	smallLogo := pngBytes(t, 16, 16)
	nonSquareIcon := pngBytes(t, 1024, 512)
	wrongSizeIcon := pngBytes(t, 512, 512)

	cases := []struct {
		name     string
		kind     AssetKind
		filename string
		data     []byte
		wantErr  bool
	}{
		{"logo ok", AssetKindLogo, "logo.png", validLogo, false},
		{"logo too small", AssetKindLogo, "logo.png", smallLogo, true},
		{"splash ok", AssetKindSplash, "splash.png", validSplash, false},
		{"icon ok", AssetKindIcon, "icon.png", validIcon, false},
		{"icon wrong size", AssetKindIcon, "icon.png", wrongSizeIcon, true},
		{"icon non-square", AssetKindIcon, "icon.png", nonSquareIcon, true},
		{"empty body", AssetKindLogo, "logo.png", nil, true},
		{"invalid kind", AssetKind("banner"), "banner.png", validLogo, true},
		{"font wrong ext", AssetKindFont, "font.bin", []byte("just some font bytes"), true},
		{"font ok ttf", AssetKindFont, "Inter.ttf", bytes.Repeat([]byte{0x00, 0x01, 0x02, 0x03}, 64), false},
		{"logo non-image bytes", AssetKindLogo, "logo.png", []byte("<html>hi</html>"), true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateAsset(tc.kind, tc.filename, tc.data)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateAsset err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestMemoryAssetStore_RoundTrip(t *testing.T) {
	t.Parallel()
	store := NewMemoryAssetStore()
	ctx := context.Background()

	logo := pngBytes(t, 256, 256)
	ref1, err := store.Put(ctx, testTenantID, AssetKindLogo, "logo.png", bytes.NewReader(logo))
	if err != nil {
		t.Fatalf("put v1: %v", err)
	}
	if ref1.Version != 1 {
		t.Errorf("v1 version=%d want 1", ref1.Version)
	}
	ref2, err := store.Put(ctx, testTenantID, AssetKindLogo, "logo.png", bytes.NewReader(logo))
	if err != nil {
		t.Fatalf("put v2: %v", err)
	}
	if ref2.Version != 2 {
		t.Errorf("v2 version=%d want 2", ref2.Version)
	}

	got, rc, err := store.Get(ctx, testTenantID, AssetKindLogo, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if !bytes.Equal(body, logo) {
		t.Errorf("round-trip bytes mismatch")
	}
	if got.StorageKey == "" {
		t.Errorf("expected non-empty storage key")
	}

	list, err := store.List(ctx, testTenantID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("list len=%d want 2", len(list))
	}

	if err := store.Delete(ctx, testTenantID, AssetKindLogo, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := store.Get(ctx, testTenantID, AssetKindLogo, 1); err == nil {
		t.Errorf("expected not found after delete")
	}
}

func TestMemoryConfigStore_Versioning(t *testing.T) {
	t.Parallel()
	store := NewMemoryConfigStore()
	ctx := context.Background()

	cfg := validConfig()
	saved, err := store.Save(ctx, &cfg)
	if err != nil {
		t.Fatalf("save v1: %v", err)
	}
	if saved.Version != 1 {
		t.Errorf("save v1 version=%d", saved.Version)
	}

	cfg2 := validConfig()
	cfg2.AppName = "Acme Pro"
	saved2, err := store.Save(ctx, &cfg2)
	if err != nil {
		t.Fatalf("save v2: %v", err)
	}
	if saved2.Version != 2 {
		t.Errorf("save v2 version=%d", saved2.Version)
	}

	current, err := store.Current(ctx, testTenantID)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if current.AppName != "Acme Pro" || current.Version != 2 {
		t.Errorf("current=%+v", current)
	}

	v1, err := store.GetVersion(ctx, testTenantID, 1)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if v1.AppName != "Acme Security" {
		t.Errorf("v1 app name=%q", v1.AppName)
	}

	list, err := store.List(ctx, testTenantID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("list len=%d", len(list))
	}
}

func TestAPI_RoundTrip(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewMemoryConfigStore(), NewMemoryAssetStore())
	srv := httptest.NewServer(api)
	defer srv.Close()

	cfg := validConfig()
	body, _ := json.Marshal(cfg)

	// PUT /brand
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/integrators/"+testTenantID+"/brand", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("put status=%d body=%s", resp.StatusCode, string(b))
	}
	var got BrandConfig
	_ = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Version != 1 || got.TenantID != testTenantID {
		t.Errorf("put returned %+v", got)
	}

	// Second PUT should bump version.
	req2, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/integrators/"+testTenantID+"/brand", bytes.NewReader(body))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("put2: %v", err)
	}
	var got2 BrandConfig
	_ = json.NewDecoder(resp2.Body).Decode(&got2)
	resp2.Body.Close()
	if got2.Version != 2 {
		t.Errorf("put2 version=%d", got2.Version)
	}

	// GET /brand returns latest
	resp3, err := http.Get(srv.URL + "/api/v1/integrators/" + testTenantID + "/brand")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var got3 BrandConfig
	_ = json.NewDecoder(resp3.Body).Decode(&got3)
	resp3.Body.Close()
	if got3.Version != 2 {
		t.Errorf("get current version=%d", got3.Version)
	}

	// GET versions list
	resp4, err := http.Get(srv.URL + "/api/v1/integrators/" + testTenantID + "/brand/versions")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var versions []BrandConfig
	_ = json.NewDecoder(resp4.Body).Decode(&versions)
	resp4.Body.Close()
	if len(versions) != 2 {
		t.Errorf("versions len=%d", len(versions))
	}

	// GET specific version
	resp5, err := http.Get(srv.URL + "/api/v1/integrators/" + testTenantID + "/brand/versions/1")
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if resp5.StatusCode != http.StatusOK {
		t.Errorf("get v1 status=%d", resp5.StatusCode)
	}
	resp5.Body.Close()

	// POST asset
	logo := pngBytes(t, 256, 256)
	areq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/api/v1/integrators/"+testTenantID+"/brand/assets/logo",
		bytes.NewReader(logo))
	areq.Header.Set("X-Asset-Filename", "logo.png")
	aresp, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatalf("post asset: %v", err)
	}
	if aresp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(aresp.Body)
		t.Fatalf("post asset status=%d body=%s", aresp.StatusCode, string(b))
	}
	var ref AssetRef
	_ = json.NewDecoder(aresp.Body).Decode(&ref)
	aresp.Body.Close()
	if ref.Version != 1 || ref.Kind != AssetKindLogo {
		t.Errorf("ref=%+v", ref)
	}

	// Invalid kind
	badReq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/api/v1/integrators/"+testTenantID+"/brand/assets/banner",
		bytes.NewReader(logo))
	badResp, _ := http.DefaultClient.Do(badReq)
	if badResp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad kind status=%d", badResp.StatusCode)
	}
	badResp.Body.Close()

	// PUT with tenant mismatch
	mm := validConfig()
	mm.TenantID = "00000000-0000-1000-8000-000000000000"
	mmBody, _ := json.Marshal(mm)
	mmReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/integrators/"+testTenantID+"/brand", bytes.NewReader(mmBody))
	mmResp, _ := http.DefaultClient.Do(mmReq)
	if mmResp.StatusCode != http.StatusBadRequest {
		t.Errorf("mismatch status=%d", mmResp.StatusCode)
	}
	mmResp.Body.Close()
}

func TestRoutes_Coverage(t *testing.T) {
	t.Parallel()
	api := NewAPI(nil, nil)
	routes := api.Routes()
	if len(routes) != 5 {
		t.Fatalf("routes len=%d want 5", len(routes))
	}
	want := map[string]bool{
		"GET /api/v1/integrators/{id}/brand":                            false,
		"PUT /api/v1/integrators/{id}/brand":                            false,
		"POST /api/v1/integrators/{id}/brand/assets/{kind}":             false,
		"GET /api/v1/integrators/{id}/brand/versions":                   false,
		"GET /api/v1/integrators/{id}/brand/versions/{version}":         false,
	}
	for _, r := range routes {
		key := r.Method + " " + r.Pattern
		if _, ok := want[key]; !ok {
			t.Errorf("unexpected route %s", key)
		}
		want[key] = true
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("missing route %s", k)
		}
	}
}
