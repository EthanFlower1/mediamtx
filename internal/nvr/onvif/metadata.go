package onvif

import "encoding/xml"

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
