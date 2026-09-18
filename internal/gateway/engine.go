// Package gateway joins immutable request snapshots with a bounded probe loop.
// Authentication is borrowed from incoming Codex requests and kept only in RAM.
package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gylive/ccodex-sleep-state/internal/proxyroute"
	"github.com/gylive/ccodex-sleep-state/internal/settings"
	"github.com/gylive/ccodex-sleep-state/internal/turnstate"
)

const idleLifetime = 30 * time.Minute

type session struct {
	mu                  sync.Mutex
	headers             http.Header
	state               *turnstate.Store
	lastUsed, nextProbe time.Time
	busy                int
	activated           bool
	blocked             bool
	cursor              int
	probing             chan struct{}
}

type Engine struct {
	config    settings.Config
	routes    []proxyroute.Route
	log       *slog.Logger
	mu        sync.Mutex
	sessions  map[string]*session
	probeSlot chan struct{}
}

func New(c settings.Config, routes []proxyroute.Route, logger *slog.Logger) *Engine {
	return &Engine{config: c, routes: routes, log: logger, sessions: make(map[string]*session), probeSlot: make(chan struct{}, 1)}
}
func (e *Engine) borrow(h http.Header) (*session, error) {
	auth := h.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))) < 8 || len(auth) > 16384 {
		return nil, errors.New("bearer authentication required")
	}
	sum := sha256.Sum256([]byte(auth + "\x00" + h.Get("ChatGPT-Account-Id")))
	key := hex.EncodeToString(sum[:])
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	for k, s := range e.sessions {
		s.mu.Lock()
		expired := s.busy == 0 && s.probing == nil && now.Sub(s.lastUsed) > idleLifetime
		s.mu.Unlock()
		if expired {
			delete(e.sessions, k)
		}
	}
	s := e.sessions[key]
	if s == nil {
		if len(e.sessions) >= 8 {
			return nil, errors.New("too many active credentials")
		}
		safe := make(http.Header)
		for _, k := range []string{"Authorization", "ChatGPT-Account-Id", "User-Agent", "Version", "Originator", "OpenAI-Beta"} {
			if v := h.Get(k); v != "" {
				safe.Set(k, v)
			}
		}
		s = &session{headers: safe, state: turnstate.New(turnstate.Policy{Blocks: e.config.BaselineBlocks, TTL: time.Duration(e.config.TTLSeconds) * time.Second, Refresh: time.Duration(e.config.RefreshSeconds) * time.Second})}
		e.sessions[key] = s
	}
	s.mu.Lock()
	s.lastUsed = now
	s.busy++
	s.mu.Unlock()
	return s, nil
}
func release(s *session) { s.mu.Lock(); s.busy--; s.mu.Unlock() }

// Run does no work until a real Codex request supplies credentials. Idle
// credentials expire; no auth file is imported and no OAuth refresh is owned.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			e.mu.Lock()
			var work []*session
			for key, s := range e.sessions {
				s.mu.Lock()
				idle := now.Sub(s.lastUsed) > idleLifetime
				available := s.busy == 0 && s.probing == nil
				activated := s.activated
				s.mu.Unlock()
				if idle && available {
					delete(e.sessions, key)
					continue
				}
				if !idle && available && activated {
					work = append(work, s)
				}
			}
			e.mu.Unlock()
			for _, s := range work {
				if s.state.NeedsRefresh(now) {
					e.refresh(ctx, s)
				}
			}
		}
	}
}

func (e *Engine) refresh(ctx context.Context, s *session) {
	s.mu.Lock()
	if pending := s.probing; pending != nil {
		s.mu.Unlock()
		select {
		case <-pending:
		case <-ctx.Done():
		}
		return
	}
	if s.blocked || time.Now().Before(s.nextProbe) {
		s.mu.Unlock()
		return
	}
	done := make(chan struct{})
	s.probing = done
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		cooldown := time.Now().Add(time.Duration(e.config.CooldownSeconds) * time.Second)
		if cooldown.After(s.nextProbe) {
			s.nextProbe = cooldown
		}
		s.probing = nil
		close(done)
		s.mu.Unlock()
	}()
	select {
	case e.probeSlot <- struct{}{}:
		defer func() { <-e.probeSlot }()
	case <-ctx.Done():
		return
	}
	successes := 0
	for i := 0; i < e.config.MaxProbes && i < len(e.routes); i++ {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		route := s.cursor % len(e.routes)
		s.cursor++
		s.mu.Unlock()
		token, status, retryAfter, err := e.probe(ctx, s.headers, e.routes[route])
		accepted := false
		if err == nil {
			accepted = s.state.Offer(token, route, time.Now())
		}
		e.log.Info("probe_finished", "route", e.routes[route].ID, "status", status, "accepted", accepted)
		// Account and quota errors are not an invitation to try more IP addresses.
		if status == 401 || status == 403 || status == 429 {
			s.mu.Lock()
			s.cursor-- // An account rejection must not cause the next round to switch IPs.
			s.blocked = status == 401 || status == 403
			if retryAfter > 0 {
				s.nextProbe = time.Now().Add(retryAfter)
			}
			s.mu.Unlock()
			return
		}
		if accepted {
			successes++
			if successes >= 2 {
				return
			}
		}
	}
}

func (e *Engine) probe(parent context.Context, h http.Header, route proxyroute.Route) (turnstate.Token, int, time.Duration, error) {
	ctx, cancel := context.WithTimeout(parent, time.Duration(e.config.ProbeSeconds)*time.Second)
	defer cancel()
	body := map[string]any{"model": settings.Model, "instructions": "Reply with OK.", "input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Reply with OK."}}}}, "stream": true, "store": false, "reasoning": map[string]string{"effort": "low"}}
	encoded, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(e.config.Upstream, "/")+"/responses", bytes.NewReader(encoded))
	req.Header = h.Clone()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	// A probe has no prior state and no conversation/session chain.
	client := &http.Client{Transport: route.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return turnstate.Token{}, 0, 0, errors.New("probe transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return turnstate.Token{}, resp.StatusCode, retryDelay(resp.Header.Get("Retry-After")), errors.New("probe upstream rejected")
	}
	token, err := turnstate.Parse(resp.Header.Get(turnstate.Header))
	if err != nil {
		return turnstate.Token{}, resp.StatusCode, 0, err
	}
	// Drain this tiny response, bounded by both bytes and time. Failed SSE
	// events cannot quietly count as a successful probe.
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || !completed(data) {
		return turnstate.Token{}, resp.StatusCode, retryDelay(resp.Header.Get("Retry-After")), errors.New("probe did not complete")
	}
	return token, resp.StatusCode, 0, nil
}
func completed(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(bytes.TrimSpace(line[5:]), &event) == nil && event.Type == "response.completed" {
			return true
		}
	}
	return false
}

func (e *Engine) Status() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	states := make([]turnstate.Status, 0, len(e.sessions))
	for _, s := range e.sessions {
		states = append(states, s.state.Status(time.Now()))
	}
	return map[string]any{"model": settings.Model, "routes": len(e.routes), "sessions": states, "state_storage": "memory", "transport": "http-sse"}
}
func (e *Engine) Close() {
	for _, r := range e.routes {
		r.Close()
	}
}

func retryDelay(value string) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := time.Until(at); delay > 0 {
			return delay
		}
	}
	return 0
}
