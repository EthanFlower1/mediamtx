package summaries

import (
	"fmt"
	"strings"
	"time"
)

// Formatter transforms raw LLM output into delivery-ready content for
// different channels (HTML email, plain text, Slack markdown).
type Formatter struct{}

// NewFormatter constructs a Formatter.
func NewFormatter() *Formatter {
	return &Formatter{}
}

// FormatPlainText returns the summary as clean plain text suitable for
// email body or logging.
func (f *Formatter) FormatPlainText(s *Summary) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Event Summary (%s)\n", periodLabel(s.Period)))
	sb.WriteString(fmt.Sprintf("Period: %s to %s\n",
		s.StartTime.Format("Jan 2, 2006 15:04 MST"),
		s.EndTime.Format("Jan 2, 2006 15:04 MST")))
	sb.WriteString(fmt.Sprintf("Events analysed: %d\n", s.EventCount))
	sb.WriteString(strings.Repeat("-", 50) + "\n\n")
	sb.WriteString(s.Text)
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s", s.GeneratedAt.Format(time.RFC1123)))
	return sb.String()
}

// FormatHTML returns the summary wrapped in a minimal HTML template for
// email delivery.
func (f *Formatter) FormatHTML(s *Summary) string {
	// Convert newlines and bullet points to HTML.
	body := htmlEscape(s.Text)
	body = strings.ReplaceAll(body, "\n- ", "\n<li>")
	body = strings.ReplaceAll(body, "\n", "<br>\n")

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Event Summary</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
  <h2 style="color: #1a1a1a;">Event Summary (%s)</h2>
  <p style="color: #666; font-size: 14px;">
    %s &mdash; %s | %d events analysed
  </p>
  <hr style="border: 1px solid #e0e0e0;">
  <div style="line-height: 1.6; color: #333;">
    %s
  </div>
  <hr style="border: 1px solid #e0e0e0;">
  <p style="color: #999; font-size: 12px;">
    Generated %s &bull; Kaivue NVR
  </p>
</body>
</html>`,
		periodLabel(s.Period),
		s.StartTime.Format("Jan 2, 2006 15:04 MST"),
		s.EndTime.Format("Jan 2, 2006 15:04 MST"),
		s.EventCount,
		body,
		s.GeneratedAt.Format(time.RFC1123))
}

// FormatSlack returns the summary in Slack mrkdwn format.
func (f *Formatter) FormatSlack(s *Summary) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Event Summary (%s)*\n", periodLabel(s.Period)))
	sb.WriteString(fmt.Sprintf("_%s to %s_ | %d events\n",
		s.StartTime.Format("Jan 2 15:04"),
		s.EndTime.Format("Jan 2 15:04"),
		s.EventCount))
	sb.WriteString("---\n")
	sb.WriteString(s.Text)
	return sb.String()
}

func periodLabel(p SummaryPeriod) string {
	switch p {
	case PeriodDaily:
		return "Daily"
	case PeriodWeekly:
		return "Weekly"
	default:
		return string(p)
	}
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
