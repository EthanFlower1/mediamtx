package synthetic

import "time"

// CheckType is the kind of synthetic check.
type CheckType string

const (
	CheckHTTP CheckType = "http"
	CheckTCP  CheckType = "tcp"
	CheckDNS  CheckType = "dns"
)

// Check defines a single synthetic monitoring check. These are declarative
// definitions that can be provisioned to Pingdom, Better Uptime, or any
// compatible provider.
type Check struct {
	// Name is a human-readable label shown in dashboards.
	Name string `json:"name"`

	// Type is the check kind (http, tcp, dns).
	Type CheckType `json:"type"`

	// URL is the target endpoint (e.g. "https://api.kaivue.io/healthz").
	URL string `json:"url"`

	// Interval is how often the check runs. Typical: 30s-60s.
	Interval time.Duration `json:"interval"`

	// Timeout is the max wait for a response before declaring failure.
	Timeout time.Duration `json:"timeout"`

	// ExpectedStatusCode is the HTTP status code that means healthy (default 200).
	ExpectedStatusCode int `json:"expected_status_code,omitempty"`

	// ExpectedBody is a substring the response body must contain.
	ExpectedBody string `json:"expected_body,omitempty"`

	// Regions are the monitoring locations (e.g. ["us-east", "eu-west"]).
	Regions []string `json:"regions,omitempty"`

	// StatuspageComponentID links this check to a Statuspage.io component for
	// automatic status updates.
	StatuspageComponentID string `json:"statuspage_component_id,omitempty"`

	// Tags for grouping/filtering in the monitoring provider dashboard.
	Tags []string `json:"tags,omitempty"`
}

// DefaultChecks returns the standard set of synthetic checks for all KaiVue
// platform components defined in KAI-375.
func DefaultChecks(baseURLs ComponentURLs) []Check {
	interval := 30 * time.Second
	timeout := 10 * time.Second
	regions := []string{"us-east-1", "eu-west-1", "ap-southeast-1"}

	return []Check{
		{
			Name:               "Cloud Control Plane",
			Type:               CheckHTTP,
			URL:                baseURLs.CloudAPI + "/healthz",
			Interval:           interval,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			ExpectedBody:       `"status":"ok"`,
			Regions:            regions,
			Tags:               []string{"infrastructure", "critical"},
		},
		{
			Name:               "Identity Service",
			Type:               CheckHTTP,
			URL:                baseURLs.Identity + "/healthz",
			Interval:           interval,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			ExpectedBody:       `"status":"ok"`,
			Regions:            regions,
			Tags:               []string{"auth", "critical"},
		},
		{
			Name:               "Cloud Directory",
			Type:               CheckHTTP,
			URL:                baseURLs.Directory + "/healthz",
			Interval:           interval,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			ExpectedBody:       `"status":"ok"`,
			Regions:            regions,
			Tags:               []string{"infrastructure"},
		},
		{
			Name:               "Integrator Portal",
			Type:               CheckHTTP,
			URL:                baseURLs.IntegratorPortal + "/healthz",
			Interval:           interval,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			Regions:            regions,
			Tags:               []string{"portal"},
		},
		{
			Name:               "AI Inference",
			Type:               CheckHTTP,
			URL:                baseURLs.AIInference + "/healthz",
			Interval:           interval,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			Regions:            regions,
			Tags:               []string{"ai"},
		},
		{
			Name:               "Recording Archive",
			Type:               CheckHTTP,
			URL:                baseURLs.RecordingArchive + "/healthz",
			Interval:           interval,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			Regions:            regions,
			Tags:               []string{"storage", "critical"},
		},
		{
			Name:               "Notifications Service",
			Type:               CheckHTTP,
			URL:                baseURLs.Notifications + "/healthz",
			Interval:           interval,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			Regions:            regions,
			Tags:               []string{"notifications"},
		},
		{
			Name:               "Cloud Relay",
			Type:               CheckHTTP,
			URL:                baseURLs.CloudRelay + "/healthz",
			Interval:           interval,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			Regions:            regions,
			Tags:               []string{"streaming", "critical"},
		},
		{
			Name:               "Marketing Site",
			Type:               CheckHTTP,
			URL:                baseURLs.MarketingSite,
			Interval:           60 * time.Second,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			Regions:            regions,
			Tags:               []string{"marketing"},
		},
		{
			Name:               "Documentation",
			Type:               CheckHTTP,
			URL:                baseURLs.Docs,
			Interval:           60 * time.Second,
			Timeout:            timeout,
			ExpectedStatusCode: 200,
			Regions:            regions,
			Tags:               []string{"docs"},
		},
	}
}

// ComponentURLs holds the base URLs for each monitored component. Populated
// from environment or config at startup.
type ComponentURLs struct {
	CloudAPI         string `json:"cloud_api"`
	Identity         string `json:"identity"`
	Directory        string `json:"directory"`
	IntegratorPortal string `json:"integrator_portal"`
	AIInference      string `json:"ai_inference"`
	RecordingArchive string `json:"recording_archive"`
	Notifications    string `json:"notifications"`
	CloudRelay       string `json:"cloud_relay"`
	MarketingSite    string `json:"marketing_site"`
	Docs             string `json:"docs"`
}
