package provider

import (
	"context"
	"fmt"
)

// ManualIncidentHelper provides a simplified API for on-call engineers to
// create and manage incidents through the Statuspage.io API. It wraps the
// Provider interface with convenience methods that enforce best practices
// (e.g. always setting affected component status, delivering notifications).
//
// KAI-375: Manual incident creation path for on-call.
type ManualIncidentHelper struct {
	provider Provider
}

// NewManualIncidentHelper wraps a Provider.
func NewManualIncidentHelper(p Provider) *ManualIncidentHelper {
	return &ManualIncidentHelper{provider: p}
}

// CreateManualIncidentRequest holds the fields an on-call engineer provides.
type CreateManualIncidentRequest struct {
	// Title is a short summary (e.g. "Cloud API elevated error rate").
	Title string

	// Body is the initial investigation message.
	Body string

	// Impact is the severity level.
	Impact IncidentImpact

	// AffectedComponentIDs are the Statuspage component IDs affected.
	AffectedComponentIDs []string

	// ComponentStatus is the status to set on all affected components.
	// Defaults to ComponentDegradedPerformance if empty.
	ComponentStatus ComponentStatus

	// Notify controls whether subscribers are notified. Default true.
	Notify bool
}

// CreateManualIncident creates an incident suitable for on-call use. It sets
// affected component statuses and delivers subscriber notifications.
func (h *ManualIncidentHelper) CreateManualIncident(ctx context.Context, req CreateManualIncidentRequest) (Incident, error) {
	if req.Title == "" {
		return Incident{}, fmt.Errorf("statuspage/provider: incident title is required")
	}
	if len(req.AffectedComponentIDs) == 0 {
		return Incident{}, fmt.Errorf("statuspage/provider: at least one affected component is required")
	}

	compStatus := req.ComponentStatus
	if compStatus == "" {
		compStatus = ComponentDegradedPerformance
	}

	components := make(map[string]ComponentStatus, len(req.AffectedComponentIDs))
	for _, id := range req.AffectedComponentIDs {
		components[id] = compStatus
	}

	return h.provider.CreateIncident(ctx, CreateIncidentRequest{
		Name:                 req.Title,
		Status:               IncidentInvestigating,
		ImpactOverride:       req.Impact,
		Body:                 req.Body,
		ComponentIDs:         req.AffectedComponentIDs,
		Components:           components,
		DeliverNotifications: req.Notify,
	})
}

// ResolveIncident resolves an incident and restores all specified components
// to operational status.
func (h *ManualIncidentHelper) ResolveIncident(ctx context.Context, incidentID string, message string, componentIDs []string) (Incident, error) {
	components := make(map[string]ComponentStatus, len(componentIDs))
	for _, id := range componentIDs {
		components[id] = ComponentOperational
	}

	return h.provider.UpdateIncident(ctx, incidentID, UpdateIncidentRequest{
		Status:     IncidentResolved,
		Body:       message,
		Components: components,
	})
}

// PostUpdate adds a status update to an existing incident.
func (h *ManualIncidentHelper) PostUpdate(ctx context.Context, incidentID string, status IncidentStatus, message string) (Incident, error) {
	return h.provider.UpdateIncident(ctx, incidentID, UpdateIncidentRequest{
		Status: status,
		Body:   message,
	})
}
