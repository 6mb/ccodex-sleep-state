package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gylive/ccodex-sleep-state/internal/codexconfig"
	"github.com/gylive/ccodex-sleep-state/internal/fsutil"
	"github.com/gylive/ccodex-sleep-state/internal/gateway"
	"github.com/gylive/ccodex-sleep-state/internal/proxyroute"
	"github.com/gylive/ccodex-sleep-state/internal/settings"
)

// control owns a whole route generation. Reconfiguration never mutates routes
// under an active request, and never discards an account's upstream rejection.
type control struct {
	targetURL, targetKind string
	authMode, codexHome   string

	mu                     sync.RWMutex
	action                 sync.Mutex
	ctx                    context.Context
	config                 settings.Config
	path, dir              string
	configure, managed     bool
	setupError, routeError string
	engine                 *gateway.Engine
	cancel                 context.CancelFunc
	done                   chan struct{}
	log                    *slog.Logger
}

func (c *control) start(routes []proxyroute.Route) {
	selected := routes
	if c.config.PinnedRoute != "" {
		selected = nil
		for _, r := range routes {
			if r.ID == c.config.PinnedRoute {
				selected = append(selected, r)
			} else {
				r.Close()
			}
		}
	}
	if len(selected) == 0 {
		c.routeError = "固定出口已不存在，请在路由页切回自动选择。"
		return
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.cancel, c.done = cancel, make(chan struct{})
	effective := c.effective()
	c.engine = gateway.New(effective, selected, c.log)
	engine, done := c.engine, c.done
	go func() { defer close(done); engine.Run(ctx) }()
	c.routeError = ""
}
func (c *control) pause() {
	if c.cancel != nil {
		c.cancel()
		<-c.done
		c.cancel = nil
		c.done = nil
	}
}
func (c *control) resume() {
	if c.engine == nil || c.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	engine, done := c.engine, c.done
	go func() { defer close(done); engine.Run(ctx) }()
}
func (c *control) stop() {
	c.pause()
	if c.engine != nil {
		c.engine.Close()
		c.engine = nil
	}
}
func (c *control) effective() settings.Config {
	next := c.config
	if c.targetURL != "" {
		next.Upstream = c.targetURL
		next.UpstreamKind = c.targetKind
	}
	if next.IsRelay() {
		next.InjectionDisabled = true
	}
	return next
}
func (c *control) setup() {
	if !c.configure {
		return
	}
	c.managed = false
	err := codexconfig.Restore(c.dir)
	if err == nil {
		err = c.installProvider()
	}
	if err != nil {
		c.setupError = "Codex 配置未接管。" + err.Error() + " 原文件不会被强制覆盖；请检查所选 provider、认证方式或恢复备份。"
		return
	}
	c.managed, c.setupError = true, ""
}
func (c *control) installProvider() error {
	home, err := c.config.CodexDir()
	if err != nil {
		return errors.New("无法确定 Codex 配置目录")
	}
	original, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil && !os.IsNotExist(err) {
		return errors.New("无法读取 Codex 配置")
	}
	authMode, err := codexconfig.ReadAuthMode(home)
	if err != nil {
		return err
	}
	selected, err := codexconfig.ResolveWithAuth(original, c.config.CodexProfile, authMode)
	if err != nil {
		return err
	}
	next := c.config
	if next.UpstreamMode != "manual" {
		next.Upstream = selected.Upstream
		next.UpstreamKind = "relay"
		if selected.Official && selected.AuthKind == "chatgpt" {
			next.UpstreamKind = "official"
		}
	} else if next.IsRelay() && selected.AuthKind != "api_key" {
		return errors.New("手动中转上游不能接管官方 ChatGPT 登录，请选择使用 API key 的 provider")
	}
	if err = next.Validate(); err != nil {
		return err
	}
	options := codexconfig.Options{Profile: c.config.CodexProfile, AuthMode: authMode, ExpectedConfigSHA256: selected.ConfigSHA256}
	if err = codexconfig.InstallWithOptions(c.dir, home, "http://"+c.config.Listen+"/backend-api/codex", options); err != nil {
		return err
	}
	c.targetURL, c.targetKind = next.Upstream, next.UpstreamKind
	c.authMode, c.codexHome = authMode, home
	return nil
}
func (c *control) checkManaged() error {
	if !c.managed {
		return nil
	}
	if err := codexconfig.CheckManaged(c.dir); err != nil {
		return err
	}
	mode, err := codexconfig.ReadAuthMode(c.codexHome)
	if err != nil || mode != c.authMode {
		return errors.New("Codex 认证方式已改变，请停止服务并重新接入")
	}
	return nil
}
func (c *control) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.managed {
		if err := c.checkManaged(); err != nil {
			reply(w, 409, map[string]string{"error": "codex_config_changed", "message": "Codex 配置已被 CCS 或其他程序修改。请停止服务，确认所选配置后重新启动；不会覆盖你的改动。"})
			return
		}
	}
	if c.engine == nil || c.setupError != "" {
		reply(w, 503, map[string]string{"error": "service_not_ready", "message": "请打开本地管理面板，处理出口配置或 Codex 配置恢复问题。"})
		return
	}
	c.engine.ServeHTTP(w, r)
}
func (c *control) status() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := map[string]any{"model": settings.Model, "routes": 0, "sessions": []any{}}
	if c.engine != nil {
		result = c.engine.Status()
	}
	result["injection_enabled"] = !c.effective().InjectionDisabled
	result["upstream_kind"] = c.effective().UpstreamKind
	result["configured_codex"], result["config_error"], result["route_error"] = c.managed, c.setupError, c.routeError
	if err := c.checkManaged(); err != nil {
		result["config_error"] = "Codex 配置或认证方式已被外部修改。请先停止服务，再确认 CCS 所选配置并重新启动。"
		result["configured_codex"] = false
	}
	result["pinned_route"] = c.config.PinnedRoute
	result["sources"] = map[string]any{"direct": c.config.Direct, "proxies": len(c.config.ProxyURLs) + len(c.config.ProxyEnvs), "subscriptions": len(c.config.Subscriptions)}
	return result
}
func (c *control) persist(next settings.Config) error {
	// Keep an independent backup and don't overwrite edits made outside the UI.
	if data, err := os.ReadFile(c.path); err == nil {
		previous, err := settings.Load(c.path)
		if err != nil {
			return errors.New("配置文件已在外部改动或损坏，请先修复并重启服务")
		}
		a, _ := json.Marshal(previous)
		b, _ := json.Marshal(c.config)
		if string(a) != string(b) {
			return errors.New("配置文件已在外部修改。为避免覆盖，请重启服务后再保存")
		}
		if err := fsutil.Write(filepath.Join(c.dir, "backups", "config-"+time.Now().UTC().Format("20060102T150405.000000000")+".json"), data); err != nil {
			return errors.New("无法创建配置备份，未保存")
		}
	} else if !os.IsNotExist(err) {
		return errors.New("无法读取原配置，未保存")
	}
	data, _ := json.MarshalIndent(next, "", "  ")
	return fsutil.Write(c.path, append(data, '\n'))
}
