// Package settings owns paths and validation, not network or process state.
package settings

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const Model = "gpt-6-astra"
const App = "ccodex-sleep-state"

type Source struct {
	URL              string   `json:"url,omitempty"`
	URLEnv           string   `json:"url_env,omitempty"`
	UserAgent        string   `json:"user_agent,omitempty"`
	IncludeProtocols []string `json:"include_protocols,omitempty"`
	ExcludeKeywords  []string `json:"exclude_keywords,omitempty"`
}

type Config struct {
	Listen               string   `json:"listen"`
	Upstream             string   `json:"upstream"`
	CodexHome            string   `json:"codex_home,omitempty"`
	Direct               bool     `json:"direct"`
	ProxyURLs            []string `json:"proxy_urls"`
	ProxyEnvs            []string `json:"proxy_envs"`
	Subscriptions        []Source `json:"subscriptions"`
	SubscriptionProxyEnv string   `json:"subscription_proxy_env,omitempty"`
	ProbeSeconds         int      `json:"probe_timeout_seconds"`
	RefreshSeconds       int      `json:"refresh_before_seconds"`
	CooldownSeconds      int      `json:"probe_cooldown_seconds"`
	MaxProbes            int      `json:"max_probes_per_round"`
	TTLSeconds           int      `json:"state_ttl_seconds"`
	BaselineBlocks       int      `json:"baseline_blocks"`
}

func Default() Config {
	return Config{Listen: "127.0.0.1:17841", Upstream: "https://chatgpt.com/backend-api/codex", Direct: true,
		ProxyURLs: []string{}, ProxyEnvs: []string{}, Subscriptions: []Source{}, ProbeSeconds: 20,
		RefreshSeconds: 1200, CooldownSeconds: 180, MaxProbes: 6, TTLSeconds: 3600, BaselineBlocks: 10}
}

func Load(path string) (Config, error) {
	c := Default()
	f, err := os.Open(path)
	if err != nil {
		return c, errors.New("cannot open service config; run init first")
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 1<<20))
	dec.DisallowUnknownFields()
	if dec.Decode(&c) != nil {
		return c, errors.New("invalid service config JSON (unknown fields are rejected)")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return c, errors.New("service config contains trailing data")
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	host, port, err := net.SplitHostPort(c.Listen)
	ip := net.ParseIP(host)
	p, pe := strconv.Atoi(port)
	if err != nil || ip == nil || !ip.IsLoopback() || pe != nil || p < 1 || p > 65535 {
		return errors.New("listen must be a literal loopback IP and port (1–65535)")
	}
	u, err := url.Parse(c.Upstream)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid upstream URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && IsLoopback(u.Hostname())) {
		return errors.New("upstream must use HTTPS; HTTP is allowed only for a loopback test server")
	}
	if strings.TrimRight(u.Path, "/") != "/backend-api/codex" {
		return errors.New("upstream path must be /backend-api/codex")
	}
	if c.ProbeSeconds < 1 || c.ProbeSeconds > 60 || c.MaxProbes < 1 || c.MaxProbes > 20 || c.CooldownSeconds < 30 || c.CooldownSeconds > 3600 {
		return errors.New("probe limits: timeout 1–60s, round 1–20 requests, cooldown 30–3600s")
	}
	if c.TTLSeconds < 120 || c.TTLSeconds > 3600 || c.RefreshSeconds < 30 || c.RefreshSeconds >= c.TTLSeconds || c.BaselineBlocks < 1 || c.BaselineBlocks > 32 {
		return errors.New("invalid state policy; refresh must be shorter than TTL (120–3600s)")
	}
	for _, source := range c.Subscriptions {
		if len(source.IncludeProtocols) > 16 || len(source.ExcludeKeywords) > 64 {
			return errors.New("too many subscription filter entries")
		}
		for _, keyword := range source.ExcludeKeywords {
			if strings.TrimSpace(keyword) == "" || len(keyword) > 128 {
				return errors.New("subscription exclude keyword must contain 1–128 bytes")
			}
		}
		if len(source.UserAgent) > 256 || strings.ContainsAny(source.UserAgent, "\r\n") {
			return errors.New("subscription user_agent must be one line, at most 256 bytes")
		}
	}
	if len(c.ProxyURLs)+len(c.ProxyEnvs) > 256 || len(c.Subscriptions) > 16 {
		return errors.New("too many proxy sources")
	}
	return nil
}

func IsLoopback(host string) bool { ip := net.ParseIP(host); return ip != nil && ip.IsLoopback() }

func DataDir() (string, error) {
	if p := os.Getenv("CCODEX_STATE_HOME"); p != "" {
		return filepath.Abs(p)
	}
	if runtime.GOOS == "windows" {
		if p := os.Getenv("LOCALAPPDATA"); p != "" {
			return filepath.Join(p, App), nil
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, App), nil
}

func (c Config) CodexDir() (string, error) {
	if c.CodexHome != "" {
		return filepath.Abs(c.CodexHome)
	}
	if p := os.Getenv("CODEX_HOME"); p != "" {
		return filepath.Abs(p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}
