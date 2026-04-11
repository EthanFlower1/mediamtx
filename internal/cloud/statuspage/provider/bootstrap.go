package provider

import (
	"context"
	"fmt"
	"log/slog"
)

// DesiredComponent describes a component that should exist on the status page.
type DesiredComponent struct {
	Name        string
	Description string
	GroupName   string // empty = ungrouped
}

// DefaultComponents returns the components required by KAI-375.
func DefaultComponents() []DesiredComponent {
	return []DesiredComponent{
		{Name: "Cloud Control Plane", Description: "Core API and orchestration layer", GroupName: "Infrastructure"},
		{Name: "Identity", Description: "Authentication and authorization service", GroupName: "Infrastructure"},
		{Name: "Cloud Directory", Description: "Device and tenant registry", GroupName: "Infrastructure"},
		{Name: "Integrator Portal", Description: "Partner management dashboard", GroupName: "Applications"},
		{Name: "AI Inference", Description: "AI model inference pipeline", GroupName: "Applications"},
		{Name: "Recording Archive", Description: "Cloud recording storage (R2/S3)", GroupName: "Storage"},
		{Name: "Notifications", Description: "Push, email, and SMS notification delivery", GroupName: "Applications"},
		{Name: "Cloud Relay", Description: "Real-time video streaming relay", GroupName: "Streaming"},
		{Name: "Marketing Site", Description: "Public website (www)", GroupName: "Web Properties"},
		{Name: "Docs", Description: "Developer documentation portal", GroupName: "Web Properties"},
	}
}

// BootstrapResult holds the IDs of components and groups created or matched
// during bootstrap.
type BootstrapResult struct {
	// ComponentsByName maps component name to Statuspage.io component ID.
	ComponentsByName map[string]string

	// GroupsByName maps group name to Statuspage.io group ID.
	GroupsByName map[string]string

	// Created is the count of newly created components.
	Created int

	// Existing is the count of components that already existed.
	Existing int
}

// Bootstrap ensures all desired components exist on the Statuspage.io page.
// It creates missing components and groups, and returns a mapping of names to
// IDs for use in automation rules.
//
// This is idempotent: running it multiple times converges to the desired state.
func Bootstrap(ctx context.Context, p Provider, desired []DesiredComponent, logger *slog.Logger) (BootstrapResult, error) {
	if logger == nil {
		logger = slog.Default()
	}

	result := BootstrapResult{
		ComponentsByName: make(map[string]string),
		GroupsByName:     make(map[string]string),
	}

	// Fetch existing components.
	existing, err := p.ListComponents(ctx)
	if err != nil {
		return result, fmt.Errorf("bootstrap: list components: %w", err)
	}
	existingByName := make(map[string]Component, len(existing))
	for _, c := range existing {
		existingByName[c.Name] = c
	}

	// Fetch existing groups.
	groups, err := p.ListComponentGroups(ctx)
	if err != nil {
		return result, fmt.Errorf("bootstrap: list groups: %w", err)
	}
	groupsByName := make(map[string]string, len(groups))
	for _, g := range groups {
		groupsByName[g.Name] = g.ID
	}

	// Ensure groups exist.
	neededGroups := make(map[string]bool)
	for _, d := range desired {
		if d.GroupName != "" {
			neededGroups[d.GroupName] = true
		}
	}
	for name := range neededGroups {
		if _, ok := groupsByName[name]; ok {
			result.GroupsByName[name] = groupsByName[name]
			continue
		}
		g, err := p.CreateComponentGroup(ctx, ComponentGroup{Name: name})
		if err != nil {
			return result, fmt.Errorf("bootstrap: create group %q: %w", name, err)
		}
		logger.Info("created component group", "name", name, "id", g.ID)
		groupsByName[name] = g.ID
		result.GroupsByName[name] = g.ID
	}

	// Ensure components exist.
	for _, d := range desired {
		if c, ok := existingByName[d.Name]; ok {
			result.ComponentsByName[d.Name] = c.ID
			result.Existing++
			continue
		}

		comp := Component{
			Name:        d.Name,
			Description: d.Description,
			Status:      ComponentOperational,
			Showcase:    true,
		}
		if d.GroupName != "" {
			comp.GroupID = groupsByName[d.GroupName]
		}

		created, err := p.CreateComponent(ctx, comp)
		if err != nil {
			return result, fmt.Errorf("bootstrap: create component %q: %w", d.Name, err)
		}
		logger.Info("created component", "name", d.Name, "id", created.ID)
		result.ComponentsByName[d.Name] = created.ID
		result.Created++
	}

	return result, nil
}
