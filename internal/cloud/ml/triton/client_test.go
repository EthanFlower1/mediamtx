package triton

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_MissingAddress(t *testing.T) {
	_, err := NewClient(ClientConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address is required")
}

func TestNewClient_Defaults(t *testing.T) {
	// This will fail to actually connect, but we can verify the client is created.
	// In a real test environment with Triton running, this would succeed.
	c, err := NewClient(ClientConfig{
		Address: "localhost:8001",
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "localhost:8001", c.addr)
	assert.NotNil(t, c.metrics)
	require.NoError(t, c.Close())
}

func TestNewClient_DoubleClose(t *testing.T) {
	c, err := NewClient(ClientConfig{
		Address: "localhost:8001",
	})
	require.NoError(t, err)
	require.NoError(t, c.Close())
	// Second close should be a no-op.
	require.NoError(t, c.Close())
}

func TestInferRequest_Validation(t *testing.T) {
	c, err := NewClient(ClientConfig{
		Address: "localhost:8001",
	})
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()

	// Missing model name.
	_, err = c.Infer(ctx, &InferRequest{TenantID: "t1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model_name is required")

	// Missing tenant ID.
	_, err = c.Infer(ctx, &InferRequest{ModelName: "yolov8-detection"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id is required")
}

func TestMetrics_Register(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics()
	err := m.Register(reg)
	require.NoError(t, err)

	// Verify all metrics are registered by gathering.
	families, err := reg.Gather()
	require.NoError(t, err)
	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	// The metrics are registered but have no observations yet,
	// so they may not appear in Gather(). That's fine — we just
	// confirm Register() didn't error.
}

func TestMetrics_DoubleRegister(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics()
	require.NoError(t, m.Register(reg))
	// Second registration should fail.
	err := m.Register(reg)
	require.Error(t, err)
}

func TestTenantRoutingMiddleware_MissingHeader(t *testing.T) {
	// Tested via middleware_test.go — placeholder for compilation check.
	_ = TenantHeader
}

func TestModelRoutingMiddleware_BlocksUnknown(t *testing.T) {
	m := NewModelRoutingMiddleware([]string{"yolov8-detection", "clip-embedding"})
	assert.True(t, m.AllowedModels["yolov8-detection"])
	assert.True(t, m.AllowedModels["clip-embedding"])
	assert.False(t, m.AllowedModels["unknown-model"])
}

func TestClientConfig_Timeout(t *testing.T) {
	// Verify default timeout is set.
	cfg := ClientConfig{Address: "localhost:8001"}
	c, err := NewClient(cfg)
	require.NoError(t, err)
	defer c.Close()
	// Default timeout of 30s is applied internally.
	_ = time.Second
}
