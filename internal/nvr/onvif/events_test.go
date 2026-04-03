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
