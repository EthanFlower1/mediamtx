package dmp

import (
	"testing"
)

func TestParseSIAMessage(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantAcct  string
		wantSeq   string
		wantData  string
		wantErr   bool
	}{
		{
			name:     "standard SIA message with burglary alarm",
			raw:      `0001004C"SIA"0001#1234[Nri01/EBA001]`,
			wantAcct: "1234",
			wantSeq:  "0001",
			wantData: "Nri01/EBA001",
		},
		{
			name:     "SIA message with fire alarm",
			raw:      `A1B20052"*SIA"0042#5678[Nri02/EFA003]_14:30:00,04-10-2026`,
			wantAcct: "5678",
			wantSeq:  "0042",
			wantData: "Nri02/EFA003",
		},
		{
			name:     "SIA message with restore",
			raw:      `00000040"SIA"0003#9999[Nri01/RBR005]`,
			wantAcct: "9999",
			wantSeq:  "0003",
			wantData: "Nri01/RBR005",
		},
		{
			name:     "SIA message with opening report",
			raw:      `00000038"SIA"0010#4444[EOP000]`,
			wantAcct: "4444",
			wantSeq:  "0010",
			wantData: "EOP000",
		},
		{
			name:    "too short",
			raw:     `short`,
			wantErr: true,
		},
		{
			name:    "missing protocol identifier",
			raw:     `00000040XSIA0001#1234[data]`,
			wantErr: true,
		},
		{
			name:    "missing account separator",
			raw:     `00000040"SIA"00011234[data]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseSIAMessage(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if msg.AccountID != tt.wantAcct {
				t.Errorf("AccountID = %q, want %q", msg.AccountID, tt.wantAcct)
			}
			if msg.Sequence != tt.wantSeq {
				t.Errorf("Sequence = %q, want %q", msg.Sequence, tt.wantSeq)
			}
			if msg.Data != tt.wantData {
				t.Errorf("Data = %q, want %q", msg.Data, tt.wantData)
			}
		})
	}
}

func TestParseAlarmEvent(t *testing.T) {
	tests := []struct {
		name          string
		data          string
		wantCode      string
		wantQualifier string
		wantZone      int
		wantArea      int
		wantSeverity  string
		wantErr       bool
	}{
		{
			name:          "burglary alarm zone 1 area 1",
			data:          "Nri01/EBA001",
			wantCode:      CodeBurglaryAlarm,
			wantQualifier: "E",
			wantZone:      1,
			wantArea:      1,
			wantSeverity:  SeverityWarning,
		},
		{
			name:          "fire alarm zone 3 area 2",
			data:          "Nri02/EFA003",
			wantCode:      CodeFireAlarm,
			wantQualifier: "E",
			wantZone:      3,
			wantArea:      2,
			wantSeverity:  SeverityCritical,
		},
		{
			name:          "burglary restore zone 5 area 1",
			data:          "Nri01/RBR005",
			wantCode:      CodeBurglaryRestore,
			wantQualifier: "R",
			wantZone:      5,
			wantArea:      1,
			wantSeverity:  SeverityInfo,
		},
		{
			name:          "panic alarm zone 10",
			data:          "Nri03/EPA010",
			wantCode:      CodePanicAlarm,
			wantQualifier: "E",
			wantZone:      10,
			wantArea:      3,
			wantSeverity:  SeverityCritical,
		},
		{
			name:          "opening report (no zone prefix)",
			data:          "EOP000",
			wantCode:      CodeOpeningReport,
			wantQualifier: "E",
			wantZone:      0,
			wantArea:      0,
			wantSeverity:  SeverityInfo,
		},
		{
			name:          "AC fail",
			data:          "Nri01/EAT000",
			wantCode:      CodeACFail,
			wantQualifier: "E",
			wantZone:      0,
			wantArea:      1,
			wantSeverity:  SeverityWarning,
		},
		{
			name:    "empty data",
			data:    "",
			wantErr: true,
		},
		{
			name:    "too short",
			data:    "E",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &SIAMessage{
				AccountID: "1234",
				Data:      tt.data,
			}
			event, err := ParseAlarmEvent(msg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if event.EventCode != tt.wantCode {
				t.Errorf("EventCode = %q, want %q", event.EventCode, tt.wantCode)
			}
			if event.EventQualifier != tt.wantQualifier {
				t.Errorf("EventQualifier = %q, want %q", event.EventQualifier, tt.wantQualifier)
			}
			if event.Zone != tt.wantZone {
				t.Errorf("Zone = %d, want %d", event.Zone, tt.wantZone)
			}
			if event.Area != tt.wantArea {
				t.Errorf("Area = %d, want %d", event.Area, tt.wantArea)
			}
			if event.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q", event.Severity, tt.wantSeverity)
			}
			if event.Description == "" {
				t.Error("Description should not be empty")
			}
		})
	}
}

func TestSIAAck(t *testing.T) {
	ack := SIAAck("0001", "1234")
	s := string(ack)

	if len(s) == 0 {
		t.Fatal("ACK should not be empty")
	}

	// Should contain ACK protocol, sequence, and account.
	if s[0] != '\n' {
		t.Errorf("ACK should start with LF, got %q", s[0])
	}
	if s[len(s)-1] != '\r' {
		t.Errorf("ACK should end with CR, got %q", s[len(s)-1])
	}

	// Check content includes expected fields.
	want := "\"ACK\"0001#1234[]"
	if !contains(s, want) {
		t.Errorf("ACK %q should contain %q", s, want)
	}
}

func TestClassifyEventSeverity(t *testing.T) {
	tests := []struct {
		code      string
		qualifier string
		want      string
	}{
		{CodeFireAlarm, "E", SeverityCritical},
		{CodePanicAlarm, "E", SeverityCritical},
		{CodeBurglaryAlarm, "E", SeverityWarning},
		{CodeBurglaryAlarm, "R", SeverityInfo}, // restores are always info
		{CodeOpeningReport, "E", SeverityInfo},
		{CodeACFail, "E", SeverityWarning},
		{CodeTestReport, "E", SeverityInfo},
	}

	for _, tt := range tests {
		got := classifyEventSeverity(tt.code, tt.qualifier)
		if got != tt.want {
			t.Errorf("classifyEventSeverity(%q, %q) = %q, want %q",
				tt.code, tt.qualifier, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
