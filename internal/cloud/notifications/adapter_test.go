package notifications

import (
	"context"
	"testing"
	"time"
)

// -----------------------------------------------------------------------
// Compile-time interface checks
// -----------------------------------------------------------------------

var _ DeliveryChannel = (*CommsChannelAdapter)(nil)
var _ DeliveryChannel = (*PushChannelAdapter)(nil)

// -----------------------------------------------------------------------
// Mock comms channel
// -----------------------------------------------------------------------

type mockCommsChannel struct {
	sendResult       CommsDeliveryResult
	batchResults     []CommsDeliveryResult
	healthErr        error
	sendCalled       bool
	batchSendCalled  bool
	healthCalled     bool
	lastMsg          CommsMessage
	lastBatchMsgs    []CommsMessage
}

func (m *mockCommsChannel) Send(_ context.Context, msg CommsMessage) CommsDeliveryResult {
	m.sendCalled = true
	m.lastMsg = msg
	return m.sendResult
}

func (m *mockCommsChannel) BatchSend(_ context.Context, msgs []CommsMessage) []CommsDeliveryResult {
	m.batchSendCalled = true
	m.lastBatchMsgs = msgs
	return m.batchResults
}

func (m *mockCommsChannel) CheckHealth(_ context.Context) error {
	m.healthCalled = true
	return m.healthErr
}

func (m *mockCommsChannel) Type() ChannelType {
	return ChannelSlack
}

// -----------------------------------------------------------------------
// Mock push channel
// -----------------------------------------------------------------------

type mockPushChannel struct {
	sendResult       PushDeliveryResult
	batchResults     []PushDeliveryResult
	healthErr        error
	sendCalled       bool
	batchSendCalled  bool
	healthCalled     bool
	lastMsg          PushMessage
	lastBatchMsg     BatchMessage
}

func (m *mockPushChannel) Send(_ context.Context, msg PushMessage) (PushDeliveryResult, error) {
	m.sendCalled = true
	m.lastMsg = msg
	return m.sendResult, nil
}

func (m *mockPushChannel) BatchSend(_ context.Context, msg BatchMessage) ([]PushDeliveryResult, error) {
	m.batchSendCalled = true
	m.lastBatchMsg = msg
	return m.batchResults, nil
}

func (m *mockPushChannel) CheckHealth(_ context.Context) error {
	m.healthCalled = true
	return m.healthErr
}

func (m *mockPushChannel) Type() ChannelType {
	return ChannelPush
}

// -----------------------------------------------------------------------
// CommsChannelAdapter tests
// -----------------------------------------------------------------------

func TestCommsChannelAdapter_Name(t *testing.T) {
	inner := &mockCommsChannel{}
	a := NewCommsChannelAdapter(inner)
	if got := a.Name(); got != "slack" {
		t.Errorf("Name() = %q, want %q", got, "slack")
	}
}

func TestCommsChannelAdapter_SupportedTypes(t *testing.T) {
	inner := &mockCommsChannel{}
	a := NewCommsChannelAdapter(inner)
	types := a.SupportedTypes()
	if len(types) != 1 || types[0] != MessageTypeComms {
		t.Errorf("SupportedTypes() = %v, want [%s]", types, MessageTypeComms)
	}
}

func TestCommsChannelAdapter_Send_Success(t *testing.T) {
	inner := &mockCommsChannel{
		sendResult: CommsDeliveryResult{
			MessageID:   "msg-1",
			ChannelType: ChannelSlack,
			State:       CommsDeliverySuccess,
		},
	}
	a := NewCommsChannelAdapter(inner)

	msg := Message{
		ID:       "msg-1",
		Type:     MessageTypeComms,
		TenantID: "tenant-1",
		To:       []Recipient{{Address: "#general"}},
		Body:     "hello slack",
	}
	result, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !inner.sendCalled {
		t.Fatal("inner.Send was not called")
	}
	if result.State != DeliveryStateDelivered {
		t.Errorf("State = %q, want %q", result.State, DeliveryStateDelivered)
	}
	if result.MessageID != "msg-1" {
		t.Errorf("MessageID = %q, want %q", result.MessageID, "msg-1")
	}
	// Verify the comms message was built correctly
	if inner.lastMsg.ID != "msg-1" {
		t.Errorf("inner msg ID = %q, want %q", inner.lastMsg.ID, "msg-1")
	}
	if inner.lastMsg.Body != "hello slack" {
		t.Errorf("inner msg Body = %q, want %q", inner.lastMsg.Body, "hello slack")
	}
}

func TestCommsChannelAdapter_Send_Failure(t *testing.T) {
	inner := &mockCommsChannel{
		sendResult: CommsDeliveryResult{
			MessageID:    "msg-2",
			ChannelType:  ChannelSlack,
			State:        CommsDeliveryFailure,
			ErrorMessage: "webhook returned 500",
		},
	}
	a := NewCommsChannelAdapter(inner)

	msg := Message{
		ID:       "msg-2",
		Type:     MessageTypeComms,
		TenantID: "tenant-1",
		To:       []Recipient{{Address: "#alerts"}},
		Body:     "test",
	}
	result, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.State != DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, DeliveryStateFailed)
	}
	if result.ErrorMessage != "webhook returned 500" {
		t.Errorf("ErrorMessage = %q, want %q", result.ErrorMessage, "webhook returned 500")
	}
}

func TestCommsChannelAdapter_BatchSend(t *testing.T) {
	inner := &mockCommsChannel{
		batchResults: []CommsDeliveryResult{
			{MessageID: "msg-1", State: CommsDeliverySuccess},
			{MessageID: "msg-2", State: CommsDeliveryFailure, ErrorMessage: "timeout"},
		},
	}
	a := NewCommsChannelAdapter(inner)

	msg := Message{
		ID:       "batch-1",
		Type:     MessageTypeComms,
		TenantID: "tenant-1",
		To:       []Recipient{{Address: "#ch1"}, {Address: "#ch2"}},
		Body:     "batch test",
	}
	results, err := a.BatchSend(context.Background(), msg)
	if err != nil {
		t.Fatalf("BatchSend() error = %v", err)
	}
	if !inner.batchSendCalled {
		t.Fatal("inner.BatchSend was not called")
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].State != DeliveryStateDelivered {
		t.Errorf("results[0].State = %q, want %q", results[0].State, DeliveryStateDelivered)
	}
	if results[1].State != DeliveryStateFailed {
		t.Errorf("results[1].State = %q, want %q", results[1].State, DeliveryStateFailed)
	}
}

func TestCommsChannelAdapter_CheckHealth(t *testing.T) {
	inner := &mockCommsChannel{}
	a := NewCommsChannelAdapter(inner)

	hs, err := a.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if !inner.healthCalled {
		t.Fatal("inner.CheckHealth was not called")
	}
	if !hs.Healthy {
		t.Error("expected Healthy = true")
	}
}

func TestCommsChannelAdapter_CheckHealth_Error(t *testing.T) {
	inner := &mockCommsChannel{healthErr: context.DeadlineExceeded}
	a := NewCommsChannelAdapter(inner)

	hs, err := a.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if hs.Healthy {
		t.Error("expected Healthy = false")
	}
	if hs.Message != "context deadline exceeded" {
		t.Errorf("Message = %q, want %q", hs.Message, "context deadline exceeded")
	}
}

// -----------------------------------------------------------------------
// PushChannelAdapter tests
// -----------------------------------------------------------------------

func TestPushChannelAdapter_Name(t *testing.T) {
	inner := &mockPushChannel{}
	a := NewPushChannelAdapter(inner, "fcm")
	if got := a.Name(); got != "fcm" {
		t.Errorf("Name() = %q, want %q", got, "fcm")
	}
}

func TestPushChannelAdapter_SupportedTypes(t *testing.T) {
	inner := &mockPushChannel{}
	a := NewPushChannelAdapter(inner, "fcm")
	types := a.SupportedTypes()
	if len(types) != 1 || types[0] != MessageTypePush {
		t.Errorf("SupportedTypes() = %v, want [%s]", types, MessageTypePush)
	}
}

func TestPushChannelAdapter_Send_Delivered(t *testing.T) {
	inner := &mockPushChannel{
		sendResult: PushDeliveryResult{
			Target:     "token-abc",
			State:      PushStateDelivered,
			PlatformID: "fcm-123",
		},
	}
	a := NewPushChannelAdapter(inner, "fcm")

	msg := Message{
		ID:       "push-1",
		Type:     MessageTypePush,
		TenantID: "tenant-1",
		To:       []Recipient{{Address: "token-abc"}},
		Subject:  "Alert",
		Body:     "Camera offline",
	}
	result, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !inner.sendCalled {
		t.Fatal("inner.Send was not called")
	}
	if result.State != DeliveryStateDelivered {
		t.Errorf("State = %q, want %q", result.State, DeliveryStateDelivered)
	}
	if result.ProviderMessageID != "fcm-123" {
		t.Errorf("ProviderMessageID = %q, want %q", result.ProviderMessageID, "fcm-123")
	}
	if result.Recipient != "token-abc" {
		t.Errorf("Recipient = %q, want %q", result.Recipient, "token-abc")
	}
	// Verify push message mapping
	if inner.lastMsg.MessageID != "push-1" {
		t.Errorf("inner msg MessageID = %q, want %q", inner.lastMsg.MessageID, "push-1")
	}
	if inner.lastMsg.Title != "Alert" {
		t.Errorf("inner msg Title = %q, want %q", inner.lastMsg.Title, "Alert")
	}
	if inner.lastMsg.Body != "Camera offline" {
		t.Errorf("inner msg Body = %q, want %q", inner.lastMsg.Body, "Camera offline")
	}
}

func TestPushChannelAdapter_Send_Failed(t *testing.T) {
	inner := &mockPushChannel{
		sendResult: PushDeliveryResult{
			Target:       "token-xyz",
			State:        PushStateFailed,
			ErrorMessage: "invalid token",
		},
	}
	a := NewPushChannelAdapter(inner, "apns")

	msg := Message{
		ID:       "push-2",
		Type:     MessageTypePush,
		TenantID: "tenant-1",
		To:       []Recipient{{Address: "token-xyz"}},
		Body:     "test",
	}
	result, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.State != DeliveryStateFailed {
		t.Errorf("State = %q, want %q", result.State, DeliveryStateFailed)
	}
	if result.ErrorMessage != "invalid token" {
		t.Errorf("ErrorMessage = %q, want %q", result.ErrorMessage, "invalid token")
	}
}

func TestPushChannelAdapter_BatchSend(t *testing.T) {
	inner := &mockPushChannel{
		batchResults: []PushDeliveryResult{
			{Target: "token-1", State: PushStateDelivered, PlatformID: "id-1"},
			{Target: "token-2", State: PushStateThrottled, ErrorCode: "429"},
			{Target: "token-3", State: PushStateUnreachable, ShouldRemoveToken: true},
		},
	}
	a := NewPushChannelAdapter(inner, "fcm")

	msg := Message{
		ID:       "batch-push-1",
		Type:     MessageTypePush,
		TenantID: "tenant-1",
		To: []Recipient{
			{Address: "token-1"},
			{Address: "token-2"},
			{Address: "token-3"},
		},
		Body: "batch test",
	}
	results, err := a.BatchSend(context.Background(), msg)
	if err != nil {
		t.Fatalf("BatchSend() error = %v", err)
	}
	if !inner.batchSendCalled {
		t.Fatal("inner.BatchSend was not called")
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[0].State != DeliveryStateDelivered {
		t.Errorf("results[0].State = %q, want %q", results[0].State, DeliveryStateDelivered)
	}
	// Throttled maps to Failed with error info preserved
	if results[1].State != DeliveryStateFailed {
		t.Errorf("results[1].State = %q, want %q", results[1].State, DeliveryStateFailed)
	}
	// Unreachable maps to Suppressed
	if results[2].State != DeliveryStateSuppressed {
		t.Errorf("results[2].State = %q, want %q", results[2].State, DeliveryStateSuppressed)
	}
}

func TestPushChannelAdapter_CheckHealth(t *testing.T) {
	inner := &mockPushChannel{}
	a := NewPushChannelAdapter(inner, "webpush")

	hs, err := a.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if !inner.healthCalled {
		t.Fatal("inner.CheckHealth was not called")
	}
	if !hs.Healthy {
		t.Error("expected Healthy = true")
	}
}

func TestPushChannelAdapter_CheckHealth_Error(t *testing.T) {
	inner := &mockPushChannel{healthErr: context.DeadlineExceeded}
	a := NewPushChannelAdapter(inner, "fcm")

	hs, err := a.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if hs.Healthy {
		t.Error("expected Healthy = false")
	}
}

// -----------------------------------------------------------------------
// State mapping tests
// -----------------------------------------------------------------------

func TestMapCommsState(t *testing.T) {
	tests := []struct {
		in  CommsDeliveryState
		out DeliveryState
	}{
		{CommsDeliverySuccess, DeliveryStateDelivered},
		{CommsDeliveryFailure, DeliveryStateFailed},
		{CommsDeliveryState("unknown"), DeliveryStateFailed},
	}
	for _, tt := range tests {
		got := mapCommsState(tt.in)
		if got != tt.out {
			t.Errorf("mapCommsState(%q) = %q, want %q", tt.in, got, tt.out)
		}
	}
}

func TestMapPushState(t *testing.T) {
	tests := []struct {
		in  PushDeliveryState
		out DeliveryState
	}{
		{PushStateDelivered, DeliveryStateDelivered},
		{PushStateFailed, DeliveryStateFailed},
		{PushStateThrottled, DeliveryStateFailed},
		{PushStateUnreachable, DeliveryStateSuppressed},
		{PushDeliveryState("unknown"), DeliveryStateFailed},
	}
	for _, tt := range tests {
		got := mapPushState(tt.in)
		if got != tt.out {
			t.Errorf("mapPushState(%q) = %q, want %q", tt.in, got, tt.out)
		}
	}
}

// -----------------------------------------------------------------------
// Registry integration — adapters can be registered in ChannelRegistry
// -----------------------------------------------------------------------

func TestAdapters_RegisterInChannelRegistry(t *testing.T) {
	reg := NewChannelRegistry()

	commsInner := &mockCommsChannel{}
	pushInner := &mockPushChannel{}

	reg.Register(NewCommsChannelAdapter(commsInner))
	reg.Register(NewPushChannelAdapter(pushInner, "fcm"))

	// Verify they can be found
	if ch := reg.Get("slack"); ch == nil {
		t.Error("expected to find slack adapter in registry")
	}
	if ch := reg.Get("fcm"); ch == nil {
		t.Error("expected to find fcm adapter in registry")
	}

	// ForType should find comms channels
	commsChannels := reg.ForType(MessageTypeComms)
	if len(commsChannels) != 1 {
		t.Errorf("ForType(comms) returned %d channels, want 1", len(commsChannels))
	}

	pushChannels := reg.ForType(MessageTypePush)
	if len(pushChannels) != 1 {
		t.Errorf("ForType(push) returned %d channels, want 1", len(pushChannels))
	}

	_ = time.Now() // avoid unused import
}
