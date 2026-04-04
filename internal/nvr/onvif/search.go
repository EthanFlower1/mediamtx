package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// EdgeEvent represents a single event stored on a camera's edge storage.
type EdgeEvent struct {
	RecordingToken string `json:"recording_token"`
	TrackToken     string `json:"track_token"`
	Time           string `json:"time"`
	Topic          string `json:"topic"`
	IsProperty     bool   `json:"is_property"`
	IsDataPresent  bool   `json:"is_data_present"`
	// Data holds the SimpleItem key-value pairs from the event.
	Data map[string]string `json:"data,omitempty"`
}

// EdgeSearchFilter specifies optional filters for edge recording and event searches.
type EdgeSearchFilter struct {
	StartTime      *time.Time // restrict to events/recordings after this time
	EndTime        *time.Time // restrict to events/recordings before this time
	RecordingToken string     // limit search to a specific recording
	EventType      string     // ONVIF topic expression filter (e.g. "tns1:VideoSource/MotionAlarm")
}

// --- SOAP response types for event search ---

type eventSearchEnvelope struct {
	XMLName xml.Name         `xml:"Envelope"`
	Body    eventSearchBody  `xml:"Body"`
}

type eventSearchBody struct {
	FindEventsResponse            *findEventsResponse            `xml:"FindEventsResponse"`
	GetEventSearchResultsResponse *getEventSearchResultsResponse `xml:"GetEventSearchResultsResponse"`
	Fault                         *recordingSearchFault          `xml:"Fault"`
}

type findEventsResponse struct {
	SearchToken string `xml:"SearchToken"`
}

type getEventSearchResultsResponse struct {
	ResultList eventResultList `xml:"ResultList"`
}

type eventResultList struct {
	SearchState string           `xml:"SearchState"`
	Result      []eventResultXML `xml:"Result"`
}

type eventResultXML struct {
	RecordingToken string        `xml:"RecordingToken"`
	TrackToken     string        `xml:"TrackToken"`
	Time           string        `xml:"Time"`
	Event          eventEntryXML `xml:"Event"`
}

type eventEntryXML struct {
	Topic      eventTopicXML      `xml:"Topic"`
	Message    eventMessageWrapper `xml:"Message"`
}

type eventMessageWrapper struct {
	Inner eventInnerMessage `xml:"Message"`
}

type eventInnerMessage struct {
	PropertyOperation string       `xml:"PropertyOperation,attr"`
	Data              eventDataXML `xml:"Data"`
}

type eventTopicXML struct {
	Value string `xml:",chardata"`
}

type eventDataXML struct {
	SimpleItems []eventSimpleItem `xml:"SimpleItem"`
}

type eventSimpleItem struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

// FindEvents discovers events stored on the camera's edge storage using
// the session-based search pattern: FindEvents -> poll GetEventSearchResults.
func FindEvents(xaddr, username, password string, filter *EdgeSearchFilter) ([]EdgeEvent, error) {
	searchURL, err := getSearchServiceURL(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("FindEvents: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Initiate a FindEvents search session.
	findBody := buildFindEventsBody(filter)

	body, err := doRecordingSearchSOAP(ctx, searchURL, username, password, findBody)
	if err != nil {
		return nil, fmt.Errorf("FindEvents initiate: %w", err)
	}

	var findEnv eventSearchEnvelope
	if err := xml.Unmarshal(body, &findEnv); err != nil {
		return nil, fmt.Errorf("FindEvents parse search token: %w", err)
	}
	if findEnv.Body.Fault != nil {
		return nil, fmt.Errorf("FindEvents SOAP fault: %s", findEnv.Body.Fault.Faultstring)
	}
	if findEnv.Body.FindEventsResponse == nil {
		return nil, fmt.Errorf("FindEvents: empty response (no search token)")
	}

	searchToken := findEnv.Body.FindEventsResponse.SearchToken
	if searchToken == "" {
		return nil, fmt.Errorf("FindEvents: empty search token")
	}

	// Step 2: Poll GetEventSearchResults until SearchState is "Completed".
	var allEvents []EdgeEvent
	for {
		select {
		case <-ctx.Done():
			return allEvents, fmt.Errorf("FindEvents: timed out waiting for search results")
		default:
		}

		getResultsBody := fmt.Sprintf(`<tse:GetEventSearchResults>
      <tse:SearchToken>%s</tse:SearchToken>
      <tse:MinResults>1</tse:MinResults>
      <tse:MaxResults>50</tse:MaxResults>
      <tse:WaitTime>PT5S</tse:WaitTime>
    </tse:GetEventSearchResults>`, xmlEscape(searchToken))

		resultBody, err := doRecordingSearchSOAP(ctx, searchURL, username, password, getResultsBody)
		if err != nil {
			return allEvents, fmt.Errorf("FindEvents poll results: %w", err)
		}

		var resultEnv eventSearchEnvelope
		if err := xml.Unmarshal(resultBody, &resultEnv); err != nil {
			return allEvents, fmt.Errorf("FindEvents parse results: %w", err)
		}
		if resultEnv.Body.Fault != nil {
			return allEvents, fmt.Errorf("FindEvents results SOAP fault: %s", resultEnv.Body.Fault.Faultstring)
		}
		if resultEnv.Body.GetEventSearchResultsResponse == nil {
			return allEvents, fmt.Errorf("FindEvents: empty search results response")
		}

		resultList := resultEnv.Body.GetEventSearchResultsResponse.ResultList

		for _, r := range resultList.Result {
			propOp := r.Event.Message.Inner.PropertyOperation
			simpleItems := r.Event.Message.Inner.Data.SimpleItems
			evt := EdgeEvent{
				RecordingToken: r.RecordingToken,
				TrackToken:     r.TrackToken,
				Time:           r.Time,
				Topic:          strings.TrimSpace(r.Event.Topic.Value),
				IsProperty:     strings.EqualFold(propOp, "Changed") || strings.EqualFold(propOp, "Initialized"),
				IsDataPresent:  len(simpleItems) > 0,
			}
			if len(simpleItems) > 0 {
				evt.Data = make(map[string]string, len(simpleItems))
				for _, si := range simpleItems {
					evt.Data[si.Name] = si.Value
				}
			}
			allEvents = append(allEvents, evt)
		}

		if strings.EqualFold(resultList.SearchState, "Completed") {
			break
		}

		// Brief pause before next poll to avoid hammering the device.
		time.Sleep(500 * time.Millisecond)
	}

	return allEvents, nil
}

// FindRecordingsFiltered discovers recordings with optional time-range filtering.
// It extends FindRecordings with support for EdgeSearchFilter.
func FindRecordingsFiltered(xaddr, username, password string, filter *EdgeSearchFilter) ([]EdgeRecording, error) {
	searchURL, err := getSearchServiceURL(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("FindRecordingsFiltered: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	findBody := buildFindRecordingsBody(filter)

	body, err := doRecordingSearchSOAP(ctx, searchURL, username, password, findBody)
	if err != nil {
		return nil, fmt.Errorf("FindRecordingsFiltered initiate: %w", err)
	}

	var findEnv recordingSearchEnvelope
	if err := xml.Unmarshal(body, &findEnv); err != nil {
		return nil, fmt.Errorf("FindRecordingsFiltered parse search token: %w", err)
	}
	if findEnv.Body.Fault != nil {
		return nil, fmt.Errorf("FindRecordingsFiltered SOAP fault: %s", findEnv.Body.Fault.Faultstring)
	}
	if findEnv.Body.FindRecordingsResponse == nil {
		return nil, fmt.Errorf("FindRecordingsFiltered: empty response (no search token)")
	}

	searchToken := findEnv.Body.FindRecordingsResponse.SearchToken
	if searchToken == "" {
		return nil, fmt.Errorf("FindRecordingsFiltered: empty search token")
	}

	var allRecordings []EdgeRecording
	for {
		select {
		case <-ctx.Done():
			return allRecordings, fmt.Errorf("FindRecordingsFiltered: timed out waiting for search results")
		default:
		}

		getResultsBody := fmt.Sprintf(`<tse:GetRecordingSearchResults>
      <tse:SearchToken>%s</tse:SearchToken>
      <tse:MinResults>1</tse:MinResults>
      <tse:MaxResults>50</tse:MaxResults>
      <tse:WaitTime>PT5S</tse:WaitTime>
    </tse:GetRecordingSearchResults>`, xmlEscape(searchToken))

		resultBody, err := doRecordingSearchSOAP(ctx, searchURL, username, password, getResultsBody)
		if err != nil {
			return allRecordings, fmt.Errorf("FindRecordingsFiltered poll results: %w", err)
		}

		var resultEnv recordingSearchEnvelope
		if err := xml.Unmarshal(resultBody, &resultEnv); err != nil {
			return allRecordings, fmt.Errorf("FindRecordingsFiltered parse results: %w", err)
		}
		if resultEnv.Body.Fault != nil {
			return allRecordings, fmt.Errorf("FindRecordingsFiltered results SOAP fault: %s", resultEnv.Body.Fault.Faultstring)
		}
		if resultEnv.Body.GetRecordingSearchResultsResponse == nil {
			return allRecordings, fmt.Errorf("FindRecordingsFiltered: empty search results response")
		}

		resultList := resultEnv.Body.GetRecordingSearchResultsResponse.ResultList

		for _, ri := range resultList.RecordingInformation {
			allRecordings = append(allRecordings, EdgeRecording{
				RecordingToken: ri.RecordingToken,
				SourceName:     ri.Source.Name,
				EarliestTime:   ri.EarliestRecording,
				LatestTime:     ri.LatestRecording,
			})
		}

		if strings.EqualFold(resultList.SearchState, "Completed") {
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	return allRecordings, nil
}

// buildFindEventsBody constructs the SOAP body for a FindEvents request
// with optional time range, recording token, and event type filters.
func buildFindEventsBody(filter *EdgeSearchFilter) string {
	var sb strings.Builder
	sb.WriteString(`<tse:FindEvents>
      <tse:StartPoint>`)
	if filter != nil && filter.StartTime != nil {
		sb.WriteString(filter.StartTime.UTC().Format(time.RFC3339))
	} else {
		sb.WriteString("2000-01-01T00:00:00Z")
	}
	sb.WriteString(`</tse:StartPoint>
      <tse:EndPoint>`)
	if filter != nil && filter.EndTime != nil {
		sb.WriteString(filter.EndTime.UTC().Format(time.RFC3339))
	} else {
		sb.WriteString("9999-12-31T23:59:59Z")
	}
	sb.WriteString(`</tse:EndPoint>`)

	sb.WriteString(`
      <tse:SearchFilter>`)
	if filter != nil && filter.RecordingToken != "" {
		sb.WriteString(fmt.Sprintf(`
        <tse:IncludedRecordings>%s</tse:IncludedRecordings>`, xmlEscape(filter.RecordingToken)))
	}
	if filter != nil && filter.EventType != "" {
		sb.WriteString(fmt.Sprintf(`
        <tse:TopicExpression Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">%s</tse:TopicExpression>`, xmlEscape(filter.EventType)))
	}
	sb.WriteString(`
      </tse:SearchFilter>`)

	sb.WriteString(`
      <tse:IncludeStartState>false</tse:IncludeStartState>
    </tse:FindEvents>`)
	return sb.String()
}

// buildFindRecordingsBody constructs the SOAP body for a FindRecordings request
// with optional time range and recording token filters.
func buildFindRecordingsBody(filter *EdgeSearchFilter) string {
	var sb strings.Builder
	sb.WriteString(`<tse:FindRecordings>
      <tse:Scope>`)

	if filter != nil && filter.RecordingToken != "" {
		sb.WriteString(fmt.Sprintf(`
        <tse:IncludedRecordings>%s</tse:IncludedRecordings>`, xmlEscape(filter.RecordingToken)))
	} else {
		sb.WriteString(`
        <tse:IncludedSources/>`)
	}

	if filter != nil && filter.StartTime != nil {
		sb.WriteString(fmt.Sprintf(`
        <tse:RecordingInformationFilter>boolean(//Track[DataFrom &gt;= "%s"])</tse:RecordingInformationFilter>`,
			filter.StartTime.UTC().Format(time.RFC3339)))
	}

	sb.WriteString(`
      </tse:Scope>`)

	// MaxMatches is optional but some cameras require it.
	sb.WriteString(`
      <tse:MaxMatches>100</tse:MaxMatches>
      <tse:KeepAliveTime>PT10S</tse:KeepAliveTime>
    </tse:FindRecordings>`)
	return sb.String()
}
