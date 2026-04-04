package onvif

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestParseEventSearchResults(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tse="http://www.onvif.org/ver10/search/wsdl">
  <s:Body>
    <tse:GetEventSearchResultsResponse>
      <tse:ResultList>
        <tse:SearchState>Completed</tse:SearchState>
        <tse:Result>
          <tse:RecordingToken>REC001</tse:RecordingToken>
          <tse:TrackToken>VIDEO001</tse:TrackToken>
          <tse:Time>2025-06-15T10:30:00Z</tse:Time>
          <tse:Event>
            <wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
                        xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:VideoSource/MotionAlarm</wsnt:Topic>
            <wsnt:Message xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
              <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema" PropertyOperation="Changed">
                <tt:Data>
                  <tt:SimpleItem Name="State" Value="true"/>
                </tt:Data>
              </tt:Message>
            </wsnt:Message>
          </tse:Event>
        </tse:Result>
        <tse:Result>
          <tse:RecordingToken>REC001</tse:RecordingToken>
          <tse:TrackToken>VIDEO001</tse:TrackToken>
          <tse:Time>2025-06-15T10:35:00Z</tse:Time>
          <tse:Event>
            <wsnt:Topic xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
                        xmlns:tns1="http://www.onvif.org/ver10/topics">tns1:VideoSource/MotionAlarm</wsnt:Topic>
            <wsnt:Message xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
              <tt:Message xmlns:tt="http://www.onvif.org/ver10/schema" PropertyOperation="Changed">
                <tt:Data>
                  <tt:SimpleItem Name="State" Value="false"/>
                </tt:Data>
              </tt:Message>
            </wsnt:Message>
          </tse:Event>
        </tse:Result>
      </tse:ResultList>
    </tse:GetEventSearchResultsResponse>
  </s:Body>
</s:Envelope>`

	var env eventSearchEnvelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}

	if env.Body.GetEventSearchResultsResponse == nil {
		t.Fatal("expected GetEventSearchResultsResponse to be non-nil")
	}

	rl := env.Body.GetEventSearchResultsResponse.ResultList
	if rl.SearchState != "Completed" {
		t.Errorf("expected SearchState=Completed, got %q", rl.SearchState)
	}
	if len(rl.Result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(rl.Result))
	}

	r0 := rl.Result[0]
	if r0.RecordingToken != "REC001" {
		t.Errorf("result[0] RecordingToken: expected REC001, got %q", r0.RecordingToken)
	}
	if r0.TrackToken != "VIDEO001" {
		t.Errorf("result[0] TrackToken: expected VIDEO001, got %q", r0.TrackToken)
	}
	if r0.Time != "2025-06-15T10:30:00Z" {
		t.Errorf("result[0] Time: expected 2025-06-15T10:30:00Z, got %q", r0.Time)
	}
	simpleItems := r0.Event.Message.Inner.Data.SimpleItems
	if len(simpleItems) != 1 {
		t.Fatalf("result[0] expected 1 simple item, got %d", len(simpleItems))
	}
	if simpleItems[0].Name != "State" || simpleItems[0].Value != "true" {
		t.Errorf("result[0] data: expected State=true, got %s=%s",
			simpleItems[0].Name, simpleItems[0].Value)
	}
	if r0.Event.Message.Inner.PropertyOperation != "Changed" {
		t.Errorf("result[0] expected PropertyOperation=Changed, got %q", r0.Event.Message.Inner.PropertyOperation)
	}
}

func TestParseFindEventsResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tse="http://www.onvif.org/ver10/search/wsdl">
  <s:Body>
    <tse:FindEventsResponse>
      <tse:SearchToken>search-token-abc-123</tse:SearchToken>
    </tse:FindEventsResponse>
  </s:Body>
</s:Envelope>`

	var env eventSearchEnvelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}

	if env.Body.FindEventsResponse == nil {
		t.Fatal("expected FindEventsResponse to be non-nil")
	}
	if env.Body.FindEventsResponse.SearchToken != "search-token-abc-123" {
		t.Errorf("expected search token 'search-token-abc-123', got %q", env.Body.FindEventsResponse.SearchToken)
	}
}

func TestBuildFindEventsBody_NoFilter(t *testing.T) {
	body := buildFindEventsBody(nil)

	if !strings.Contains(body, "<tse:StartPoint>2000-01-01T00:00:00Z</tse:StartPoint>") {
		t.Error("expected default start time in body")
	}
	if !strings.Contains(body, "<tse:EndPoint>9999-12-31T23:59:59Z</tse:EndPoint>") {
		t.Error("expected default end time in body")
	}
	if strings.Contains(body, "<tse:IncludedRecordings>") {
		t.Error("should not contain IncludedRecordings when no filter")
	}
	if strings.Contains(body, "<tse:TopicExpression") {
		t.Error("should not contain TopicExpression when no filter")
	}
}

func TestBuildFindEventsBody_WithFilter(t *testing.T) {
	start := time.Date(2025, 1, 15, 8, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 18, 0, 0, 0, time.UTC)
	filter := &EdgeSearchFilter{
		StartTime:      &start,
		EndTime:        &end,
		RecordingToken: "REC-TOKEN-1",
		EventType:      "tns1:VideoSource/MotionAlarm",
	}

	body := buildFindEventsBody(filter)

	if !strings.Contains(body, "<tse:StartPoint>2025-01-15T08:00:00Z</tse:StartPoint>") {
		t.Errorf("expected start time in body, got:\n%s", body)
	}
	if !strings.Contains(body, "<tse:EndPoint>2025-01-15T18:00:00Z</tse:EndPoint>") {
		t.Errorf("expected end time in body, got:\n%s", body)
	}
	if !strings.Contains(body, "<tse:IncludedRecordings>REC-TOKEN-1</tse:IncludedRecordings>") {
		t.Errorf("expected recording token in body, got:\n%s", body)
	}
	if !strings.Contains(body, "tns1:VideoSource/MotionAlarm") {
		t.Errorf("expected event type in body, got:\n%s", body)
	}
}

func TestBuildFindRecordingsBody_NoFilter(t *testing.T) {
	body := buildFindRecordingsBody(nil)

	if !strings.Contains(body, "<tse:IncludedSources/>") {
		t.Error("expected IncludedSources in body when no filter")
	}
}

func TestBuildFindRecordingsBody_WithFilter(t *testing.T) {
	start := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	filter := &EdgeSearchFilter{
		StartTime:      &start,
		RecordingToken: "REC-42",
	}

	body := buildFindRecordingsBody(filter)

	if !strings.Contains(body, "<tse:IncludedRecordings>REC-42</tse:IncludedRecordings>") {
		t.Errorf("expected recording token in body, got:\n%s", body)
	}
	if strings.Contains(body, "<tse:IncludedSources/>") {
		t.Error("should not contain IncludedSources when recording token is set")
	}
	if !strings.Contains(body, "RecordingInformationFilter") {
		t.Errorf("expected RecordingInformationFilter with start time, got:\n%s", body)
	}
}

func TestParseEventSearchFault(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <s:Fault>
      <faultstring>Receiver not found</faultstring>
    </s:Fault>
  </s:Body>
</s:Envelope>`

	var env eventSearchEnvelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}

	if env.Body.Fault == nil {
		t.Fatal("expected Fault to be non-nil")
	}
	if env.Body.Fault.Faultstring != "Receiver not found" {
		t.Errorf("expected fault string 'Receiver not found', got %q", env.Body.Fault.Faultstring)
	}
}
