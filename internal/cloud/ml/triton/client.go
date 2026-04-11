// Package triton provides a Go client for the NVIDIA Triton Inference Server
// gRPC API (KAI-277). It implements per-model inference requests, health
// checks, and Prometheus metrics collection for latency, throughput, and
// queue depth tracking.
//
// Multi-tenant invariant: every InferRequest carries a TenantID that is
// propagated as gRPC metadata (x-kaivue-tenant-id) for per-tenant routing
// in the service mesh.
//
// Package boundary: this package imports only stdlib, gRPC, and prometheus.
// It never imports other internal/cloud packages to avoid cycles.
package triton

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

// Client is a thread-safe Triton Inference Server gRPC client.
type Client struct {
	conn    *grpc.ClientConn
	addr    string
	metrics *Metrics
	mu      sync.RWMutex
	closed  bool
}

// ClientConfig holds the configuration for a Triton client.
type ClientConfig struct {
	// Address is the Triton gRPC endpoint (host:port).
	Address string

	// TLSConfig enables mTLS for the gRPC connection. If nil, insecure.
	TLSConfig *TLSConfig

	// MaxRetries is the number of retry attempts for transient failures.
	MaxRetries int

	// Timeout is the default per-request timeout.
	Timeout time.Duration

	// Metrics is the Prometheus metrics collector. If nil, metrics are no-ops.
	Metrics *Metrics
}

// TLSConfig holds mTLS certificate paths.
type TLSConfig struct {
	CACertPath     string
	ClientCertPath string
	ClientKeyPath  string
}

// NewClient creates a new Triton gRPC client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("triton: address is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}

	dialOpts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(64 * 1024 * 1024), // 64MB for large tensors
			grpc.MaxCallSendMsgSize(64 * 1024 * 1024),
		),
	}

	if cfg.TLSConfig != nil {
		tlsCreds, err := buildTLSCredentials(cfg.TLSConfig)
		if err != nil {
			return nil, fmt.Errorf("triton: tls config: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(tlsCreds))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure()) //nolint:staticcheck
	}

	conn, err := grpc.Dial(cfg.Address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("triton: dial %s: %w", cfg.Address, err)
	}

	m := cfg.Metrics
	if m == nil {
		m = NewMetrics() // creates unregistered (no-op) metrics
	}

	return &Client{
		conn:    conn,
		addr:    cfg.Address,
		metrics: m,
	}, nil
}

// Close shuts down the gRPC connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

// ServerLive checks if the Triton server is live.
func (c *Client) ServerLive(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp := &ServerLiveResponse{}
	err := c.conn.Invoke(ctx, "/inference.GRPCInferenceService/ServerLive", &ServerLiveRequest{}, resp)
	if err != nil {
		return false, fmt.Errorf("triton: server live check: %w", err)
	}
	return resp.Live, nil
}

// ServerReady checks if the Triton server is ready to accept requests.
func (c *Client) ServerReady(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp := &ServerReadyResponse{}
	err := c.conn.Invoke(ctx, "/inference.GRPCInferenceService/ServerReady", &ServerReadyRequest{}, resp)
	if err != nil {
		return false, fmt.Errorf("triton: server ready check: %w", err)
	}
	return resp.Ready, nil
}

// ModelReady checks if a specific model is loaded and ready.
func (c *Client) ModelReady(ctx context.Context, modelName, modelVersion string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req := &ModelReadyRequest{
		Name:    modelName,
		Version: modelVersion,
	}
	resp := &ModelReadyResponse{}
	err := c.conn.Invoke(ctx, "/inference.GRPCInferenceService/ModelReady", req, resp)
	if err != nil {
		return false, fmt.Errorf("triton: model ready check %s/%s: %w", modelName, modelVersion, err)
	}
	return resp.Ready, nil
}

// Infer sends an inference request to the specified model with per-tenant
// routing metadata. It tracks latency, throughput, and errors in Prometheus.
func (c *Client) Infer(ctx context.Context, req *InferRequest) (*InferResponse, error) {
	if req.ModelName == "" {
		return nil, fmt.Errorf("triton: model_name is required")
	}
	if req.TenantID == "" {
		return nil, fmt.Errorf("triton: tenant_id is required")
	}

	// Inject tenant ID as gRPC metadata for service mesh routing.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-kaivue-tenant-id", req.TenantID)

	start := time.Now()
	c.metrics.InferenceRequestsTotal.WithLabelValues(req.ModelName, req.TenantID).Inc()
	c.metrics.InferenceQueueDepth.WithLabelValues(req.ModelName).Inc()
	defer c.metrics.InferenceQueueDepth.WithLabelValues(req.ModelName).Dec()

	grpcReq := &ModelInferRequest{
		ModelName:    req.ModelName,
		ModelVersion: req.ModelVersion,
		ID:           req.RequestID,
		Inputs:       make([]*ModelInferRequest_InferInputTensor, 0, len(req.Inputs)),
		Outputs:      make([]*ModelInferRequest_InferRequestedOutputTensor, 0, len(req.Outputs)),
	}

	for _, inp := range req.Inputs {
		grpcReq.Inputs = append(grpcReq.Inputs, &ModelInferRequest_InferInputTensor{
			Name:     inp.Name,
			Datatype: inp.Datatype,
			Shape:    inp.Shape,
			Contents: &InferTensorContents{Fp32Contents: inp.FP32Data},
		})
	}

	for _, out := range req.Outputs {
		grpcReq.Outputs = append(grpcReq.Outputs, &ModelInferRequest_InferRequestedOutputTensor{
			Name: out.Name,
		})
	}

	grpcResp := &ModelInferResponse{}
	err := c.conn.Invoke(ctx, "/inference.GRPCInferenceService/ModelInfer", grpcReq, grpcResp)

	duration := time.Since(start)
	c.metrics.InferenceLatency.WithLabelValues(req.ModelName, req.TenantID).Observe(duration.Seconds())

	if err != nil {
		c.metrics.InferenceErrorsTotal.WithLabelValues(req.ModelName, req.TenantID).Inc()
		return nil, fmt.Errorf("triton: infer %s: %w", req.ModelName, err)
	}

	c.metrics.InferenceThroughput.WithLabelValues(req.ModelName, req.TenantID).Inc()

	resp := &InferResponse{
		ModelName:    grpcResp.ModelName,
		ModelVersion: grpcResp.ModelVersion,
		ID:           grpcResp.ID,
		Outputs:      make([]OutputTensor, 0, len(grpcResp.Outputs)),
		LatencyMs:    float64(duration.Milliseconds()),
	}

	for _, out := range grpcResp.Outputs {
		ot := OutputTensor{
			Name:     out.Name,
			Datatype: out.Datatype,
			Shape:    out.Shape,
		}
		if out.Contents != nil {
			ot.FP32Data = out.Contents.Fp32Contents
		}
		resp.Outputs = append(resp.Outputs, ot)
	}

	return resp, nil
}

func buildTLSCredentials(cfg *TLSConfig) (credentials.TransportCredentials, error) {
	caCert, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA cert")
	}

	clientCert, err := tls.LoadX509KeyPair(cfg.ClientCertPath, cfg.ClientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
		MinVersion:   tls.VersionTLS13,
	}), nil
}
