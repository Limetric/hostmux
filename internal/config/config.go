// Package config loads and watches the TOML config file used by the hostmux
// daemon. The config defines the TLS listen address, the Unix socket path,
// and a list of persistent hostname → upstream mappings. Files are
// hot-reloaded via fsnotify with a 200ms debounce so a multi-write save is
// coalesced into a single reload event.
package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"

	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/router"
)

const DefaultTLSListen = ":8443"

// Config is the parsed TOML config file.
type Config struct {
	Listen string    `toml:"listen"`
	Socket string    `toml:"socket"`
	Domain string    `toml:"domain"`
	TLS    *TLSBlock `toml:"tls"`
	Apps   []App     `toml:"app"`
}

// TLSBlock configures the TLS listener.
type TLSBlock struct {
	Listen string `toml:"listen"`
	Cert   string `toml:"cert"`
	Key    string `toml:"key"`
}

// App is one persistent registration.
type App struct {
	Hosts    []string `toml:"hosts"`
	Upstream string   `toml:"upstream"`
}

// Load parses the TOML file at path, applies defaults, and validates.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = DefaultTLSListen
	}
	if c.TLS != nil && c.TLS.Listen == "" {
		c.TLS.Listen = c.Listen
	}
}

func (c *Config) normalize() error {
	c.Domain = hostnames.NormalizeDomain(c.Domain)
	for i := range c.Apps {
		expanded, err := hostnames.Expand(c.Apps[i].Hosts, c.Domain)
		if err != nil {
			return fmt.Errorf("config: app[%d]: %w", i, err)
		}
		c.Apps[i].Hosts = expanded
	}
	return nil
}

func (c *Config) validate() error {
	for i, app := range c.Apps {
		if len(app.Hosts) == 0 {
			return fmt.Errorf("config: app[%d]: hosts must be non-empty", i)
		}
		if app.Upstream == "" {
			return fmt.Errorf("config: app[%d]: upstream must be non-empty", i)
		}
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
