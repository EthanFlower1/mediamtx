package connect

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// StreamProxyConfig configures the stream proxy handler.
type StreamProxyConfig struct {
	Registry *Registry
	// TenantID is used for session lookup. In production this comes from
	// auth; for testing we accept it as config.
	TenantID string
	Logger   *slog.Logger
}

// StreamProxyHandler returns an http.Handler that proxies HLS requests
// to on-prem Recorders through the WSS tunnel.
//
// Route: /stream/{alias}/{path...}
// Example: /stream/ethans-home/nvr/cam-id/main/index.m3u8
func StreamProxyHandler(cfg StreamProxyConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse: /stream/{alias}/{rest...}
		path := strings.TrimPrefix(r.URL.Path, "/stream/")
		slashIdx := strings.Index(path, "/")
		if slashIdx < 0 {
			http.Error(w, `{"error":"path must be /stream/{alias}/{path}"}`, http.StatusBadRequest)
			return
		}

		alias := path[:slashIdx]
		streamPath := path[slashIdx:] // includes leading /

		// Look up the site.
		session, ok := cfg.Registry.LookupByAlias(cfg.TenantID, alias)
		if !ok {
			http.Error(w, `{"error":"site not found or offline"}`, http.StatusNotFound)
			return
		}

		if session.Conn == nil {
			http.Error(w, `{"error":"site has no active tunnel"}`, http.StatusBadGateway)
			return
		}

		// Build proxy request command.
		proxyReq := ProxyHTTPRequest{
			Method: r.Method,
			Path:   streamPath,
		}
		reqData, _ := json.Marshal(proxyReq)

		cmdID := randomID()
		respData, err := session.Conn.SendCommand(cmdID, "proxy_http", reqData, 15*time.Second)
		if err != nil {
			cfg.Logger.Error("proxy command failed", "error", err, "alias", alias, "path", streamPath)
			http.Error(w, fmt.Sprintf(`{"error":"proxy failed: %s"}`, err), http.StatusBadGateway)
			return
		}

		var proxyResp ProxyHTTPResponse
		if err := json.Unmarshal(respData, &proxyResp); err != nil {
			http.Error(w, `{"error":"invalid proxy response"}`, http.StatusBadGateway)
			return
		}

		body, err := proxyResp.DecodeBody()
		if err != nil {
			http.Error(w, `{"error":"decode body failed"}`, http.StatusBadGateway)
			return
		}

		// Set response headers.
		for k, v := range proxyResp.Header {
			w.Header().Set(k, v)
		}

		// CORS — allow any origin for remote viewing.
		w.Header().Set("Access-Control-Allow-Origin", "*")

		w.WriteHeader(proxyResp.StatusCode)
		w.Write(body)
	})
}

func randomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
