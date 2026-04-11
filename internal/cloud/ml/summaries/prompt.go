package summaries

import (
	"fmt"
	"sort"
	"strings"
)

// systemPrompt is the fixed system instruction for the LLM. It must never
// contain tenant data and is shared across all tenants.
const systemPrompt = `You are a security camera event summariser for a professional NVR system.
Given a structured event report, produce a concise natural language summary.

Rules:
- Be professional and factual.
- Highlight security-relevant events (tamper, loitering, falls, camera offline).
- Group routine events (motion, person/vehicle detected) into aggregate counts.
- Never fabricate events that are not in the input.
- Keep the summary under 300 words.
- Use bullet points for notable incidents.
- End with a one-sentence overall assessment.`

// PromptBuilder transforms AggregatedEvents into a prompt string suitable
// for an LLM. The prompt is deterministic for a given input, which aids
// testing and reproducibility.
type PromptBuilder struct{}

// NewPromptBuilder constructs a PromptBuilder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// Build produces the user-role prompt from aggregated events.
func (pb *PromptBuilder) Build(agg *AggregatedEvents) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Event Report for period %s to %s\n",
		agg.StartTime.Format("2006-01-02 15:04 MST"),
		agg.EndTime.Format("2006-01-02 15:04 MST")))
	sb.WriteString(fmt.Sprintf("Total events: %d across %d cameras\n\n",
		agg.TotalEvents, len(agg.ByCameraCategory)))

	// Global category breakdown.
	sb.WriteString("Category Totals:\n")
	cats := sortedCategories(agg.TotalByCategory)
	for _, cat := range cats {
		sb.WriteString(fmt.Sprintf("  - %s: %d\n", cat, agg.TotalByCategory[cat]))
	}
	sb.WriteString("\n")

	// Per-camera breakdown.
	sb.WriteString("Per-Camera Breakdown:\n")
	cameraIDs := sortedKeys(agg.ByCameraCategory)
	for _, camID := range cameraIDs {
		cats := agg.ByCameraCategory[camID]
		parts := make([]string, 0, len(cats))
		for cat, count := range cats {
			parts = append(parts, fmt.Sprintf("%s=%d", cat, count))
		}
		sort.Strings(parts)
		sb.WriteString(fmt.Sprintf("  %s: %s\n", camID, strings.Join(parts, ", ")))
	}
	sb.WriteString("\n")

	// Notable events.
	if len(agg.NotableEvents) > 0 {
		sb.WriteString("Notable Events (require attention):\n")
		for _, ev := range agg.NotableEvents {
			detail := ev.Detail
			if detail == "" {
				detail = "no details"
			}
			sb.WriteString(fmt.Sprintf("  - [%s] %s on camera %s: %s\n",
				ev.Timestamp.Format("15:04"), ev.Category, ev.CameraID, detail))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Please summarise these events for the site administrator.")
	return sb.String()
}

// SystemPrompt returns the system-level instruction for the LLM.
// Kept separate so callers can construct the chat format their Triton
// model expects.
func (pb *PromptBuilder) SystemPrompt() string {
	return systemPrompt
}

// sortedCategories returns category keys in sorted order for deterministic output.
func sortedCategories(m map[EventCategory]int) []EventCategory {
	keys := make([]EventCategory, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// sortedKeys returns string-map keys in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
