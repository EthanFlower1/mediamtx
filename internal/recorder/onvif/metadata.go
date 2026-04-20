package onvif

import (
	"encoding/xml"
	"strings"
)

// MetadataStream represents a full ONVIF metadata stream message
// containing one or more video analytics frames.
type MetadataStream struct {
	XMLName xml.Name        `xml:"MetadataStream"`
	Frames  []MetadataFrame `xml:"VideoAnalytics>Frame"`
}

// MetadataFrame represents a single analytics frame with a UTC timestamp
// and zero or more detected objects.
type MetadataFrame struct {
	UtcTime string           `xml:"UtcTime,attr"`
	Objects []MetadataObject `xml:"Object"`
}

// MetadataObject represents a single detected object within an analytics frame.
type MetadataObject struct {
	ObjectId string              `xml:"ObjectId,attr"`
	Class    string              `xml:"Appearance>Class>Type"`
	Score    float64             `xml:"Appearance>Class>Type>Likelihood,attr"`
	Box      MetadataBoundingBox `xml:"Appearance>Shape>BoundingBox"`
}

// MetadataBoundingBox represents the bounding box of a detected object
// using normalized ONVIF coordinates.
type MetadataBoundingBox struct {
	Left   float64 `xml:"left,attr"`
	Top    float64 `xml:"top,attr"`
	Right  float64 `xml:"right,attr"`
	Bottom float64 `xml:"bottom,attr"`
}

// EventMetadata represents a motion or scene-change event extracted from
// an ONVIF metadata stream frame.
type EventMetadata struct {
	Topic  string // e.g. "Motion", "GlobalSceneChange"
	Source string // source token (video source)
	Active bool
}

// MetadataStreamFull parses both analytics frames and event notifications
// from a metadata stream XML chunk.
type MetadataStreamFull struct {
	XMLName xml.Name        `xml:"MetadataStream"`
	Frames  []MetadataFrame `xml:"VideoAnalytics>Frame"`
	Events  []metadataEvent `xml:"Event>NotificationMessage"`
}

type metadataEvent struct {
	Topic   string          `xml:"Topic"`
	Message metadataMessage `xml:"Message>Message"`
}

type metadataMessage struct {
	Source metadataItemSet `xml:"Source"`
	Data   metadataItemSet `xml:"Data"`
}

type metadataItemSet struct {
	SimpleItems []metadataSimpleItem `xml:"SimpleItem"`
}

type metadataSimpleItem struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

// ParseMetadataFrame parses raw XML bytes into a MetadataFrame.
// It first tries to parse as a full MetadataStream, then falls back
// to parsing as a single Frame element.
func ParseMetadataFrame(data []byte) (*MetadataFrame, error) {
	var stream MetadataStream
	if err := xml.Unmarshal(data, &stream); err != nil {
		// Try parsing as a single Frame
		var frame MetadataFrame
		if err2 := xml.Unmarshal(data, &frame); err2 != nil {
			return nil, err
		}
		return &frame, nil
	}
	if len(stream.Frames) == 0 {
		return nil, nil
	}
	return &stream.Frames[0], nil
}

// ParseMetadataStreamFull parses a metadata stream XML chunk and returns
// both analytics frames and event notifications.
func ParseMetadataStreamFull(data []byte) (*MetadataFrame, []EventMetadata, error) {
	var stream MetadataStreamFull
	if err := xml.Unmarshal(data, &stream); err != nil {
		// Fall back to single-frame parsing.
		frame, err := ParseMetadataFrame(data)
		return frame, nil, err
	}

	var frame *MetadataFrame
	if len(stream.Frames) > 0 {
		frame = &stream.Frames[0]
	}

	var events []EventMetadata
	for _, evt := range stream.Events {
		em := EventMetadata{Topic: evt.Topic}
		for _, item := range evt.Message.Source.SimpleItems {
			if item.Name == "VideoSourceConfigurationToken" || item.Name == "Source" {
				em.Source = item.Value
			}
		}
		for _, item := range evt.Message.Data.SimpleItems {
			lower := strings.ToLower(item.Name)
			if lower == "ismotion" || lower == "state" {
				em.Active = strings.ToLower(item.Value) == "true" || item.Value == "1"
			}
		}
		events = append(events, em)
	}

	return frame, events, nil
}
