package synthetic

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ComponentHealth represents the health of a single component as reported by
// the synthetic health endpoint.
type ComponentHealth struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "ok", "degraded", "down"
	Latency string `json:"latency,omitempty"`
}

// HealthResponse is the JSON body returned by the health handler.
type HealthResponse struct {
	Status     string            `json:"status"` // "ok", "degraded", "down"
	Components []ComponentHealth `json:"components"`
	Timestamp  time.Time         `json:"timestamp"`
}

// Prober is a function that checks a single component. It returns the status
// string ("ok", "degraded", "down") and a latency measurement.
type Prober func() (status string, latency time.Duration)

// HealthHandler returns an HTTP handler at /status/health that aggregates
// component health probes. If any component is down, the overall status is
// "down"; if any is degraded, overall is "degraded"; otherwise "ok".
//
// The handler is designed to be the target of synthetic monitoring checks
// from Pingdom, Better Uptime, or similar.
func HealthHandler(probes map[string]Prober) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		type result struct {
			name    string
			status  string
			latency time.Duration
		}

		results := make([]result, 0, len(probes))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for name, probe := range probes {
			wg.Add(1)
			go func(n string, p Prober) {
				defer wg.Done()
				s, l := p()
				mu.Lock()
				results = append(results, result{name: n, status: s, latency: l})
				mu.Unlock()
			}(name, probe)
		}
		wg.Wait()

		overall := "ok"
		components := make([]ComponentHealth, 0, len(results))
		for _, res := range results {
			if res.status == "down" && overall != "down" {
				overall = "down"
			} else if res.status == "degraded" && overall == "ok" {
				overall = "degraded"
			}
			components = append(components, ComponentHealth{
				Name:    res.name,
				Status:  res.status,
				Latency: res.latency.String(),
			})
		}

		resp := HealthResponse{
			Status:     overall,
			Components: components,
			Timestamp:  time.Now().UTC(),
		}

		w.Header().Set("Content-Type", "application/json")
		if overall == "down" {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else if overall == "degraded" {
			w.WriteHeader(http.StatusOK) // still 200 so monitors don't alarm on degraded
		}
		json.NewEncoder(w).Encode(resp)
	})
}

// HTTPProber returns a Prober that makes an HTTP GET to the given URL and
// reports the component status based on the response code.
func HTTPProber(url string, timeout time.Duration) Prober {
	client := &http.Client{Timeout: timeout}
	return func() (string, time.Duration) {
		start := time.Now()
		resp, err := client.Get(url)
		latency := time.Since(start)
		if err != nil {
			return "down", latency
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 {
			return "down", latency
		}
		if resp.StatusCode >= 400 {
			return "degraded", latency
		}
		return "ok", latency
	}
}
