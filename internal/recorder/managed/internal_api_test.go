package managed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestInternalAPI_HealthEndpoint(t *testing.T) {
	token := "test-service-token-1234"

	api := &InternalAPI{
		ServiceToken:   token,
		RecorderID:     "test-recorder-1",
		RecordingsPath: t.TempDir(),
	}

	if err := api.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer api.Shutdown()

	// Give server a moment to bind.
	time.Sleep(50 * time.Millisecond)

	baseURL := fmt.Sprintf("http://%s", api.Addr())
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("missing token returns 401", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/internal/v1/health")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("got status %d, want 401", resp.StatusCode)
		}
	})

	t.Run("wrong token returns 403", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/internal/v1/health", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("got status %d, want 403", resp.StatusCode)
		}
	})

	t.Run("valid token returns 200 with health data", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/internal/v1/health", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got status %d, want 200", resp.StatusCode)
		}

		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if body["status"] != "ok" {
			t.Errorf("status = %v, want ok", body["status"])
		}
		if body["recorder_id"] != "test-recorder-1" {
			t.Errorf("recorder_id = %v, want test-recorder-1", body["recorder_id"])
		}
	})
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid",
			cfg: Config{
				DirectoryURL: "https://directory.local:9000",
				ServiceToken: "secret-token",
			},
			wantErr: false,
		},
		{
			name: "missing directory URL",
			cfg: Config{
				ServiceToken: "secret-token",
			},
			wantErr: true,
		},
		{
			name: "missing service token",
			cfg: Config{
				DirectoryURL: "https://directory.local:9000",
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			cfg: Config{
				DirectoryURL: "://bad",
				ServiceToken: "secret-token",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
