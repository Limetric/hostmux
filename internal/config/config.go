// Package config loads and watches the TOML config file used by the hostmux
// daemon. The config defines the TLS listen address, the Unix socket path,
// and a list of persistent hostname → upstream mappings. Files are
// hot-reloaded via fsnotify with a 200ms debounce so a multi-write save is
// coalesced into a single reload event.
package config

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"

	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/router"
)

const (
	DefaultTLSListen = ":8443"
	// DefaultDomain is the base domain used to expand bare host labels in config
	// when the domain field is omitted (e.g. "api" → "api.localhost").
	DefaultDomain = "localhost"
)

// Config is the parsed TOML config file.
type Config struct {
	Listen    string      `toml:"listen"`
	Socket    string      `toml:"socket"`
	Domain    string      `toml:"domain"`
	HidePort  bool        `toml:"hide_port"`
	AccessLog bool        `toml:"access_log"`
	LogFormat string      `toml:"log_format"`
	TLS       *TLSBlock   `toml:"tls"`
	Proxy     *ProxyBlock `toml:"proxy"`
	Apps      []App       `toml:"app"`
}

// Log format values accepted in `log_format`.
const (
	LogFormatText = "text"
	LogFormatJSON = "json"
)

// TLSBlock configures the TLS listener.
type TLSBlock struct {
	Listen string `toml:"listen"`
	Cert   string `toml:"cert"`
	Key    string `toml:"key"`
}

// ProxyBlock holds optional hardening knobs for the proxy edge. All fields
// default to zero, which preserves hostmux's prior behavior (Go's defaults):
// no server-side timeouts, the standard upstream transport, and TLS
// verification enabled for HTTPS upstreams.
type ProxyBlock struct {
	// ReadHeaderTimeout bounds how long the server waits for request
	// headers. Mitigates slow-header (Slowloris) clients. Server-side.
	ReadHeaderTimeout Duration `toml:"read_header_timeout"`
	// IdleTimeout bounds how long an idle keep-alive connection is kept
	// open. Server-side.
	IdleTimeout Duration `toml:"idle_timeout"`
	// ResponseHeaderTimeout bounds how long the proxy waits for an upstream
	// to start sending response headers before returning 504. Upstream
	// transport.
	ResponseHeaderTimeout Duration `toml:"response_header_timeout"`
	// DialTimeout bounds how long the proxy waits to establish a TCP
	// connection to the upstream. Upstream transport.
	DialTimeout Duration `toml:"dial_timeout"`
	// MaxHeaderBytes caps the size of request headers the server accepts.
	// Server-side. Zero uses Go's default (1 MiB).
	MaxHeaderBytes int `toml:"max_header_bytes"`
	// UpstreamInsecureSkipVerify disables TLS certificate verification for
	// HTTPS upstreams. Off by default; enable only for trusted local dev
	// servers that present self-signed certificates.
	UpstreamInsecureSkipVerify bool `toml:"upstream_insecure_skip_verify"`
}

// Duration is a time.Duration that decodes from a TOML string such as "5s"
// or "120s". A bare TOML integer is rejected to avoid ambiguity between
// seconds and nanoseconds.
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler so BurntSushi/toml can
// decode duration strings.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(strings.TrimSpace(string(text)))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	*d = Duration(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// AsDuration returns the value as a time.Duration.
func (d Duration) AsDuration() time.Duration { return time.Duration(d) }

// App is one persistent registration.
type App struct {
	Hosts    []string          `toml:"hosts"`
	Upstream string            `toml:"upstream"`
	Labels   map[string]string `toml:"labels"`
}

// Load parses the TOML file at path, applies defaults, and validates.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = DefaultTLSListen
	}
	if c.Domain == "" {
		c.Domain = DefaultDomain
	}
	if c.TLS != nil && c.TLS.Listen == "" {
		c.TLS.Listen = c.Listen
	}
}

func (c *Config) normalize() {
	c.Domain = hostnames.NormalizeDomain(c.Domain)
	for i := range c.Apps {
		c.Apps[i].Hosts = hostnames.Expand(c.Apps[i].Hosts, c.Domain)
	}
}

func (c *Config) validate() error {
	switch c.LogFormat {
	case "", LogFormatText, LogFormatJSON:
	default:
		return fmt.Errorf("config: log_format must be %q or %q, got %q", LogFormatText, LogFormatJSON, c.LogFormat)
	}
	if err := ValidateListenAddr(c.Listen); err != nil {
		return fmt.Errorf("config: listen: %w", err)
	}
	if c.TLS != nil && c.TLS.Listen != "" {
		if err := ValidateListenAddr(c.TLS.Listen); err != nil {
			return fmt.Errorf("config: tls.listen: %w", err)
		}
	}
	if c.Proxy != nil {
		p := c.Proxy
		for name, d := range map[string]Duration{
			"read_header_timeout":     p.ReadHeaderTimeout,
			"idle_timeout":            p.IdleTimeout,
			"response_header_timeout": p.ResponseHeaderTimeout,
			"dial_timeout":            p.DialTimeout,
		} {
			if d < 0 {
				return fmt.Errorf("config: proxy.%s must not be negative", name)
			}
		}
		if p.MaxHeaderBytes < 0 {
			return fmt.Errorf("config: proxy.max_header_bytes must not be negative")
		}
	}
	for i, app := range c.Apps {
		if len(app.Hosts) == 0 {
			return fmt.Errorf("config: app[%d]: hosts must be non-empty", i)
		}
		for j, host := range app.Hosts {
			if !hostnames.ValidHostToken(host) {
				return fmt.Errorf("config: app[%d]: hosts[%d]: must be a valid hostname", i, j)
			}
		}
		if app.Upstream == "" {
			return fmt.Errorf("config: app[%d]: upstream must be non-empty", i)
		}
		if err := validateUpstreamURL(app.Upstream); err != nil {
			return fmt.Errorf("config: app[%d]: %w", i, err)
		}
	}
	return nil
}

// ValidateListenAddr checks that addr is a valid "host:port" TCP listen
// address such as ":8443", ":443", "127.0.0.1:8443", or "[::1]:443". The
// host part may be empty (bind all interfaces). The port must be a number in
// 0–65535 (0 means OS-assigned). Exposed so `hostmux config check` and
// `hostmux doctor` can reuse the same rule the daemon enforces at load.
func ValidateListenAddr(addr string) error {
	if addr == "" {
		return fmt.Errorf("must not be empty")
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("must be host:port (e.g. \":8443\"): %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("port %q must be numeric", portStr)
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("port %d out of range 0-65535", port)
	}
	return nil
}

func validateUpstreamURL(raw string) error {
	if strings.TrimSpace(raw) != raw {
		return fmt.Errorf("upstream must not contain surrounding whitespace")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("upstream must be a valid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Host == "" || (scheme != "http" && scheme != "https") {
		return fmt.Errorf("upstream must be an absolute http or https URL")
	}
	return nil
}

// RouterEntries converts the parsed apps into router.Entry values ready for
// router.ReplaceSource("config", ...).
func (c *Config) RouterEntries() []router.Entry {
	out := make([]router.Entry, 0, len(c.Apps))
	for _, app := range c.Apps {
		out = append(out, router.Entry{
			Source:   "config",
			Hosts:    append([]string(nil), app.Hosts...),
			Upstream: app.Upstream,
			Labels:   app.Labels,
		})
	}
	return out
}

// Watcher reloads the config file on disk changes and pushes new Configs onto
// a channel. Reload events are debounced (200ms) so a multi-write save shows
// up as one event.
type Watcher struct {
	path string
	w    *fsnotify.Watcher

	mu      sync.Mutex
	current *Config
}

// NewWatcher starts a fsnotify watch on path and returns a Watcher with the
// initial Config already loaded. The caller should call Run on a goroutine.
func NewWatcher(path string) (*Watcher, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(path); err != nil {
		w.Close()
		return nil, err
	}
	return &Watcher{path: path, w: w, current: cfg}, nil
}

// Current returns the most recently loaded Config.
func (w *Watcher) Current() *Config {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current
}

// Run watches for file changes until ctx is cancelled. For each successful
// reload it calls onReload(newConfig). For each rejected reload it calls
// onError(err). Both callbacks may be nil.
func (w *Watcher) Run(ctx context.Context, onReload func(*Config), onError func(error)) {
	defer w.w.Close()
	var debounce *time.Timer
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(200*time.Millisecond, func() {
				cfg, err := Load(w.path)
				if err != nil {
					if onError != nil {
						onError(err)
					}
					return
				}
				w.mu.Lock()
				w.current = cfg
				w.mu.Unlock()
				// Re-add the watch — atomic-save editors (vim, IntelliJ) replace the
				// file via rename, which detaches the inotify/kqueue watch from the
				// original inode. Re-adding the path ensures we keep getting events
				// for subsequent saves.
				_ = w.w.Remove(w.path)
				_ = w.w.Add(w.path)
				if onReload != nil {
					onReload(cfg)
				}
			})
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			if onError != nil {
				onError(err)
			}
		}
	}
}
