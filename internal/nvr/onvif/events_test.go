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
