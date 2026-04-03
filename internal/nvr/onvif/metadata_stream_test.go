package onvif

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataStreamSubscriberRequiresStreamURI(t *testing.T) {
	cb := func(eventType DetectedEventType, active bool) {}
	sub, err := NewMetadataStreamSubscriber("", "", "", cb, nil)
	require.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "stream URI is required")
}

func TestMetadataStreamSubscriberRequiresCallback(t *testing.T) {
	sub, err := NewMetadataStreamSubscriber("rtsp://192.168.1.100:554/metadata", "", "", nil, nil)
	require.Error(t, err)
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "at least one callback")
}

func TestMetadataStreamSubscriberCreation(t *testing.T) {
	eventCb := func(eventType DetectedEventType, active bool) {}
	frameCb := func(frame *MetadataFrame) {}

	// With both callbacks.
	sub, err := NewMetadataStreamSubscriber("rtsp://192.168.1.100:554/metadata", "admin", "pass", eventCb, frameCb)
	require.NoError(t, err)
	require.NotNil(t, sub)
	assert.Equal(t, "rtsp://192.168.1.100:554/metadata", sub.streamURI)
	assert.Equal(t, "admin", sub.username)
	assert.Equal(t, "pass", sub.password)

	// With only event callback.
	sub2, err := NewMetadataStreamSubscriber("rtsp://192.168.1.100:554/metadata", "", "", eventCb, nil)
	require.NoError(t, err)
	require.NotNil(t, sub2)

	// With only frame callback.
	sub3, err := NewMetadataStreamSubscriber("rtsp://192.168.1.100:554/metadata", "", "", nil, frameCb)
	require.NoError(t, err)
	require.NotNil(t, sub3)
}
