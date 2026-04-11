package publicapi

import (
	"testing"
)

func TestPublicServicePathFormat(t *testing.T) {
	got := PublicServicePath("PublicCamerasService", "ListCameras")
	want := "/kaivue.v1.public.PublicCamerasService/ListCameras"
	if got != want {
		t.Errorf("PublicServicePath = %q; want %q", got, want)
	}
}

func TestPublicRESTPathFormat(t *testing.T) {
	got := PublicRESTPath("cameras")
	want := "/api/v1/cameras"
	if got != want {
		t.Errorf("PublicRESTPath = %q; want %q", got, want)
	}
}

func TestAllPublicPathsNonEmpty(t *testing.T) {
	paths := AllPublicPaths()
	if len(paths) == 0 {
		t.Fatal("AllPublicPaths() returned empty")
	}
	// Every path should start with /kaivue.v1.public.
	for _, p := range paths {
		if p[:len("/kaivue.v1.public.")] != "/kaivue.v1.public." {
			t.Errorf("path %q does not start with /kaivue.v1.public.", p)
		}
	}
}

func TestPublicRouteAuthorizationsComplete(t *testing.T) {
	routes := PublicRouteAuthorizations()
	paths := AllPublicPaths()

	for _, p := range paths {
		if _, ok := routes[p]; !ok {
			t.Errorf("path %s has no route authorization", p)
		}
	}
}

func TestRouteAuthorizationsHaveValidResources(t *testing.T) {
	validResources := map[string]bool{
		"cameras":      true,
		"users":        true,
		"recordings":   true,
		"events":       true,
		"schedules":    true,
		"retention":    true,
		"integrations": true,
	}
	validActions := map[string]bool{
		"create": true,
		"read":   true,
		"update": true,
		"delete": true,
		"test":   true,
	}

	routes := PublicRouteAuthorizations()
	for path, auth := range routes {
		if !validResources[auth.Resource] {
			t.Errorf("path %s has invalid resource %q", path, auth.Resource)
		}
		if !validActions[auth.Action] {
			t.Errorf("path %s has invalid action %q", path, auth.Action)
		}
	}
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{`say "hi"`, `say \"hi\"`},
		{"line\nbreak", `line\nbreak`},
		{`back\slash`, `back\\slash`},
	}
	for _, tt := range tests {
		got := escapeJSON(tt.in)
		if got != tt.want {
			t.Errorf("escapeJSON(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"cameras", "camera"},
		{"users", "user"},
		{"policies", "policy"},
		{"addresses", "address"},
		{"event", "event"},
	}
	for _, tt := range tests {
		got := singularize(tt.in)
		if got != tt.want {
			t.Errorf("singularize(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}
