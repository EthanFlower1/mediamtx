package support_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/support"
)

func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

var seqID int

func testIDGen() string {
	seqID++
	return fmt.Sprintf("test-%04d", seqID)
}

func newService(t *testing.T) *support.Service {
	t.Helper()
	db := openTestDB(t)
	svc, err := support.NewService(support.Config{
		DB:    db,
		IDGen: testIDGen,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestCreateAndGetTicket(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	ticket, err := svc.CreateTicket(ctx, support.Ticket{
		TenantID:    "tenant-1",
		Subject:     "Camera offline",
		Description: "Camera 3 in lobby is not responding",
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ticket.TicketID == "" {
		t.Fatal("expected ticket_id")
	}
	if ticket.Status != support.StatusOpen {
		t.Errorf("expected open, got %s", ticket.Status)
	}

	got, err := svc.GetTicket(ctx, "tenant-1", ticket.TicketID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Subject != "Camera offline" {
		t.Errorf("subject = %q", got.Subject)
	}
}

func TestGetTicketNotFound(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	_, err := svc.GetTicket(ctx, "tenant-1", "nonexistent")
	if err != support.ErrTicketNotFound {
		t.Errorf("expected ErrTicketNotFound, got %v", err)
	}
}

func TestListTicketsWithStatusFilter(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.CreateTicket(ctx, support.Ticket{
		TenantID: "tenant-1", Subject: "Open ticket", RequesterID: "user-1",
	})
	ticket2, _ := svc.CreateTicket(ctx, support.Ticket{
		TenantID: "tenant-1", Subject: "Resolved ticket", RequesterID: "user-1",
	})
	svc.UpdateTicketStatus(ctx, "tenant-1", ticket2.TicketID, support.StatusResolved)

	// All tickets
	all, err := svc.ListTickets(ctx, "tenant-1", nil)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}

	// Only open
	open := support.StatusOpen
	filtered, err := svc.ListTickets(ctx, "tenant-1", &open)
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 open, got %d", len(filtered))
	}
}

func TestUpdateTicketStatus(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	ticket, _ := svc.CreateTicket(ctx, support.Ticket{
		TenantID: "tenant-1", Subject: "Test", RequesterID: "user-1",
	})

	if err := svc.UpdateTicketStatus(ctx, "tenant-1", ticket.TicketID, support.StatusInProgress); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := svc.GetTicket(ctx, "tenant-1", ticket.TicketID)
	if got.Status != support.StatusInProgress {
		t.Errorf("expected in_progress, got %s", got.Status)
	}
}

func TestUpdateTicketStatusNotFound(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	err := svc.UpdateTicketStatus(ctx, "tenant-1", "nonexistent", support.StatusClosed)
	if err != support.ErrTicketNotFound {
		t.Errorf("expected ErrTicketNotFound, got %v", err)
	}
}

func TestAddAndListComments(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	ticket, _ := svc.CreateTicket(ctx, support.Ticket{
		TenantID: "tenant-1", Subject: "Need help", RequesterID: "user-1",
	})

	c1, err := svc.AddComment(ctx, support.Comment{
		TicketID: ticket.TicketID, TenantID: "tenant-1",
		AuthorID: "user-1", Body: "Can someone help?", IsPublic: true,
	})
	if err != nil {
		t.Fatalf("add comment 1: %v", err)
	}
	if c1.CommentID == "" {
		t.Fatal("expected comment_id")
	}

	svc.AddComment(ctx, support.Comment{
		TicketID: ticket.TicketID, TenantID: "tenant-1",
		AuthorID: "agent-1", Body: "Looking into it", Source: support.SourceAgent, IsPublic: true,
	})

	comments, err := svc.ListComments(ctx, "tenant-1", ticket.TicketID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].Source != support.SourceUser {
		t.Errorf("first comment source = %s, want user", comments[0].Source)
	}
	if comments[1].Source != support.SourceAgent {
		t.Errorf("second comment source = %s, want agent", comments[1].Source)
	}
}

func TestUpsertAndGetProviderConfig(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	pc, err := svc.UpsertProviderConfig(ctx, support.ProviderConfig{
		TenantID:      "tenant-1",
		Provider:      support.ProviderZendesk,
		WebhookSecret: "whsec_test123",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if pc.ConfigID == "" {
		t.Fatal("expected config_id")
	}

	got, err := svc.GetProviderConfig(ctx, "tenant-1", support.ProviderZendesk)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.WebhookSecret != "whsec_test123" {
		t.Errorf("webhook_secret = %q", got.WebhookSecret)
	}
}

func TestGetProviderConfigNotFound(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	_, err := svc.GetProviderConfig(ctx, "tenant-1", support.ProviderFreshdesk)
	if err != support.ErrProviderNotFound {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.CreateTicket(ctx, support.Ticket{
		TenantID: "tenant-1", Subject: "Tenant 1 issue", RequesterID: "user-1",
	})
	svc.CreateTicket(ctx, support.Ticket{
		TenantID: "tenant-2", Subject: "Tenant 2 issue", RequesterID: "user-2",
	})

	t1, _ := svc.ListTickets(ctx, "tenant-1", nil)
	t2, _ := svc.ListTickets(ctx, "tenant-2", nil)

	if len(t1) != 1 || t1[0].Subject != "Tenant 1 issue" {
		t.Errorf("tenant-1 should only see its own ticket")
	}
	if len(t2) != 1 || t2[0].Subject != "Tenant 2 issue" {
		t.Errorf("tenant-2 should only see its own ticket")
	}
}

func TestExternalTicketSync(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	extID := "ZD-12345"
	ticket, err := svc.CreateTicket(ctx, support.Ticket{
		TenantID:    "tenant-1",
		ExternalID:  &extID,
		Provider:    support.ProviderZendesk,
		Subject:     "Synced from Zendesk",
		Description: "Customer reported via Zendesk",
		RequesterID: "user-1",
		Priority:    support.PriorityHigh,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ticket.Provider != support.ProviderZendesk {
		t.Errorf("provider = %s", ticket.Provider)
	}
	if ticket.ExternalID == nil || *ticket.ExternalID != "ZD-12345" {
		t.Errorf("external_id = %v", ticket.ExternalID)
	}

	// Add a webhook-sourced comment
	svc.AddComment(ctx, support.Comment{
		TicketID: ticket.TicketID, TenantID: "tenant-1",
		AuthorID: "zendesk-agent", Body: "Response from Zendesk agent",
		Source: support.SourceWebhook, IsPublic: true,
	})

	comments, _ := svc.ListComments(ctx, "tenant-1", ticket.TicketID)
	if len(comments) != 1 || comments[0].Source != support.SourceWebhook {
		t.Errorf("expected webhook comment, got %v", comments)
	}
}
