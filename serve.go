package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/listener"
	"github.com/Limetric/hostmux/internal/proxy"
	"github.com/Limetric/hostmux/internal/router"
	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/sockserver"
)

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to TOML config file (optional)")
	socketFlag := fs.String("socket", "", "override Unix socket path")
	fs.Parse(args)

	r := router.New()

	// Optional config file. If not provided, look in standard locations.
	resolvedConfigPath := *configPath
	if resolvedConfigPath == "" {
		resolvedConfigPath = defaultConfigPath()
	}
	var cfg *config.Config
	var watcher *config.Watcher
	if resolvedConfigPath != "" {
		if _, err := os.Stat(resolvedConfigPath); err == nil {
			w, err := config.NewWatcher(resolvedConfigPath)
			if err != nil {
				log.Printf("config: %v", err)
				return 1
			}
			watcher = w
			cfg = w.Current()
			if err := r.ReplaceSource("config", cfg.RouterEntries()); err != nil {
				log.Printf("config: initial load rejected: %v", err)
				return 1
			}
			log.Printf("config: loaded %s (%d apps)", resolvedConfigPath, len(cfg.Apps))
		}
	}

	// Resolve listen addresses.
	plain := ":8080"
	var tlsCfg *listener.TLSConfig
	configSocket := ""
	if cfg != nil {
		if cfg.Listen != "" {
			plain = cfg.Listen
		}
		if cfg.TLS != nil {
			tlsCfg = &listener.TLSConfig{
				Listen:   cfg.TLS.Listen,
				CertFile: cfg.TLS.Cert,
				KeyFile:  cfg.TLS.Key,
			}
		}
		configSocket = cfg.Socket
	}

	// Resolve socket path.
	sockPath, err := sockpath.ResolveServe(sockpath.Options{
		Flag:         *socketFlag,
		ConfigSocket: configSocket,
	})
	if err != nil {
		log.Printf("sockpath: %v", err)
		return 1
	}
	if dir := filepath.Dir(sockPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	// Acquire the PID-file flock for this socket path. The PID file lives
	// next to the socket so daemons on different sockets coexist; the flock
	// makes "two daemons on the same socket" detect each other and the
	// loser exits cleanly with no error.
	pidPath := sockpath.PIDFilePathFor(sockPath)
	pidLock, contention, err := acquirePIDLock(pidPath)
	if err != nil {
		log.Printf("hostmux serve: pid lock: %v", err)
		return 1
	}
	if contention {
		log.Printf("hostmux serve: another daemon already serving %s (pid file: %s); exiting", sockPath, pidPath)
		return 0
	}
	defer pidLock.Close()

	// HTTP listeners.
	handler := proxy.New(r)
	servers, err := listener.Build(listener.Config{Plain: plain, TLS: tlsCfg}, handler)
	if err != nil {
		log.Printf("listener: %v", err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start HTTP servers.
	for i, srv := range servers {
		srv := srv
		isTLS := i == 1 && tlsCfg != nil
		go func() {
			var serr error
			if isTLS {
				serr = srv.ListenAndServeTLS(tlsCfg.CertFile, tlsCfg.KeyFile)
			} else {
				serr = srv.ListenAndServe()
			}
			if serr != nil && serr != http.ErrServerClosed {
				log.Printf("http server: %v", serr)
				cancel()
			}
		}()
	}
	log.Printf("hostmux serve: HTTP listening on %s", plain)
	if tlsCfg != nil {
		log.Printf("hostmux serve: TLS listening on %s", tlsCfg.Listen)
	}

	// Unix socket server.
	sockSrv := sockserver.New(r)
	if err := sockSrv.Listen(sockPath); err != nil {
		log.Printf("sockserver: %v", err)
		return 1
	}
	go sockSrv.Serve()
	log.Printf("hostmux serve: socket listening on %s", sockPath)

	// Discovery file (PID file is already written under the flock above).
	if err := sockpath.WriteDiscovery(sockPath); err != nil {
		log.Printf("sockpath: write discovery: %v", err)
	}

	// Config watcher.
	if watcher != nil {
		go watcher.Run(ctx,
			func(c *config.Config) {
				if err := r.ReplaceSource("config", c.RouterEntries()); err != nil {
					log.Printf("config: reload rejected: %v", err)
					return
				}
				log.Printf("config: reloaded (%d apps)", len(c.Apps))
			},
			func(err error) { log.Printf("config: %v", err) },
		)
	}

	// Wait for signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
	case <-ctx.Done():
	}
	log.Printf("hostmux serve: shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdownCtx)
	}
	_ = sockSrv.Close()
	_ = sockpath.RemoveDiscovery()
	// We deliberately do NOT delete the PID file. The flock is released
	// automatically when pidLock.Close() runs, and leaving the file in
	// place avoids a race window where a second daemon could start, see no
	// file, create one, and acquire its lock — all between our os.Remove
	// and our process exit. The next daemon will simply truncate and
	// rewrite it under its own flock.
	return 0
}

func defaultConfigPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "hostmux", "hostmux.toml")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "hostmux", "hostmux.toml")
	}
	return ""
}

// acquirePIDLock attempts to take an exclusive flock on the PID file. It
// returns the open file (which the caller MUST keep open for the duration
// of the daemon — closing it releases the lock), and a contention bool
// that is true when another process already holds the lock.
//
// The flock is advisory and held by the file descriptor, so it is released
// automatically when the process exits even on SIGKILL. The PID file
// itself is left in place across daemon restarts; subsequent daemons
// truncate and rewrite it under their own flock.
func acquirePIDLock(path string) (*os.File, bool, error) {
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, fmt.Errorf("open pid file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("flock pid file: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		f.Close()
		return nil, false, fmt.Errorf("truncate pid file: %w", err)
	}
	if _, err := f.WriteAt([]byte(fmt.Sprintf("%d\n", os.Getpid())), 0); err != nil {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		f.Close()
		return nil, false, fmt.Errorf("write pid file: %w", err)
	}
	return f, false, nil
}
