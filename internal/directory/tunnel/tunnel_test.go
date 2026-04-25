package tunnel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate_EmptyConfig(t *testing.T) {
	_, err := New(Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ServerAddr is required")
}

func TestValidate_MissingToken(t *testing.T) {
	_, err := New(Config{
		ServerAddr: "relay.example.com",
		ServerPort: 7000,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Token is required")
}

func TestValidate_MissingSubDomain(t *testing.T) {
	_, err := New(Config{
		ServerAddr: "relay.example.com",
		ServerPort: 7000,
		Token:      "secret",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "SubDomain is required")
}

func TestValidate_MissingAPIPort(t *testing.T) {
	_, err := New(Config{
		ServerAddr: "relay.example.com",
		ServerPort: 7000,
		Token:      "secret",
		SubDomain:  "site1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "LocalPorts.API must be positive")
}

func TestNew_ValidConfig(t *testing.T) {
	tun, err := New(Config{
		ServerAddr: "relay.example.com",
		ServerPort: 7000,
		Token:      "secret",
		SubDomain:  "site1",
		LocalPorts: LocalPorts{
			API:      9995,
			HLS:      8898,
			WebRTC:   8889,
			Playback: 9996,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, tun)
}

func TestBuildProxies_AllPorts(t *testing.T) {
	cfg := Config{
		SubDomain: "mysite",
		LocalPorts: LocalPorts{
			API:      9995,
			HLS:      8898,
			WebRTC:   8889,
			Playback: 9996,
		},
	}
	proxies := buildProxies(cfg)
	require.Len(t, proxies, 4)

	names := make([]string, len(proxies))
	for i, p := range proxies {
		names[i] = p.GetBaseConfig().Name
	}
	require.Contains(t, names, "mysite-api")
	require.Contains(t, names, "mysite-hls")
	require.Contains(t, names, "mysite-webrtc")
	require.Contains(t, names, "mysite-playback")
}

func TestBuildProxies_SkipsZeroPorts(t *testing.T) {
	cfg := Config{
		SubDomain: "mysite",
		LocalPorts: LocalPorts{
			API: 9995,
			// HLS, WebRTC, Playback left at zero
		},
	}
	proxies := buildProxies(cfg)
	require.Len(t, proxies, 1)
	require.Equal(t, "mysite-api", proxies[0].GetBaseConfig().Name)
}
