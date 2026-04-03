package onvif

import "testing"

func TestClassifyTopic(t *testing.T) {
	tests := []struct {
		topic    string
		wantType DetectedEventType
		wantOK   bool
	}{
		// Existing: motion
		{"tns1:RuleEngine/CellMotionDetector/Motion", EventMotion, true},
		{"tns1:VideoAnalytics/Motion", EventMotion, true},
		// Existing: tampering
		{"tns1:RuleEngine/TamperDetector/Tamper", EventTampering, true},
		{"tns1:VideoSource/GlobalSceneChange/ImagingService", EventTampering, true},
		// New: digital input
		{"tns1:Device/Trigger/DigitalInput", EventDigitalInput, true},
		{"tns1:Device/IO/Digital_Input", EventDigitalInput, true},
		{"tns1:Device/Trigger/LogicalState", EventDigitalInput, true},
		// New: signal loss
		{"tns1:VideoSource/SignalLoss", EventSignalLoss, true},
		{"tns1:MediaControl/VideoLoss", EventSignalLoss, true},
		// New: hardware failure
		{"tns1:Device/HardwareFailure/StorageFailure", EventHardwareFailure, true},
		{"tns1:Monitoring/ProcessorUsage", EventHardwareFailure, true},
		// New: relay
		{"tns1:Device/Trigger/Relay", EventRelay, true},
		{"tns1:Device/IO/RelayOutput", EventRelay, true},
		{"tns1:Device/Trigger/DigitalOutput", EventRelay, true},
		// Unknown
		{"tns1:SomeUnknown/Topic", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			gotType, gotOK := classifyTopic(tt.topic)
			if gotOK != tt.wantOK {
				t.Errorf("classifyTopic(%q) ok = %v, want %v", tt.topic, gotOK, tt.wantOK)
			}
			if gotType != tt.wantType {
				t.Errorf("classifyTopic(%q) type = %q, want %q", tt.topic, gotType, tt.wantType)
			}
		})
	}
}

func TestParseEvents_AllTypes(t *testing.T) {
	// SOAP Notify body with one message per event type.
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body>
    <wsnt:Notify>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="IsMotion" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:Device/Trigger/DigitalInput</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:VideoSource/SignalLoss</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="false"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:Device/HardwareFailure/StorageFailure</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:Device/Trigger/Relay</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`)

	events, err := parseEvents(body)
	if err != nil {
		t.Fatalf("parseEvents error: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("got %d events, want 5", len(events))
	}

	expected := []DetectedEvent{
		{Type: EventMotion, Active: true},
		{Type: EventDigitalInput, Active: true},
		{Type: EventSignalLoss, Active: false},
		{Type: EventHardwareFailure, Active: true},
		{Type: EventRelay, Active: true},
	}
	for i, want := range expected {
		if events[i].Type != want.Type {
			t.Errorf("event[%d] type = %q, want %q", i, events[i].Type, want.Type)
		}
		if events[i].Active != want.Active {
			t.Errorf("event[%d] active = %v, want %v", i, events[i].Active, want.Active)
		}
	}
}

func TestParseEventProperties(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tev:GetEventPropertiesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
      <tev:TopicNamespaceLocation>http://www.onvif.org/onvif/ver10/topics/topicns.xml</tev:TopicNamespaceLocation>
      <tev:TopicSet>
        <tns1:RuleEngine xmlns:tns1="http://www.onvif.org/ver10/topics">
          <CellMotionDetector>
            <Motion wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
          </CellMotionDetector>
        </tns1:RuleEngine>
        <tns1:VideoSource xmlns:tns1="http://www.onvif.org/ver10/topics">
          <GlobalSceneChange>
            <ImagingService wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
          </GlobalSceneChange>
          <SignalLoss wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
        </tns1:VideoSource>
        <tns1:Device xmlns:tns1="http://www.onvif.org/ver10/topics">
          <Trigger>
            <DigitalInput wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
            <Relay wstop:topic="true" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1"/>
          </Trigger>
        </tns1:Device>
      </tev:TopicSet>
    </tev:GetEventPropertiesResponse>
  </s:Body>
</s:Envelope>`)

	topics, err := parseEventProperties(body)
	if err != nil {
		t.Fatalf("parseEventProperties error: %v", err)
	}

	want := map[DetectedEventType]bool{
		EventMotion:       true,
		EventTampering:    true,
		EventSignalLoss:   true,
		EventDigitalInput: true,
		EventRelay:        true,
	}
	got := make(map[DetectedEventType]bool)
	for _, tp := range topics {
		got[tp] = true
	}
	for wantType := range want {
		if !got[wantType] {
			t.Errorf("missing expected topic: %s", wantType)
		}
	}
	if len(topics) != len(got) {
		t.Errorf("got %d topics but %d unique — duplicates present", len(topics), len(got))
	}
}

func TestParseEventProperties_Empty(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tev:GetEventPropertiesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
      <tev:TopicNamespaceLocation>http://www.onvif.org/onvif/ver10/topics/topicns.xml</tev:TopicNamespaceLocation>
      <tev:TopicSet/>
    </tev:GetEventPropertiesResponse>
  </s:Body>
</s:Envelope>`)

	topics, err := parseEventProperties(body)
	if err != nil {
		t.Fatalf("parseEventProperties error: %v", err)
	}
	if len(topics) != 0 {
		t.Errorf("expected 0 topics, got %d", len(topics))
	}
}
