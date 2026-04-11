package summaries

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TritonClient calls a Triton Inference Server hosting the LLM.
// It uses the HTTP/REST v2 inference protocol.
//
// The client enforces per-tenant isolation by never mixing tenant data
// in a single request and by including no cross-tenant context.
type TritonClient struct {
	cfg    TritonConfig
	client *http.Client
}

// NewTritonClient constructs a TritonClient with the given config.
func NewTritonClient(cfg TritonConfig) *TritonClient {
	return &TritonClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
	}
}

// tritonRequest is the KServe v2 inference request body.
type tritonRequest struct {
	Inputs     []tritonInput      `json:"inputs"`
	Parameters map[string]any     `json:"parameters,omitempty"`
}

type tritonInput struct {
	Name     string   `json:"name"`
	Shape    []int    `json:"shape"`
	Datatype string   `json:"datatype"`
	Data     []string `json:"data"`
}

// tritonResponse is the relevant subset of the v2 inference response.
type tritonResponse struct {
	Outputs []tritonOutput `json:"outputs"`
}

type tritonOutput struct {
	Name string   `json:"name"`
	Data []string `json:"data"`
}

// Infer sends a prompt to the Triton-hosted LLM and returns the generated
// text. The system prompt and user prompt are sent as separate inputs
// following the chat-completion format that vLLM/TensorRT-LLM backends
// on Triton expect.
//
// This method MUST be called with a single tenant's data per invocation
// to maintain per-tenant isolation.
func (tc *TritonClient) Infer(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := tritonRequest{
		Inputs: []tritonInput{
			{
				Name:     "text_input",
				Shape:    []int{1},
				Datatype: "BYTES",
				Data:     []string{formatChatPrompt(systemPrompt, userPrompt)},
			},
		},
		Parameters: map[string]any{
			"max_tokens":  tc.cfg.MaxTokens,
			"temperature": tc.cfg.Temperature,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal triton request: %w", err)
	}

	url := fmt.Sprintf("http://%s/v2/models/%s/infer",
		tc.cfg.Endpoint, tc.cfg.ModelName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create triton request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTritonUnavail, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("%w: status %d: %s", ErrInferenceFailed, resp.StatusCode, string(respBody))
	}

	var tritonResp tritonResponse
	if err := json.NewDecoder(resp.Body).Decode(&tritonResp); err != nil {
		return "", fmt.Errorf("decode triton response: %w", err)
	}

	if len(tritonResp.Outputs) == 0 || len(tritonResp.Outputs[0].Data) == 0 {
		return "", fmt.Errorf("%w: empty response from model", ErrInferenceFailed)
	}

	return tritonResp.Outputs[0].Data[0], nil
}

// formatChatPrompt formats system + user prompts into the Llama 3 chat
// template. This keeps the LLM interaction self-contained within a single
// text_input, compatible with vLLM and TensorRT-LLM backends on Triton.
func formatChatPrompt(system, user string) string {
	return fmt.Sprintf(
		"<|begin_of_text|><|start_header_id|>system<|end_header_id|>\n\n%s<|eot_id|>"+
			"<|start_header_id|>user<|end_header_id|>\n\n%s<|eot_id|>"+
			"<|start_header_id|>assistant<|end_header_id|>\n\n",
		system, user)
}
