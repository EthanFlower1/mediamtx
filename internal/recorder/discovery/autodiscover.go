package discovery

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultDiscoverTimeout is the recommended mDNS listen window.
	DefaultDiscoverTimeout = 30 * time.Second
	// DefaultApprovalTimeout is how long to poll for admin decision.
	DefaultApprovalTimeout = 10 * time.Minute
	// pollInterval is how often the Recorder checks for admin approval.
	pollInterval = 5 * time.Second
)

// RunOptions controls a single AutoDiscoverer.Run call.
type RunOptions struct {
	// DiscoverTimeout is how long to listen for mDNS broadcasts.
	DiscoverTimeout time.Duration
	// ApprovalTimeout is how long to poll for admin approval.
	ApprovalTimeout time.Duration
	// SkipPrompt skips the interactive "Detected Directory at X. Join? [y/N]"
	// prompt and proceeds automatically. For non-interactive (CI / scripted)
	// use.
	SkipPrompt bool
	// Stdin, Stdout, Stderr override the standard streams for testing.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// HTTPClient overrides the HTTP client used for API calls. Nil = default.
	HTTPClient *http.Client
}

// AutoDiscovererConfig parameterises an AutoDiscoverer.
type AutoDiscovererConfig struct {
	Logger *slog.Logger
}

// AutoDiscoverer orchestrates the mDNS discovery + admin-approval flow.
// It is safe for single-shot concurrent use.
type AutoDiscoverer struct {
	log *slog.Logger
}

// NewAutoDiscoverer constructs an AutoDiscoverer.
func NewAutoDiscoverer(cfg AutoDiscovererConfig) *AutoDiscoverer {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &AutoDiscoverer{log: log}
}

// Run executes the full auto-discover flow:
//  1. Listen for mDNS broadcast (opts.DiscoverTimeout).
//  2. Prompt the operator to confirm (skipped if opts.SkipPrompt).
//  3. POST /api/v1/pairing/request to the discovered Directory.
//  4. Poll GET /api/v1/pairing/request/{id}/token until approved or timeout.
//  5. Return the raw pairing token string (ready for Joiner.Run).
func (a *AutoDiscoverer) Run(ctx context.Context, opts RunOptions) (string, error) {
	if opts.DiscoverTimeout == 0 {
		opts.DiscoverTimeout = DefaultDiscoverTimeout
	}
	if opts.ApprovalTimeout == 0 {
		opts.ApprovalTimeout = DefaultApprovalTimeout
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	// Step 1: discover.
	fmt.Fprintf(stdout, "Listening for Kaivue Directory on local network (up to %s)...\n", opts.DiscoverTimeout)
	info, err := Listen(opts.DiscoverTimeout, nil, a.log)
	if err != nil {
		if errors.Is(err, ErrTimeout) {
			return "", fmt.Errorf("no Directory found on LAN within %s — run 'mediamtx-pair <token>' instead", opts.DiscoverTimeout)
		}
		return "", fmt.Errorf("mDNS listen: %w", err)
	}

	dirURL := fmt.Sprintf("http://%s:%d", strings.TrimSuffix(info.Hostname, "."), info.Port)
	fmt.Fprintf(stdout, "\nDetected Directory at %s (source IP: %s)\n", dirURL, info.SourceIP)

	// Step 2: prompt.
	if !opts.SkipPrompt {
		fmt.Fprintf(stdout, "Join this Directory? [y/N] ")
		reader := bufio.NewReader(stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			return "", fmt.Errorf("user declined to join Directory at %s", dirURL)
		}
	}

	// Step 3: submit pairing request.
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-recorder"
	}

	reqBody, _ := json.Marshal(map[string]any{
		"recorder_hostname": hostname,
		"requested_roles":   []string{"recorder"},
	})
	pairURL := dirURL + "/api/v1/pairing/request"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, pairURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("build pairing request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", pairURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pairing request rejected (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var pendingResp struct {
		ID        string `json:"id"`
		ExpiresIn string `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pendingResp); err != nil {
		return "", fmt.Errorf("parse pairing request response: %w", err)
	}

	fmt.Fprintf(stdout, "\nPairing request submitted (ID: %s, expires in: %s)\n", pendingResp.ID, pendingResp.ExpiresIn)
	fmt.Fprintf(stdout, "Waiting for admin to approve at the Directory console...\n")
	fmt.Fprintf(stderr, "  Directory admin URL: %s\n", dirURL+"/admin")

	// Step 4: poll for approval.
	pollURL := fmt.Sprintf("%s/api/v1/pairing/request/%s/token", dirURL, pendingResp.ID)
	deadline := time.Now().Add(opts.ApprovalTimeout)
	for {
		if ctx.Err() != nil {
			return "", fmt.Errorf("cancelled while waiting for approval")
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("admin did not approve within %s", opts.ApprovalTimeout)
		}

		pollReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if err != nil {
			return "", fmt.Errorf("build poll request: %w", err)
		}
		pollResp, err := client.Do(pollReq)
		if err != nil {
			a.log.Warn("discovery: poll error, retrying", "error", err)
			time.Sleep(pollInterval)
			continue
		}

		switch pollResp.StatusCode {
		case http.StatusAccepted:
			// Still pending.
			_ = pollResp.Body.Close()
			fmt.Fprintf(stdout, ".")
			time.Sleep(pollInterval)

		case http.StatusOK:
			// Approved.
			var tokenResp struct {
				Token string `json:"token"`
			}
			if err := json.NewDecoder(pollResp.Body).Decode(&tokenResp); err != nil {
				_ = pollResp.Body.Close()
				return "", fmt.Errorf("parse token response: %w", err)
			}
			_ = pollResp.Body.Close()
			fmt.Fprintf(stdout, "\nApproved! Running join sequence...\n")
			return tokenResp.Token, nil

		case http.StatusForbidden:
			_ = pollResp.Body.Close()
			return "", fmt.Errorf("pairing request was denied by admin")

		case http.StatusGone:
			_ = pollResp.Body.Close()
			return "", fmt.Errorf("pairing request expired without admin decision")

		default:
			body, _ := io.ReadAll(pollResp.Body)
			_ = pollResp.Body.Close()
			return "", fmt.Errorf("unexpected poll response (%d): %s", pollResp.StatusCode, strings.TrimSpace(string(body)))
		}
	}
}
