// Package alerts provides email notification delivery and alert evaluation
// for the NVR subsystem.
package alerts

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// maxRetries is the number of delivery attempts before giving up.
const maxRetries = 3

// EmailSender sends alert emails using SMTP.
type EmailSender struct {
	DB *db.DB
}

// SendTestEmail sends a test email to verify SMTP configuration.
func (s *EmailSender) SendTestEmail(cfg *db.SMTPConfig, to string) error {
	subject := "Raikada - SMTP Test"
	body := fmt.Sprintf("This is a test email from Raikada.\n\nSent at: %s\n\nIf you received this email, your SMTP configuration is working correctly.",
		time.Now().UTC().Format(time.RFC3339))
	return s.sendMail(cfg, to, subject, body)
}

// SendAlertEmail sends an email notification for an alert.
func (s *EmailSender) SendAlertEmail(cfg *db.SMTPConfig, to string, alert *db.Alert) error {
	subject := fmt.Sprintf("Raikada Alert: %s [%s]", alert.RuleType, alert.Severity)
	body := fmt.Sprintf("Alert: %s\n\nSeverity: %s\nType: %s\nTime: %s\n\nDetails:\n%s",
		alert.Message, alert.Severity, alert.RuleType, alert.CreatedAt, alert.Details)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := s.sendMail(cfg, to, subject, body); err != nil {
			lastErr = err
			log.Printf("[NVR] [WARN] [alerts] email send attempt %d/%d failed: %v", attempt, maxRetries, err)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("email delivery failed after %d attempts: %w", maxRetries, lastErr)
}

// sendMail performs the actual SMTP send.
func (s *EmailSender) sendMail(cfg *db.SMTPConfig, to, subject, body string) error {
	if cfg.Host == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	msg := buildMessage(cfg.FromAddr, to, subject, body)

	if cfg.TLSEnabled {
		return s.sendTLS(cfg, addr, to, msg)
	}
	return s.sendPlain(cfg, addr, to, msg)
}

// sendTLS sends mail using STARTTLS or direct TLS.
func (s *EmailSender) sendTLS(cfg *db.SMTPConfig, addr, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName: cfg.Host,
	}

	// Try STARTTLS first (port 587), fall back to direct TLS (port 465).
	c, err := smtp.Dial(addr)
	if err != nil {
		// Try direct TLS connection.
		conn, tlsErr := tls.Dial("tcp", addr, tlsConfig)
		if tlsErr != nil {
			return fmt.Errorf("connect to SMTP server: dial failed (%v), TLS failed (%v)", err, tlsErr)
		}
		c, err = smtp.NewClient(conn, cfg.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("create SMTP client: %w", err)
		}
	} else {
		// STARTTLS upgrade.
		if err := c.StartTLS(tlsConfig); err != nil {
			c.Close()
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	defer c.Close()

	return s.deliver(c, cfg, to, msg)
}

// sendPlain sends mail without TLS.
func (s *EmailSender) sendPlain(cfg *db.SMTPConfig, addr, to string, msg []byte) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer c.Close()

	return s.deliver(c, cfg, to, msg)
}

// deliver authenticates (if credentials are set) and sends the message.
func (s *EmailSender) deliver(c *smtp.Client, cfg *db.SMTPConfig, to string, msg []byte) error {
	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}

	if err := c.Mail(cfg.FromAddr); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}

	recipients := strings.Split(to, ",")
	for _, rcpt := range recipients {
		rcpt = strings.TrimSpace(rcpt)
		if rcpt == "" {
			continue
		}
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close message: %w", err)
	}

	return c.Quit()
}

// buildMessage constructs an RFC 2822 email message.
func buildMessage(from, to, subject, body string) []byte {
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nDate: %s\r\n\r\n",
		from, to, subject, time.Now().UTC().Format(time.RFC1123Z))
	return []byte(headers + body)
}
