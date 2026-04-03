package onvif

import "testing"

func TestParseEventsLineCrossing(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <wsnt:Notify xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/LineCrossingDetector/Crossed</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="State" Value="true"/>
              <tt:SimpleItem Name="Direction" Value="LeftToRight"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`

	events, err := parseEvents([]byte(xml))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventLineCrossing {
		t.Errorf("expected type %q, got %q", EventLineCrossing, events[0].Type)
	}
	if !events[0].Active {
		t.Error("expected active=true")
	}
	if events[0].Metadata == nil || events[0].Metadata["direction"] != "LeftToRight" {
		t.Errorf("expected metadata direction=LeftToRight, got %v", events[0].Metadata)
	}
}

func TestParseEventsObjectCount(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <wsnt:Notify xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/ObjectCounting/Count</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="Count" Value="5"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`

	events, err := parseEvents([]byte(xml))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventObjectCount {
		t.Errorf("expected type %q, got %q", EventObjectCount, events[0].Type)
	}
	if !events[0].Active {
		t.Error("expected active=true for count > 0")
	}
	if events[0].Metadata == nil || events[0].Metadata["count"] != "5" {
		t.Errorf("expected metadata count=5, got %v", events[0].Metadata)
	}
}

func TestParseEventsObjectCountZero(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <wsnt:Notify xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/ObjectCounting/Count</wsnt:Topic>
        <wsnt:Message>
          <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema">
            <tt:Data>
              <tt:SimpleItem Name="Count" Value="0"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`

	events, err := parseEvents([]byte(xml))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Active {
		t.Error("expected active=false for count == 0")
	}
}

func TestParseEventsIntrusion(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <wsnt:Notify xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
      <wsnt:NotificationMessage>
        <wsnt:Topic>tns1:RuleEngine/FieldDetector/ObjectsInside</wsnt:Topic>
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
</s:Envelope>`

	events, err := parseEvents([]byte(xml))
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventIntrusion {
		t.Errorf("expected type %q, got %q", EventIntrusion, events[0].Type)
	}
	if !events[0].Active {
		t.Error("expected active=true")
	}
}

func TestClassifyTopic(t *testing.T) {
	tests := []struct {
		topic    string
		wantType DetectedEventType
		wantOK   bool
	}{
		// Existing: motion
		{"tns1:RuleEngine/CellMotionDetector/Motion", EventMotion, true},
		{"tns1:VideoSource/MotionAlarm", EventMotion, true},
		// Existing: tampering
		{"tns1:VideoSource/GlobalSceneChange/ImagingService", EventTampering, true},
		{"tns1:VideoSource/Tamper", EventTampering, true},
		// New: line crossing
		{"tns1:RuleEngine/LineCrossingDetector/Crossed", EventLineCrossing, true},
		{"tns1:RuleEngine/LineCounter/Crossed", EventLineCrossing, true},
		// New: intrusion / field detection
		{"tns1:RuleEngine/FieldDetector/ObjectsInside", EventIntrusion, true},
		{"tns1:RuleEngine/IntrusionDetection/Alert", EventIntrusion, true},
		{"tns1:RuleEngine/FieldDetection/ObjectsInside", EventIntrusion, true},
		// New: loitering
		{"tns1:RuleEngine/LoiteringDetector/Alert", EventLoitering, true},
		// New: object counting
		{"tns1:RuleEngine/ObjectCounting/Count", EventObjectCount, true},
		{"tns1:RuleEngine/CountAggregation/Counting", EventObjectCount, true},
		// Unknown topic
		{"tns1:Device/HardwareFailure", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		gotType, gotOK := classifyTopic(tt.topic)
		if gotType != tt.wantType || gotOK != tt.wantOK {
			t.Errorf("classifyTopic(%q) = (%q, %v), want (%q, %v)",
				tt.topic, gotType, gotOK, tt.wantType, tt.wantOK)
		}
	}
}
