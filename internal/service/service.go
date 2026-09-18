// Package service owns process lifetime; all commands share this one listener.
package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gylive/ccodex-sleep-state/internal/codexconfig"
	"github.com/gylive/ccodex-sleep-state/internal/fsutil"
	"github.com/gylive/ccodex-sleep-state/internal/gateway"
	"github.com/gylive/ccodex-sleep-state/internal/instance"
	"github.com/gylive/ccodex-sleep-state/internal/logbook"
	"github.com/gylive/ccodex-sleep-state/internal/proxyroute"
	"github.com/gylive/ccodex-sleep-state/internal/settings"
	C "github.com/metacubex/mihomo/constant"
)

type Runtime struct {
	Address string `json:"address"`
	Token   string `json:"control_token"`
}

func Run(parent context.Context, dir string, c settings.Config, configure bool, out io.Writer) (result error) {
	if err := c.Validate(); err != nil {
		return err
	}
	lock, err := instance.Acquire(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return errors.New("cannot bind loopback port; another service may already be running")
	}
	defer listener.Close()
	logger, logs, err := logbook.Open(filepath.Join(dir, "logs"))
	if err != nil {
		return err
	}
	defer logs.Close()
	C.SetHomeDir(filepath.Join(dir, "core"))
	proxyroute.QuietCore()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	routes, err := proxyroute.Load(ctx, c)
	if err != nil {
		return err
	}
	engine := gateway.New(c, routes, logger)
	defer engine.Close()
	if configure {
		home, err := c.CodexDir()
		if err != nil {
			return err
		}
		if err = codexconfig.Install(dir, home, "http://"+c.Listen+"/backend-api/codex"); err != nil {
			return err
		}
		defer func() {
			if err := codexconfig.Restore(dir); err != nil {
				logger.Error("config_restore_conflict")
				result = errors.Join(result, err)
			}
		}()
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return err
	}
	runtime := Runtime{c.Listen, hex.EncodeToString(secret)}
	data, _ := json.Marshal(runtime)
	runtimePath := filepath.Join(dir, "runtime.json")
	if err = fsutil.Write(runtimePath, data); err != nil {
		return err
	}
	defer os.Remove(runtimePath)
	mux := http.NewServeMux()
	mux.HandleFunc("/_sleep/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+runtime.Token)) != 1 {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(engine.Status())
	})
	mux.Handle("/", engine)
	handler := gateway.ProtectLocal(c.Listen, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCtx, requestCancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer requestCancel()
		mux.ServeHTTP(w, r.WithContext(requestCtx))
	}))
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); engine.Run(ctx) }()
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	logger.Info("service_started", "routes", len(routes), "model", settings.Model, "configured_codex", configure)
	fmt.Fprintf(out, "Listening on http://%s\nAstra only. Restart Codex to load the local provider. Ctrl+C stops and restores configuration.\n", c.Listen)
	select {
	case err = <-stopped:
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.New("HTTP service stopped unexpectedly")
		}
	case <-parent.Done():
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err = server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
	}
	<-workerDone
	logger.Info("service_stopped")
	return result
}

func Status(ctx context.Context, dir string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, "runtime.json"))
	if err != nil {
		return nil, errors.New("service is not running (runtime file missing)")
	}
	var runtime Runtime
	if json.Unmarshal(data, &runtime) != nil {
		return nil, errors.New("invalid runtime file")
	}
	host, _, err := net.SplitHostPort(runtime.Address)
	if err != nil || !settings.IsLoopback(host) {
		return nil, errors.New("runtime address is not loopback")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+runtime.Address+"/_sleep/status", nil)
	if err != nil {
		return nil, errors.New("invalid runtime address")
	}
	req.Header.Set("Authorization", "Bearer "+runtime.Token)
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("service is not reachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("service status check rejected")
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}
