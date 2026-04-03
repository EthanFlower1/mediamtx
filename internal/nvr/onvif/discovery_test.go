package onvif

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupProfilesByVideoSource(t *testing.T) {
	profiles := []MediaProfile{
		{Token: "P1", Name: "Main Ch1", Width: 2560, Height: 1440, VideoSourceToken: "VS_1"},
		{Token: "P2", Name: "Sub Ch1", Width: 640, Height: 480, VideoSourceToken: "VS_1"},
		{Token: "P3", Name: "Main Ch2", Width: 2560, Height: 1440, VideoSourceToken: "VS_2"},
		{Token: "P4", Name: "Sub Ch2", Width: 640, Height: 480, VideoSourceToken: "VS_2"},
	}

	channels := GroupProfilesByVideoSource(profiles)
	require.Len(t, channels, 2)

	assert.Equal(t, "VS_1", channels[0].VideoSourceToken)
	assert.Len(t, channels[0].Profiles, 2)
	assert.Equal(t, "Channel 1", channels[0].Name)

	assert.Equal(t, "VS_2", channels[1].VideoSourceToken)
	assert.Len(t, channels[1].Profiles, 2)
	assert.Equal(t, "Channel 2", channels[1].Name)
}

func TestGroupProfilesByVideoSourceSingleChannel(t *testing.T) {
	profiles := []MediaProfile{
		{Token: "P1", Name: "Main", Width: 1920, Height: 1080, VideoSourceToken: "VS_1"},
		{Token: "P2", Name: "Sub", Width: 640, Height: 480, VideoSourceToken: "VS_1"},
	}

	channels := GroupProfilesByVideoSource(profiles)
	require.Len(t, channels, 1)
	assert.Equal(t, "VS_1", channels[0].VideoSourceToken)
	assert.Len(t, channels[0].Profiles, 2)
}

func TestGroupProfilesByVideoSourceEmptyTokens(t *testing.T) {
	profiles := []MediaProfile{
		{Token: "P1", Name: "Main", Width: 1920, Height: 1080},
		{Token: "P2", Name: "Sub", Width: 640, Height: 480},
	}

	channels := GroupProfilesByVideoSource(profiles)
	require.Len(t, channels, 1)
	assert.Len(t, channels[0].Profiles, 2)
}
