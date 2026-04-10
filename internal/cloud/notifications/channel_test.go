package notifications_test

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// fakeChannel is a test double for DeliveryChannel.
type fakeChannel struct {
	name           string
	types          []notifications.MessageType
	sendFn         func(ctx context.Context, msg notifications.Message) (notifications.DeliveryResult, error)
	batchSendFn    func(ctx context.Context, msg notifications.Message) ([]notifications.DeliveryResult, error)
	checkHealthFn  func(ctx context.Context) (notifications.HealthStatus, error)
}

func (f *fakeChannel) Name() string                                  { return f.name }
func (f *fakeChannel) SupportedTypes() []notifications.MessageType   { return f.types }

func (f *fakeChannel) Send(ctx context.Context, msg notifications.Message) (notifications.DeliveryResult, error) {
	if f.sendFn != nil {
		return f.sendFn(ctx, msg)
	}
	return notifications.DeliveryResult{
		MessageID: msg.ID,
		Recipient: msg.To[0].Address,
		State:     notifications.DeliveryStateDelivered,
		Timestamp: time.Now().UTC(),
	}, nil
}

func (f *fakeChannel) BatchSend(ctx context.Context, msg notifications.Message) ([]notifications.DeliveryResult, error) {
	if f.batchSendFn != nil {
		return f.batchSendFn(ctx, msg)
	}
	results := make([]notifications.DeliveryResult, len(msg.To))
	for i, r := range msg.To {
		results[i] = notifications.DeliveryResult{
			MessageID: msg.ID,
			Recipient: r.Address,
			State:     notifications.DeliveryStateDelivered,
			Timestamp: time.Now().UTC(),
		}
	}
	return results, nil
}

func (f *fakeChannel) CheckHealth(ctx context.Context) (notifications.HealthStatus, error) {
	if f.checkHealthFn != nil {
		return f.checkHealthFn(ctx)
	}
	return notifications.HealthStatus{
		Healthy:   true,
		Latency:   5 * time.Millisecond,
		Message:   "ok",
		CheckedAt: time.Now().UTC(),
	}, nil
}

func TestMessageValidate(t *testing.T) {
	tests := []struct {
		name    string
		msg     notifications.Message
		wantErr bool
	}{
		{
			name:    "empty type",
			msg:     notifications.Message{TenantID: "t1", To: []notifications.Recipient{{Address: "a@b.c"}}, Body: "hi"},
			wantErr: true,
		},
		{
			name:    "empty tenant",
			msg:     notifications.Message{Type: notifications.MessageTypeEmail, To: []notifications.Recipient{{Address: "a@b.c"}}, Body: "hi"},
			wantErr: true,
		},
		{
			name:    "no recipients",
			msg:     notifications.Message{Type: notifications.MessageTypeEmail, TenantID: "t1", Body: "hi"},
			wantErr: true,
		},
		{
			name:    "empty address",
			msg:     notifications.Message{Type: notifications.MessageTypeEmail, TenantID: "t1", To: []notifications.Recipient{{Address: ""}}, Body: "hi"},
			wantErr: true,
		},
		{
			name:    "no body or template",
			msg:     notifications.Message{Type: notifications.MessageTypeEmail, TenantID: "t1", To: []notifications.Recipient{{Address: "a@b.c"}}},
			wantErr: true,
		},
		{
			name:    "valid with body",
			msg:     notifications.Message{Type: notifications.MessageTypeEmail, TenantID: "t1", To: []notifications.Recipient{{Address: "a@b.c"}}, Body: "hi"},
			wantErr: false,
		},
		{
			name:    "valid with template",
			msg:     notifications.Message{Type: notifications.MessageTypeSMS, TenantID: "t1", To: []notifications.Recipient{{Address: "+1234567890"}}, TemplateID: "tmpl-1"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestChannelRegistryRegisterAndGet(t *testing.T) {
	reg := notifications.NewChannelRegistry()
	ch := &fakeChannel{
		name:  "test_email",
		types: []notifications.MessageType{notifications.MessageTypeEmail},
	}
	reg.Register(ch)

	got := reg.Get("test_email")
	if got == nil {
		t.Fatal("expected channel, got nil")
	}
	if got.Name() != "test_email" {
		t.Errorf("expected name test_email, got %s", got.Name())
	}

	if reg.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent channel")
	}
}

func TestChannelRegistryDuplicatePanics(t *testing.T) {
	reg := notifications.NewChannelRegistry()
	ch := &fakeChannel{name: "dup", types: []notifications.MessageType{notifications.MessageTypeSMS}}
	reg.Register(ch)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	reg.Register(ch)
}

func TestChannelRegistryForType(t *testing.T) {
	reg := notifications.NewChannelRegistry()
	email := &fakeChannel{name: "email", types: []notifications.MessageType{notifications.MessageTypeEmail}}
	sms := &fakeChannel{name: "sms", types: []notifications.MessageType{notifications.MessageTypeSMS, notifications.MessageTypeWhatsApp}}
	reg.Register(email)
	reg.Register(sms)

	emailChs := reg.ForType(notifications.MessageTypeEmail)
	if len(emailChs) != 1 || emailChs[0].Name() != "email" {
		t.Errorf("expected [email], got %v", emailChs)
	}

	whatsappChs := reg.ForType(notifications.MessageTypeWhatsApp)
	if len(whatsappChs) != 1 || whatsappChs[0].Name() != "sms" {
		t.Errorf("expected [sms], got %v", whatsappChs)
	}

	voiceChs := reg.ForType(notifications.MessageTypeVoice)
	if len(voiceChs) != 0 {
		t.Errorf("expected no voice channels, got %d", len(voiceChs))
	}
}

func TestChannelRegistryHealthCheckAll(t *testing.T) {
	reg := notifications.NewChannelRegistry()
	healthy := &fakeChannel{name: "healthy", types: []notifications.MessageType{notifications.MessageTypeEmail}}
	unhealthy := &fakeChannel{
		name:  "unhealthy",
		types: []notifications.MessageType{notifications.MessageTypeSMS},
		checkHealthFn: func(_ context.Context) (notifications.HealthStatus, error) {
			return notifications.HealthStatus{
				Healthy:   false,
				Message:   "provider down",
				CheckedAt: time.Now().UTC(),
			}, nil
		},
	}
	reg.Register(healthy)
	reg.Register(unhealthy)

	results := reg.HealthCheckAll(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results["healthy"].Healthy {
		t.Error("expected healthy=true for healthy channel")
	}
	if results["unhealthy"].Healthy {
		t.Error("expected healthy=false for unhealthy channel")
	}
}

func TestChannelRegistryAll(t *testing.T) {
	reg := notifications.NewChannelRegistry()
	reg.Register(&fakeChannel{name: "a", types: []notifications.MessageType{notifications.MessageTypeEmail}})
	reg.Register(&fakeChannel{name: "b", types: []notifications.MessageType{notifications.MessageTypeSMS}})

	all := reg.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(all))
	}
}
