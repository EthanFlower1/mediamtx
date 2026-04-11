// Package dmp implements a first-party integration with DMP XR-Series alarm
// panels. It receives alarm events via the SIA DC-03-1990.01 (level 2) protocol
// over TCP, maps zones/areas to NVR cameras, and injects alarm events into the
// video timeline so operators can correlate alarms with footage.
//
// References:
//   - SIA DC-03-1990.01 (Ademco Contact ID)
//   - DMP XR-Series Programming Guide, Network Communications chapter
package dmp

import (
	"fmt"
	"strings"
	"time"
)

// SIA event code categories used by DMP XR-Series panels.
const (
	// Burglary / intrusion codes.
	CodeBurglaryAlarm   = "BA"
	CodeBurglaryRestore = "BR"
	CodeEntryExit       = "EE"

	// Fire codes.
	CodeFireAlarm   = "FA"
	CodeFireRestore = "FR"

	// Panic / duress codes.
	CodePanicAlarm   = "PA"
	CodePanicRestore = "PR"

	// Supervisory codes.
	CodeACFail    = "AT"
	CodeACRestore = "AR"
	CodeBattLow   = "YT"
	CodeBattOK    = "YR"

	// Arming / disarming codes.
	CodeOpeningReport = "OP"
	CodeClosingReport = "CL"

	// Trouble codes.
	CodeZoneTrouble  = "TA"
	CodeZoneRestore  = "TR"
	CodeCommTrouble  = "YC"

	// Test codes.
	CodeTestReport = "RP"
)

// Severity levels for alarm events.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// AlarmEvent represents a parsed SIA alarm event from a DMP XR-Series panel.
type AlarmEvent struct {
	// Raw is the original SIA message line.
	Raw string `json:"raw"`

	// AccountID is the panel account number (typically 4-6 digits).
	AccountID string `json:"account_id"`

	// EventCode is the 2-character SIA event code (e.g., "BA" for burglary alarm).
	EventCode string `json:"event_code"`

	// EventQualifier is "E" for event/alarm or "R" for restore.
	EventQualifier string `json:"event_qualifier"`

	// Zone is the zone number that triggered the event (0 = panel-wide).
	Zone int `json:"zone"`

	// Area is the area/partition number.
	Area int `json:"area"`

	// Timestamp is when the event was received.
	Timestamp time.Time `json:"timestamp"`

	// Description is a human-readable description of the event.
	Description string `json:"description"`

	// Severity is the alarm severity: critical, warning, or info.
	Severity string `json:"severity"`
}

// SIAMessage represents a raw SIA protocol frame received over TCP.
// Format: LF <crc> <len> "<SIA>"<seq>#<acct>[<data>]<timestamp> CR
type SIAMessage struct {
	CRC       string
	Length    int
	Sequence  string
	AccountID string
	Data      string
	Timestamp string
	Raw       string
}

// ParseSIAMessage parses a SIA DC-03 level 2 message.
// The general format is:
//
//	\n<CRC><0LLL>"<PROTO>"<SEQ>#<ACCT>[<DATA>]<TS>\r
//
// where CRC is 4 hex chars, LLL is 3-digit length, PROTO is "SIA" or "*SIA",
// SEQ is a sequence number, ACCT is the account code, DATA is the event payload,
// and TS is an optional timestamp.
func ParseSIAMessage(raw string) (*SIAMessage, error) {
	msg := &SIAMessage{Raw: raw}

	// Strip leading LF and trailing CR.
	s := strings.TrimLeft(raw, "\n\x0a")
	s = strings.TrimRight(s, "\r\x0d")
	s = strings.TrimSpace(s)

	if len(s) < 12 {
		return nil, fmt.Errorf("message too short: %d bytes", len(s))
	}

	// CRC: first 4 characters.
	msg.CRC = s[:4]
	s = s[4:]

	// Length: next 4 characters (0LLL format).
	if len(s) < 4 {
		return nil, fmt.Errorf("missing length field")
	}
	lenStr := s[:4]
	var msgLen int
	if _, err := fmt.Sscanf(lenStr, "%04d", &msgLen); err != nil {
		return nil, fmt.Errorf("invalid length: %q", lenStr)
	}
	msg.Length = msgLen
	s = s[4:]

	// Protocol identifier in quotes: "SIA" or "*SIA".
	protoStart := strings.Index(s, "\"")
	if protoStart < 0 {
		return nil, fmt.Errorf("missing protocol identifier")
	}
	protoEnd := strings.Index(s[protoStart+1:], "\"")
	if protoEnd < 0 {
		return nil, fmt.Errorf("unterminated protocol identifier")
	}
	// Skip past the closing quote.
	s = s[protoStart+protoEnd+2:]

	// Sequence number: digits up to '#'.
	hashIdx := strings.Index(s, "#")
	if hashIdx < 0 {
		return nil, fmt.Errorf("missing account separator '#'")
	}
	msg.Sequence = s[:hashIdx]
	s = s[hashIdx+1:]

	// Account ID: up to '['.
	bracketIdx := strings.Index(s, "[")
	if bracketIdx < 0 {
		// No data section — rest is account + optional timestamp.
		msg.AccountID = s
		return msg, nil
	}
	msg.AccountID = s[:bracketIdx]
	s = s[bracketIdx:]

	// Data section: between '[' and ']'.
	closeIdx := strings.Index(s, "]")
	if closeIdx < 0 {
		return nil, fmt.Errorf("unterminated data section")
	}
	msg.Data = s[1:closeIdx]

	// Optional timestamp after ']'.
	if closeIdx+1 < len(s) {
		msg.Timestamp = strings.TrimSpace(s[closeIdx+1:])
	}

	return msg, nil
}

// ParseAlarmEvent extracts an AlarmEvent from a parsed SIA message.
// The data field format for DMP XR-Series is typically:
//
//	Nri<area>/<event_qualifier><event_code><zone>
//
// For example: "Nri01/EA001" means area 1, event, burglary alarm, zone 1.
func ParseAlarmEvent(msg *SIAMessage) (*AlarmEvent, error) {
	event := &AlarmEvent{
		Raw:       msg.Raw,
		AccountID: msg.AccountID,
		Timestamp: time.Now().UTC(),
	}

	data := msg.Data
	if data == "" {
		return nil, fmt.Errorf("empty data section")
	}

	// Handle the Nri<area>/<qualifier><code><zone> format.
	// Also handle simpler formats: <qualifier><code><zone>
	if strings.HasPrefix(data, "Nri") || strings.HasPrefix(data, "ri") {
		data = strings.TrimPrefix(data, "N")
		data = strings.TrimPrefix(data, "ri")

		// Parse area if present (before '/').
		slashIdx := strings.Index(data, "/")
		if slashIdx >= 0 {
			if _, err := fmt.Sscanf(data[:slashIdx], "%d", &event.Area); err != nil {
				event.Area = 0
			}
			data = data[slashIdx+1:]
		}
	}

	// Now parse: <qualifier><code><zone>
	// Qualifier: E (event/alarm) or R (restore), 1 char.
	// Code: 2 chars.
	// Zone: remaining digits.
	if len(data) < 3 {
		return nil, fmt.Errorf("data too short for event parsing: %q", data)
	}

	event.EventQualifier = string(data[0])
	event.EventCode = data[1:3]

	if len(data) > 3 {
		if _, err := fmt.Sscanf(data[3:], "%d", &event.Zone); err != nil {
			event.Zone = 0
		}
	}

	event.Description = describeEvent(event.EventCode, event.EventQualifier, event.Zone, event.Area)
	event.Severity = classifyEventSeverity(event.EventCode, event.EventQualifier)

	return event, nil
}

// describeEvent returns a human-readable description of the alarm event.
func describeEvent(code, qualifier string, zone, area int) string {
	action := "alarm"
	if qualifier == "R" {
		action = "restore"
	}

	var desc string
	switch code {
	case CodeBurglaryAlarm, CodeBurglaryRestore:
		desc = "Burglary"
	case CodeEntryExit:
		desc = "Entry/exit"
	case CodeFireAlarm, CodeFireRestore:
		desc = "Fire"
	case CodePanicAlarm, CodePanicRestore:
		desc = "Panic/duress"
	case CodeACFail, CodeACRestore:
		desc = "AC power"
	case CodeBattLow, CodeBattOK:
		desc = "Battery"
	case CodeOpeningReport:
		desc = "System opened"
		action = ""
	case CodeClosingReport:
		desc = "System closed"
		action = ""
	case CodeZoneTrouble, CodeZoneRestore:
		desc = "Zone trouble"
	case CodeCommTrouble:
		desc = "Communication trouble"
	case CodeTestReport:
		desc = "Test report"
		action = ""
	default:
		desc = fmt.Sprintf("Event %s", code)
	}

	parts := []string{desc}
	if action != "" {
		parts = append(parts, action)
	}
	if zone > 0 {
		parts = append(parts, fmt.Sprintf("zone %d", zone))
	}
	if area > 0 {
		parts = append(parts, fmt.Sprintf("area %d", area))
	}

	return strings.Join(parts, " ")
}

// classifyEventSeverity determines the severity of an alarm event.
func classifyEventSeverity(code, qualifier string) string {
	// Restores are always informational.
	if qualifier == "R" {
		return SeverityInfo
	}

	switch code {
	case CodeFireAlarm, CodePanicAlarm:
		return SeverityCritical
	case CodeBurglaryAlarm, CodeEntryExit:
		return SeverityWarning
	case CodeACFail, CodeBattLow, CodeZoneTrouble, CodeCommTrouble:
		return SeverityWarning
	case CodeOpeningReport, CodeClosingReport, CodeTestReport:
		return SeverityInfo
	default:
		return SeverityWarning
	}
}

// SIAAck generates an acknowledgment response for the SIA receiver.
// DMP panels expect an ACK in the format: \n<CRC><0LLL>"ACK"<SEQ>#<ACCT>[]\r
func SIAAck(seq, acct string) []byte {
	body := fmt.Sprintf("\"ACK\"%s#%s[]", seq, acct)
	length := len(body)
	// Simple CRC placeholder — DMP panels typically accept 0000 for ACK.
	frame := fmt.Sprintf("\n0000%04d%s\r", length, body)
	return []byte(frame)
}
