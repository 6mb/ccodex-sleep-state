package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gylive/ccodex-sleep-state/internal/settings"
	"github.com/gylive/ccodex-sleep-state/internal/turnstate"
)

const maxRequestBytes = 16 << 20

var errShape = errors.New("upstream state outside configured baseline")

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") != "" {
		fail(w, http.StatusUpgradeRequired, "http_sse_required", "This provider uses HTTP/SSE, not WebSocket.")
		return
	}
	generation := r.Method == http.MethodPost && (r.URL.Path == "/backend-api/codex/responses" || r.URL.Path == "/backend-api/codex/responses/compact")
	passthrough := (r.Method == http.MethodGet && r.URL.Path == "/backend-api/codex/models") || (r.Method == http.MethodPost && r.URL.Path == "/backend-api/codex/alpha/search")
	if !generation && !passthrough {
		fail(w, http.StatusNotFound, "unsupported_endpoint", "Endpoint is not exposed by this service.")
		return
	}
	s, err := e.borrow(r.Header)
	if err != nil {
		fail(w, http.StatusUnauthorized, "authentication_required", "Log in with Codex before using this service.")
		return
	}
	defer release(s)
	if r.Method == http.MethodPost {
		if encoding := r.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
			fail(w, 415, "unsupported_encoding", "Send uncompressed JSON.")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
		r.Body.Close()
		if err != nil {
			fail(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body exceeds the limit or could not be read.")
			return
		}
		if generation {
			var input struct {
				Model string `json:"model"`
			}
			if json.Unmarshal(body, &input) != nil {
				fail(w, 400, "invalid_json", "Expected a JSON request.")
				return
			}
			if input.Model != settings.Model {
				fail(w, 400, "unsupported_model", "Only gpt-6-astra is supported.")
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
	}
	if generation {
		s.mu.Lock()
		s.activated = true
		s.mu.Unlock()
	}
	snapshot, usable := s.state.Acquire(time.Now())
	if generation && !usable {
		e.refresh(r.Context(), s)
		snapshot, usable = s.state.Acquire(time.Now())
	}
	if generation && !usable {
		w.Header().Set("Retry-After", "30")
		fail(w, 503, "state_unavailable", "No usable state. Check service status and proxy connectivity; probes are rate-limited.")
		return
	}
	route := 0
	if usable {
		route = snapshot.Route
	}
	target, _ := url.Parse(e.config.Upstream)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			pr.Out.Host = target.Host
			pr.Out.Header.Del(turnstate.Header)
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Proxy-Authorization")
			if generation {
				pr.Out.Header.Set(turnstate.Header, snapshot.Token.Value)
			}
		},
		Transport: e.routes[route].Transport, FlushInterval: -1, ErrorLog: log.New(io.Discard, "", 0),
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			if generation && resp.StatusCode >= 200 && resp.StatusCode < 300 && s.state.Observe(resp.Header.Get(turnstate.Header), snapshot, time.Now()) {
				// Never replay a generation request: it may already have run upstream.
				resp.Body.Close()
				return errShape
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, errShape) {
				fail(w, 503, "state_shape_changed", "Upstream state changed shape. Request was not replayed; check status before retrying.")
				return
			}
			fail(w, 502, "upstream_unavailable", "Upstream connection failed. Request was not replayed.")
		},
	}
	started := time.Now()
	tracked := &statusWriter{ResponseWriter: w, status: 200}
	defer func() {
		e.log.Info("request_finished", "status", tracked.status, "duration_ms", time.Since(started).Milliseconds(), "route", e.routes[route].ID, "state_version", snapshot.Version)
	}()
	proxy.ServeHTTP(tracked, r)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": "sleep_state_error", "code": code, "message": message}})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(status int) {
	if !w.wrote {
		w.status = status
		w.wrote = true
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
func (w *statusWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// ProtectLocal rejects browser-origin requests and DNS-rebinding hostnames.
// Management endpoints additionally require a separate random local token.
func ProtectLocal(host string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, host) || r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			fail(w, 403, "local_clients_only", "Browser and non-loopback host requests are not accepted.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
