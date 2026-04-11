package whitelabel

import (
	"bytes"
	"html/template"
	"strings"
	"time"
)

// templateData is the struct passed into the branded status page HTML template.
type templateData struct {
	Config          StatusPageConfig
	OverallStatus   string
	OverallLabel    string
	OverallColor    string
	Components      []componentTemplateData
	ActiveIncidents []incidentTemplateData
	GeneratedAt     string
}

type componentTemplateData struct {
	DisplayName string
	Status      string
	StatusLabel string
	StatusColor string
}

type incidentTemplateData struct {
	Title    string
	Severity string
	Status   string
	Started  string
	Resolved string
	Updates  []updateTemplateData
}

type updateTemplateData struct {
	Status  string
	Message string
	Time    string
}

// statusLabel returns a human-readable label for a machine status.
func statusLabel(s string) string {
	switch s {
	case "operational":
		return "Operational"
	case "degraded":
		return "Degraded Performance"
	case "partial_outage":
		return "Partial Outage"
	case "major_outage":
		return "Major Outage"
	default:
		return strings.Title(strings.ReplaceAll(s, "_", " ")) //nolint:staticcheck
	}
}

// statusColor returns a CSS color for a machine status string.
func statusColor(s string) string {
	switch s {
	case "operational":
		return "#22c55e"
	case "degraded":
		return "#f59e0b"
	case "partial_outage":
		return "#f97316"
	case "major_outage":
		return "#ef4444"
	default:
		return "#6b7280"
	}
}

// buildTemplateData converts a PublicStatusPage into the template-friendly
// struct used by the HTML renderer.
func buildTemplateData(page PublicStatusPage) templateData {
	var comps []componentTemplateData
	for _, c := range page.Components {
		comps = append(comps, componentTemplateData{
			DisplayName: c.DisplayName,
			Status:      c.Status,
			StatusLabel: statusLabel(c.Status),
			StatusColor: statusColor(c.Status),
		})
	}

	var incidents []incidentTemplateData
	for _, inc := range page.ActiveIncidents {
		var updates []updateTemplateData
		for _, u := range inc.Updates {
			updates = append(updates, updateTemplateData{
				Status:  statusLabel(u.Status),
				Message: u.Message,
				Time:    u.CreatedAt.Format(time.RFC1123),
			})
		}
		resolved := ""
		if inc.ResolvedAt != nil {
			resolved = inc.ResolvedAt.Format(time.RFC1123)
		}
		incidents = append(incidents, incidentTemplateData{
			Title:    inc.Title,
			Severity: inc.Severity,
			Status:   statusLabel(inc.Status),
			Started:  inc.StartedAt.Format(time.RFC1123),
			Resolved: resolved,
			Updates:  updates,
		})
	}

	return templateData{
		Config:          page.Config,
		OverallStatus:   page.OverallStatus,
		OverallLabel:    statusLabel(page.OverallStatus),
		OverallColor:    statusColor(page.OverallStatus),
		Components:      comps,
		ActiveIncidents: incidents,
		GeneratedAt:     time.Now().UTC().Format(time.RFC1123),
	}
}

// brandedPageTemplate is the Go html/template for the integrator-branded
// public status page.
var brandedPageTemplate = template.Must(template.New("status").Parse(statusPageHTML))

// RenderHTML renders the branded status page as an HTML byte slice.
func RenderHTML(page PublicStatusPage) ([]byte, error) {
	data := buildTemplateData(page)
	var buf bytes.Buffer
	if err := brandedPageTemplate.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const statusPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Config.PageTitle}}</title>
{{- if .Config.FaviconURL}}
<link rel="icon" href="{{.Config.FaviconURL}}">
{{- end}}
<style>
  :root {
    --primary: {{.Config.PrimaryColor}};
    --secondary: {{.Config.SecondaryColor}};
    --accent: {{.Config.AccentColor}};
    --header-bg: {{.Config.HeaderBgColor}};
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f9fafb; color: #111827; }
  .header { background: var(--header-bg); border-bottom: 3px solid var(--primary); padding: 1.5rem 2rem; display: flex; align-items: center; gap: 1rem; }
  .header img { height: 40px; }
  .header h1 { font-size: 1.25rem; color: var(--accent); }
  .container { max-width: 800px; margin: 2rem auto; padding: 0 1rem; }
  .overall-banner { border-radius: 8px; padding: 1.25rem; text-align: center; color: #fff; font-size: 1.1rem; font-weight: 600; margin-bottom: 2rem; }
  .component-list { list-style: none; }
  .component-item { display: flex; justify-content: space-between; align-items: center; padding: 0.875rem 1rem; border: 1px solid #e5e7eb; border-radius: 6px; margin-bottom: 0.5rem; background: #fff; }
  .component-name { font-weight: 500; }
  .component-status { font-size: 0.875rem; font-weight: 600; display: flex; align-items: center; gap: 0.375rem; }
  .status-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
  .incidents-section { margin-top: 2rem; }
  .incidents-section h2 { font-size: 1rem; color: var(--accent); margin-bottom: 1rem; border-bottom: 1px solid #e5e7eb; padding-bottom: 0.5rem; }
  .incident { background: #fff; border: 1px solid #e5e7eb; border-radius: 6px; padding: 1rem; margin-bottom: 0.75rem; }
  .incident-title { font-weight: 600; margin-bottom: 0.25rem; }
  .incident-meta { font-size: 0.8125rem; color: #6b7280; margin-bottom: 0.5rem; }
  .incident-update { font-size: 0.875rem; padding: 0.5rem 0; border-top: 1px solid #f3f4f6; }
  .incident-update-status { font-weight: 600; }
  .footer { text-align: center; padding: 2rem; font-size: 0.8125rem; color: #9ca3af; }
  .footer a { color: var(--primary); text-decoration: none; }
  {{.Config.CustomCSS}}
</style>
</head>
<body>
<header class="header">
{{- if .Config.LogoURL}}
  <img src="{{.Config.LogoURL}}" alt="Logo">
{{- end}}
  <h1>{{.Config.PageTitle}}</h1>
</header>
<main class="container">
  <div class="overall-banner" style="background:{{.OverallColor}}">
    {{.OverallLabel}}
  </div>
  <ul class="component-list">
{{- range .Components}}
    <li class="component-item">
      <span class="component-name">{{.DisplayName}}</span>
      <span class="component-status">
        <span class="status-dot" style="background:{{.StatusColor}}"></span>
        {{.StatusLabel}}
      </span>
    </li>
{{- end}}
  </ul>
{{- if .ActiveIncidents}}
  <section class="incidents-section">
    <h2>Active Incidents</h2>
{{- range .ActiveIncidents}}
    <div class="incident">
      <div class="incident-title">{{.Title}}</div>
      <div class="incident-meta">{{.Severity}} &middot; {{.Status}} &middot; Started {{.Started}}</div>
{{- range .Updates}}
      <div class="incident-update">
        <span class="incident-update-status">{{.Status}}</span> &mdash; {{.Message}}
        <div style="font-size:0.75rem;color:#9ca3af">{{.Time}}</div>
      </div>
{{- end}}
    </div>
{{- end}}
  </section>
{{- end}}
</main>
<footer class="footer">
{{- if .Config.FooterText}}
  <p>{{.Config.FooterText}}</p>
{{- end}}
{{- if .Config.SupportURL}}
  <p><a href="{{.Config.SupportURL}}">Support</a></p>
{{- end}}
  <p>Last updated {{.GeneratedAt}}</p>
</footer>
</body>
</html>
`
