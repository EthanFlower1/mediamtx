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
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MotionCallback is invoked when motion state changes (true = motion detected).
type MotionCallback func(detected bool)

// EventSubscriber manages an ONVIF WS-BaseNotification subscription where the
// camera pushes events to the NVR via HTTP POST (no polling).
type EventSubscriber struct {
	xaddr           string
	eventServiceURL string
	username        string
	password        string
	callback        MotionCallback
	callbackURL     string // URL the camera will POST to

	client *http.Client
	cancel context.CancelFunc
	subRef string // subscription reference for renewal/unsubscribe
}

// NewEventSubscriber creates a new push-based event subscriber.
// callbackURL is the NVR's webhook URL that the camera will POST notifications to.
func NewEventSubscriber(xaddr, username, password, callbackURL string, cb MotionCallback) (*EventSubscriber, error) {
	if xaddr == "" {
		return nil, fmt.Errorf("onvif events: xaddr is required")
	}
	if cb == nil {
		return nil, fmt.Errorf("onvif events: callback is required")
	}

	dev, err := connectDevice(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("onvif events: connect to device: %w", err)
	}

	services := dev.GetServices()
	eventURL, ok := services["events"]
	if !ok {
		eventURL, ok = services["event"]
	}
	if !ok || eventURL == "" {
		return nil, fmt.Errorf("onvif events: camera does not expose an event service")
	}

	return &EventSubscriber{
		xaddr:           strings.TrimRight(xaddr, "/"),
		eventServiceURL: eventURL,
		username:        username,
		password:        password,
		callback:        cb,
		callbackURL:     callbackURL,
		client:          &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Start subscribes for push notifications and blocks until cancelled.
// It renews the subscription periodically and retries on errors.
func (es *EventSubscriber) Start(ctx context.Context) {
	ctx, es.cancel = context.WithCancel(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := es.subscribeAndMaintain(ctx)
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

// Stop cancels the subscription.
func (es *EventSubscriber) Stop() {
	if es.cancel != nil {
		es.cancel()
	}
	// Try to unsubscribe so the camera stops sending notifications.
	if es.subRef != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		es.unsubscribe(ctx, es.subRef)
	}
}

// subscribeAndMaintain creates a WS-BaseNotification subscription telling the
// camera to POST events to our callback URL, then renews it periodically.
func (es *EventSubscriber) subscribeAndMaintain(ctx context.Context) error {
	subRef, err := es.subscribe(ctx)
	if err != nil {
		// If push subscription fails, fall back to PullPoint polling.
		log.Printf("onvif events [%s]: push subscribe failed (%v), falling back to PullPoint polling", es.xaddr, err)
		return es.fallbackPullPoint(ctx)
	}

	es.subRef = subRef
	log.Printf("onvif events [%s]: push subscription active, camera will POST to %s", es.xaddr, es.callbackURL)

	// Renew every 48s (subscription has 60s termination time).
	renewTicker := time.NewTicker(48 * time.Second)
	defer renewTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-renewTicker.C:
			if err := es.renew(ctx, subRef); err != nil {
				return fmt.Errorf("renew push subscription: %w", err)
			}
		}
	}
}

// subscribe sends a WS-BaseNotification Subscribe request with our callback URL.
func (es *EventSubscriber) subscribe(ctx context.Context) (string, error) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tev="http://www.onvif.org/ver10/events/wsdl"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
            xmlns:wsa="http://www.w3.org/2005/08/addressing">
  <s:Header></s:Header>
  <s:Body>
    <wsnt:Subscribe>
      <wsnt:ConsumerReference>
        <wsa:Address>%s</wsa:Address>
      </wsnt:ConsumerReference>
      <wsnt:InitialTerminationTime>PT60S</wsnt:InitialTerminationTime>
    </wsnt:Subscribe>
  </s:Body>
</s:Envelope>`, es.callbackURL)

	respBody, err := es.doSOAP(ctx, es.eventServiceURL, body)
	if err != nil {
		return "", err
	}

	return parseSubscribeReference(respBody)
}

// HandleNotification processes an incoming SOAP notification from the camera.
// This is called by the HTTP handler when the camera POSTs to the callback URL.
func (es *EventSubscriber) HandleNotification(body []byte) {
	detected, hasMotion, err := parseMotionEvents(body)
	if err != nil {
		log.Printf("onvif events [%s]: parse push notification error: %v", es.xaddr, err)
		return
	}
	if hasMotion {
		log.Printf("onvif events [%s]: motion=%v (push)", es.xaddr, detected)
		es.callback(detected)
	}
}

// fallbackPullPoint uses the old polling approach when push isn't supported.
func (es *EventSubscriber) fallbackPullPoint(ctx context.Context) error {
	subRef, err := es.createPullPointSubscription(ctx)
	if err != nil {
		return fmt.Errorf("create pullpoint: %w", err)
	}

	log.Printf("onvif events [%s]: PullPoint fallback active at %s", es.xaddr, subRef)

	defer func() {
		unsubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		es.unsubscribe(unsubCtx, subRef)
	}()

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
			detected, hasMotion, err := es.pullMessages(ctx, subRef)
			if err != nil {
				return fmt.Errorf("pull messages: %w", err)
			}
			if hasMotion {
				log.Printf("onvif events [%s]: motion=%v (poll)", es.xaddr, detected)
				es.callback(detected)
			}
		}
	}
}

// createPullPointSubscription creates a PullPoint subscription (fallback).
func (es *EventSubscriber) createPullPointSubscription(ctx context.Context) (string, error) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
  <s:Header></s:Header>
  <s:Body>
    <tev:CreatePullPointSubscription>
      <tev:InitialTerminationTime>PT60S</tev:InitialTerminationTime>
    </tev:CreatePullPointSubscription>
  </s:Body>
</s:Envelope>`

	respBody, err := es.doSOAP(ctx, es.eventServiceURL, body)
	if err != nil {
		return "", err
	}
	return parseSubscriptionReference(respBody)
}

// pullMessages sends a PullMessages request (fallback).
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

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

func injectWSSecurity(soapXML, username, password string) string {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	created := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

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

	return strings.Replace(soapXML, "</s:Header>", secHeader+"\n  </s:Header>", 1)
}

// --- Callback HTTP Handler ---

// CallbackManager manages ONVIF notification callbacks for multiple cameras.
type CallbackManager struct {
	mu          sync.RWMutex
	subscribers map[string]*EventSubscriber // camera ID -> subscriber
}

// NewCallbackManager creates a new CallbackManager.
func NewCallbackManager() *CallbackManager {
	return &CallbackManager{
		subscribers: make(map[string]*EventSubscriber),
	}
}

// Register associates a camera ID with an EventSubscriber for callback routing.
func (m *CallbackManager) Register(cameraID string, sub *EventSubscriber) {
	m.mu.Lock()
	m.subscribers[cameraID] = sub
	m.mu.Unlock()
}

// Unregister removes a camera's callback association.
func (m *CallbackManager) Unregister(cameraID string) {
	m.mu.Lock()
	delete(m.subscribers, cameraID)
	m.mu.Unlock()
}

// HandleCallback processes an incoming ONVIF notification for the given camera ID.
func (m *CallbackManager) HandleCallback(cameraID string, body []byte) {
	m.mu.RLock()
	sub, ok := m.subscribers[cameraID]
	m.mu.RUnlock()

	if ok {
		sub.HandleNotification(body)
	}
}

// GetLocalIP returns the best local IP address for the callback URL.
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

// --- XML parsing ---

type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	CreateResponse    createPullPointResponse `xml:"CreatePullPointSubscriptionResponse"`
	SubscribeResponse subscribeResponse       `xml:"SubscribeResponse"`
	PullResponse      pullMessagesResponse    `xml:"PullMessagesResponse"`
	Notify            notifyBody              `xml:"Notify"`
}

type subscribeResponse struct {
	SubscriptionReference subscriptionReference `xml:"SubscriptionReference"`
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

type notifyBody struct {
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

func parseSubscriptionReference(body []byte) (string, error) {
	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("parse subscription response: %w", err)
	}
	addr := strings.TrimSpace(env.Body.CreateResponse.SubscriptionReference.Address)
	if addr == "" {
		return "", fmt.Errorf("empty subscription reference address")
	}
	return addr, nil
}

func parseSubscribeReference(body []byte) (string, error) {
	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("parse subscribe response: %w", err)
	}
	addr := strings.TrimSpace(env.Body.SubscribeResponse.SubscriptionReference.Address)
	if addr == "" {
		return "", fmt.Errorf("empty subscribe reference address")
	}
	return addr, nil
}

// parseMotionEvents scans for motion events in PullMessages or Notify responses.
func parseMotionEvents(body []byte) (bool, bool, error) {
	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return false, false, fmt.Errorf("parse response: %w", err)
	}

	// Check both PullMessages and Notify message arrays.
	messages := env.Body.PullResponse.Messages
	if len(messages) == 0 {
		messages = env.Body.Notify.Messages
	}

	for _, msg := range messages {
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
