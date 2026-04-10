package delivery

import "context"

// Sender is the interface that delivery backends must implement.
// Production uses SESSender and TwilioSender; tests use FakeSender.
type Sender interface {
	// Send delivers a message and returns the result. Implementations
	// must be safe for concurrent use.
	Send(ctx context.Context, msg Message) Result

	// Name returns the provider name for logging/tracking.
	Name() ProviderName
}

// SESSender sends email via Amazon SES. The SESClient interface
// decouples from the real AWS SDK so tests can use a fake.
type SESSender struct {
	Client      SESClient
	FromAddress string
}

// SESClient is the minimal interface into the AWS SES SDK.
type SESClient interface {
	SendEmail(ctx context.Context, from, to, subject, body string) (messageID string, err error)
}

// Send delivers an email via SES.
func (s *SESSender) Send(ctx context.Context, msg Message) Result {
	msgID, err := s.Client.SendEmail(ctx, s.FromAddress, msg.To, msg.Subject, msg.Body)
	if err != nil {
		return Result{Status: "failed", Error: err}
	}
	return Result{MessageID: msgID, Status: "sent"}
}

// Name returns "ses".
func (s *SESSender) Name() ProviderName { return ProviderSES }

// TwilioSender sends SMS via Twilio. The TwilioClient interface
// decouples from the real Twilio SDK so tests can use a fake.
type TwilioSender struct {
	Client     TwilioClient
	FromNumber string
}

// TwilioClient is the minimal interface into the Twilio API.
type TwilioClient interface {
	SendSMS(ctx context.Context, from, to, body string) (messageSID string, err error)
}

// Send delivers an SMS via Twilio.
func (t *TwilioSender) Send(ctx context.Context, msg Message) Result {
	sid, err := t.Client.SendSMS(ctx, t.FromNumber, msg.To, msg.Body)
	if err != nil {
		return Result{Status: "failed", Error: err}
	}
	return Result{MessageID: sid, Status: "sent"}
}

// Name returns "twilio".
func (t *TwilioSender) Name() ProviderName { return ProviderTwilio }
