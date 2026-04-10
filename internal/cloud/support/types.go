package support

import "time"

// Provider identifies the external ticketing system.
type Provider string

const (
	ProviderInternal  Provider = "internal"
	ProviderZendesk   Provider = "zendesk"
	ProviderFreshdesk Provider = "freshdesk"
)

// TicketStatus represents the lifecycle state of a support ticket.
type TicketStatus string

const (
	StatusOpen              TicketStatus = "open"
	StatusPending           TicketStatus = "pending"
	StatusInProgress        TicketStatus = "in_progress"
	StatusWaitingOnCustomer TicketStatus = "waiting_on_customer"
	StatusResolved          TicketStatus = "resolved"
	StatusClosed            TicketStatus = "closed"
)

// TicketPriority represents the priority level.
type TicketPriority string

const (
	PriorityLow    TicketPriority = "low"
	PriorityNormal TicketPriority = "normal"
	PriorityHigh   TicketPriority = "high"
	PriorityUrgent TicketPriority = "urgent"
)

// CommentSource distinguishes who created a comment.
type CommentSource string

const (
	SourceUser    CommentSource = "user"
	SourceAgent   CommentSource = "agent"
	SourceSystem  CommentSource = "system"
	SourceWebhook CommentSource = "webhook"
)

// Ticket represents a support ticket.
type Ticket struct {
	TicketID    string         `json:"ticket_id"`
	TenantID    string         `json:"tenant_id"`
	ExternalID  *string        `json:"external_id,omitempty"`
	Provider    Provider       `json:"provider"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	Status      TicketStatus   `json:"status"`
	Priority    TicketPriority `json:"priority"`
	RequesterID string         `json:"requester_id"`
	AssigneeID  *string        `json:"assignee_id,omitempty"`
	Tags        string         `json:"tags"`
	Metadata    string         `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Comment represents a comment on a support ticket.
type Comment struct {
	CommentID string        `json:"comment_id"`
	TicketID  string        `json:"ticket_id"`
	TenantID  string        `json:"tenant_id"`
	AuthorID  string        `json:"author_id"`
	Body      string        `json:"body"`
	Source    CommentSource `json:"source"`
	IsPublic  bool          `json:"is_public"`
	CreatedAt time.Time     `json:"created_at"`
}

// ProviderConfig holds the integration credentials for an external provider.
type ProviderConfig struct {
	ConfigID       string   `json:"config_id"`
	TenantID       string   `json:"tenant_id"`
	Provider       Provider `json:"provider"`
	WebhookSecret  string   `json:"webhook_secret"`
	APICredentials string   `json:"api_credentials"`
	Enabled        bool     `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
