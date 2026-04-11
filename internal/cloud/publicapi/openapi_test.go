package publicapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAPISpecIsValidJSON(t *testing.T) {
	spec := OpenAPISpec()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(spec), &parsed); err != nil {
		t.Fatalf("OpenAPI spec is not valid JSON: %v\nspec prefix: %s", err, spec[:min(500, len(spec))])
	}
}

func TestOpenAPISpecContainsAllResources(t *testing.T) {
	spec := OpenAPISpec()

	resources := []string{
		"cameras", "users", "recordings", "events",
		"schedules", "retention-policies", "integrations",
	}
	for _, r := range resources {
		if !strings.Contains(spec, "/"+r) {
			t.Errorf("OpenAPI spec missing resource: %s", r)
		}
	}
}

func TestOpenAPISpecContainsSecuritySchemes(t *testing.T) {
	spec := OpenAPISpec()
	if !strings.Contains(spec, "ApiKeyAuth") {
		t.Error("OpenAPI spec missing ApiKeyAuth security scheme")
	}
	if !strings.Contains(spec, "BearerAuth") {
		t.Error("OpenAPI spec missing BearerAuth security scheme")
	}
}

func TestOpenAPISpecContainsSchemas(t *testing.T) {
	spec := OpenAPISpec()
	schemas := []string{
		"Camera", "User", "Recording", "Event",
		"Schedule", "RetentionPolicy", "Integration",
		"Error", "Pagination",
	}
	for _, s := range schemas {
		if !strings.Contains(spec, `"`+s+`"`) {
			t.Errorf("OpenAPI spec missing schema: %s", s)
		}
	}
}

func TestOpenAPISpecHasVersionInfo(t *testing.T) {
	spec := OpenAPISpec()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(spec), &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["openapi"] != "3.0.3" {
		t.Errorf("openapi = %v; want 3.0.3", parsed["openapi"])
	}

	info := parsed["info"].(map[string]interface{})
	if info["version"] != "1.0.0" {
		t.Errorf("info.version = %v; want 1.0.0", info["version"])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
