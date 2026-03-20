package onvif

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// MotionCallback is invoked when motion state changes (true = motion detected).
type MotionCallback func(detected bool)

// EventSubscriber manages a PullPoint subscription to an ONVIF camera's event
// service for motion detection events. It uses raw SOAP XML because the
// use-go/onvif library lacks SDK helpers for events.
type EventSubscriber struct {
	xaddr    string
	username string
	password string
	callback MotionCallback

	client *http.Client
	cancel context.CancelFunc
}

// NewEventSubscriber creates a new EventSubscriber for the given ONVIF device.
func NewEventSubscriber(xaddr, username, password string, cb MotionCallback) (*EventSubscriber, error) {
	if xaddr == "" {
		return nil, fmt.Errorf("onvif events: xaddr is required")
	}
	if cb == nil {
		return nil, fmt.Errorf("onvif events: callback is required")
	}
	return &EventSubscriber{
		xaddr:    strings.TrimRight(xaddr, "/"),
		username: username,
		password: password,
		callback: cb,
		client:   &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// Start begins subscribing and polling for events. It blocks until the context
// is cancelled or Stop is called. On transient errors, it waits 5 seconds and
// retries from subscription creation.
func (es *EventSubscriber) Start(ctx context.Context) {
	ctx, es.cancel = context.WithCancel(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := es.subscribeAndPoll(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("onvif events [%s]: %v, retrying in 5s", es.xaddr, err)
		}

		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

// Stop cancels the subscription polling loop.
func (es *EventSubscriber) Stop() {
	if es.cancel != nil {
		es.cancel()
	}
}

// subscribeAndPoll creates a PullPoint subscription and polls for events
// until an error occurs or the context is cancelled.
func (es *EventSubscriber) subscribeAndPoll(ctx context.Context) error {
	eventServiceURL := es.xaddr + "/onvif/event_service"

	// Create PullPoint subscription with 60s termination time.
	subRef, err := es.createPullPointSubscription(ctx, eventServiceURL)
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}

	log.Printf("onvif events [%s]: subscription created at %s", es.xaddr, subRef)

	// Ensure we unsubscribe on exit.
	defer func() {
		unsubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := es.unsubscribe(unsubCtx, subRef); err != nil {
			log.Printf("onvif events [%s]: unsubscribe: %v", es.xaddr, err)
		}
	}()

	// Poll loop: PullMessages every 2s, Renew every 48s.
	pollTicker := time.NewTicker(2 * time.Second)
	defer pollTicker.Stop()

	renewTicker := time.NewTicker(48 * time.Second)
	defer renewTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-renewTicker.C:
			if err := es.renew(ctx, subRef); err != nil {
				return fmt.Errorf("renew: %w", err)
			}

		case <-pollTicker.C:
			motionDetected, hasMotion, err := es.pullMessages(ctx, subRef)
			if err != nil {
				return fmt.Errorf("pull messages: %w", err)
			}
			if hasMotion {
				es.callback(motionDetected)
			}
		}
	}
}

// createPullPointSubscription sends a CreatePullPointSubscription request
// and returns the subscription reference address.
func (es *EventSubscriber) createPullPointSubscription(ctx context.Context, eventServiceURL string) (string, error) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tev="http://www.onvif.org/ver10/events/wsdl"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Header></s:Header>
  <s:Body>
    <tev:CreatePullPointSubscription>
      <tev:InitialTerminationTime>PT60S</tev:InitialTerminationTime>
    </tev:CreatePullPointSubscription>
  </s:Body>
</s:Envelope>`

	respBody, err := es.doSOAP(ctx, eventServiceURL, body)
	if err != nil {
		return "", err
	}

	return parseSubscriptionReference(respBody)
}

// pullMessages sends a PullMessages request and parses motion events.
// Returns (motionDetected, hasMotionEvent, error).
func (es *EventSubscriber) pullMessages(ctx context.Context, subRef string) (bool, bool, error) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
  <s:Header></s:Header>
  <s:Body>
    <tev:PullMessages>
      <tev:Timeout>PT1S</tev:Timeout>
      <tev:MessageLimit>100</tev:MessageLimit>
    </tev:PullMessages>
  </s:Body>
</s:Envelope>`

	respBody, err := es.doSOAP(ctx, subRef, body)
	if err != nil {
		return false, false, err
	}

	return parseMotionEvents(respBody)
}

// renew sends a Renew request to extend the subscription for another 60s.
func (es *EventSubscriber) renew(ctx context.Context, subRef string) error {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Header></s:Header>
  <s:Body>
    <wsnt:Renew>
      <wsnt:TerminationTime>PT60S</wsnt:TerminationTime>
    </wsnt:Renew>
  </s:Body>
</s:Envelope>`

	_, err := es.doSOAP(ctx, subRef, body)
	return err
}

// unsubscribe sends an Unsubscribe request to terminate the subscription.
func (es *EventSubscriber) unsubscribe(ctx context.Context, subRef string) error {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Header></s:Header>
  <s:Body>
    <wsnt:Unsubscribe/>
  </s:Body>
</s:Envelope>`

	_, err := es.doSOAP(ctx, subRef, body)
	return err
}

// doSOAP sends a SOAP request to the given URL and returns the response body.
// If credentials are configured, WS-Security UsernameToken authentication is
// injected into the SOAP Header.
func (es *EventSubscriber) doSOAP(ctx context.Context, url, body string) ([]byte, error) {
	if es.username != "" {
		body = injectWSSecurity(body, es.username, es.password)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	resp, err := es.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

// injectWSSecurity inserts a WS-Security UsernameToken header into the SOAP
// envelope. The password is transmitted as a digest: Base64(SHA1(nonce+created+password)).
func injectWSSecurity(soapXML, username, password string) string {
	nonce := make([]byte, 16)
	rand.Read(nonce) //nolint:errcheck // crypto/rand.Read never returns an error

	created := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// Password digest = Base64(SHA1(nonce + created + password))
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)

	secHeader := fmt.Sprintf(
		`<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd" s:mustUnderstand="1">
      <wsse:UsernameToken>
        <wsse:Username>%s</wsse:Username>
        <wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
        <wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</wsse:Nonce>
        <wsu:Created>%s</wsu:Created>
      </wsse:UsernameToken>
    </wsse:Security>`, username, digest, nonceB64, created)

	// Inject into the SOAP Header
	return strings.Replace(soapXML, "</s:Header>", secHeader+"\n  </s:Header>", 1)
}

// --- XML parsing ---

// soapEnvelope is a minimal SOAP envelope for parsing responses.
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	CreateResponse createPullPointResponse `xml:"CreatePullPointSubscriptionResponse"`
	PullResponse   pullMessagesResponse    `xml:"PullMessagesResponse"`
}

type createPullPointResponse struct {
	SubscriptionReference subscriptionReference `xml:"SubscriptionReference"`
}

type subscriptionReference struct {
	Address string `xml:"Address"`
}

type pullMessagesResponse struct {
	Messages []notificationMessage `xml:"NotificationMessage"`
}

type notificationMessage struct {
	Topic   topicExpression `xml:"Topic"`
	Message messageContent  `xml:"Message"`
}

type topicExpression struct {
	Value string `xml:",chardata"`
}

type messageContent struct {
	InnerMessage innerMessage `xml:"Message"`
}

type innerMessage struct {
	Data simpleItemData `xml:"Data"`
}

type simpleItemData struct {
	SimpleItems []simpleItem `xml:"SimpleItem"`
}

type simpleItem struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

// parseSubscriptionReference extracts the subscription address from a
// CreatePullPointSubscriptionResponse.
func parseSubscriptionReference(body []byte) (string, error) {
	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("parse subscription response: %w", err)
	}

	addr := strings.TrimSpace(env.Body.CreateResponse.SubscriptionReference.Address)
	if addr == "" {
		return "", fmt.Errorf("empty subscription reference address in response")
	}
	return addr, nil
}

// parseMotionEvents scans PullMessagesResponse for motion-related events.
// Returns (motionDetected, hasMotionEvent, error).
func parseMotionEvents(body []byte) (bool, bool, error) {
	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return false, false, fmt.Errorf("parse pull response: %w", err)
	}

	for _, msg := range env.Body.PullResponse.Messages {
		topicLower := strings.ToLower(msg.Topic.Value)
		if !strings.Contains(topicLower, "motion") && !strings.Contains(topicLower, "cellmotion") {
			continue
		}

		for _, item := range msg.Message.InnerMessage.Data.SimpleItems {
			nameLower := strings.ToLower(item.Name)
			if nameLower == "ismotion" || nameLower == "state" {
				valueLower := strings.ToLower(strings.TrimSpace(item.Value))
				detected := valueLower == "true" || valueLower == "1"
				return detected, true, nil
			}
		}
	}

	return false, false, nil
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
