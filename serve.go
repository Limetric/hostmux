package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/daemon"
	"github.com/Limetric/hostmux/internal/daemonctl"
	"github.com/Limetric/hostmux/internal/listener"
	"github.com/Limetric/hostmux/internal/proxy"
	"github.com/Limetric/hostmux/internal/router"
	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/sockserver"
	"github.com/Limetric/hostmux/internal/tlsconfig"
)

func cmdStart(args []string) int {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "path to TOML config file (optional)")
	socketFlag := fs.String("socket", "", "override Unix socket path")
	forceFlag := fs.Bool("force", false, "stop any existing daemon on this socket and take over")
	foregroundFlag := fs.Bool("foreground", false, "run in the foreground instead of daemonizing")
	fs.Parse(args)

	if *foregroundFlag {
		return runForegroundDaemon("start", *configPath, *socketFlag, *forceFlag)
	}

	sockPath, err := resolveServeSocketPath(*configPath, *socketFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux start: %v\n", err)
		return 1
	}

	spawnArgs := startForegroundArgs(*configPath, *socketFlag, *forceFlag)
	if !*forceFlag {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := daemon.EnsureRunning(ctx, sockPath, daemon.EnsureOpts{
			Spawn: func() error { return daemon.SpawnDetached(spawnArgs...) },
		})
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "hostmux start: could not start daemon: %v\n", err)
			return 1
		}
		return 0
	}

	if err := startForcedDetachedDaemon(sockPath, spawnArgs); err != nil {
		fmt.Fprintf(os.Stderr, "hostmux start: could not start daemon: %v\n", err)
		return 1
	}
	return 0
}

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to TOML config file (optional)")
	socketFlag := fs.String("socket", "", "override Unix socket path")
	forceFlag := fs.Bool("force", false, "stop any existing daemon on this socket and take over")
	fs.Parse(args)
	return runForegroundDaemon("serve", *configPath, *socketFlag, *forceFlag)
}

func runForegroundDaemon(name, configPath, socketFlag string, force bool) int {
	r := router.New()

	// Optional config file. If not provided, look in standard locations.
	resolvedConfigPath := resolveConfigPath(configPath)
	cfg, err := loadOptionalConfig(resolvedConfigPath)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	var watcher *config.Watcher
	if cfg != nil {
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

	// Resolve listen address and TLS material.
	var tlsBlock *config.TLSBlock
	configSocket := ""
	var currentDomain atomic.Value
	currentDomain.Store("")
	if cfg != nil {
		if cfg.TLS != nil {
			block := *cfg.TLS
			if block.Listen == "" && cfg.Listen != "" {
				block.Listen = cfg.Listen
			}
			tlsBlock = &block
		} else if cfg.Listen != "" {
			tlsBlock = &config.TLSBlock{Listen: cfg.Listen}
		}
		configSocket = cfg.Socket
		currentDomain.Store(cfg.Domain)
	}
	effectiveTLS, err := tlsconfig.Resolve(tlsBlock)
	if err != nil {
		log.Printf("tls: %v", err)
		return 1
	}
	generatedTLS := !pathExists(effectiveTLS.CertFile) && !pathExists(effectiveTLS.KeyFile)
	if err := tlsconfig.EnsurePair(effectiveTLS); err != nil {
		log.Printf("tls: %v", err)
		return 1
	}
	tlsCfg := &listener.TLSConfig{
		Listen:   effectiveTLS.Listen,
		CertFile: effectiveTLS.CertFile,
		KeyFile:  effectiveTLS.KeyFile,
	}

	// Resolve socket path.
	sockPath, err := sockpath.ResolveServe(sockpath.Options{
		Flag:         socketFlag,
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
		log.Printf("hostmux %s: pid lock: %v", name, err)
		return 1
	}
	if contention {
		if !force {
			log.Printf("hostmux %s: another daemon already serving %s (pid file: %s); exiting", name, sockPath, pidPath)
			return 0
		}
		log.Printf("hostmux %s: --force: stopping existing daemon on %s", name, sockPath)
		res, stopErr := daemonctl.Stop(daemonctl.StopOptions{
			SockPath:        sockPath,
			PIDPath:         pidPath,
			GracefulTimeout: 5 * time.Second,
			KillTimeout:     2 * time.Second,
		})
		if stopErr != nil {
			log.Printf("hostmux %s: --force: stop failed: %v", name, stopErr)
			return 1
		}
		if res.NotRunning {
			log.Printf("hostmux %s: --force: no daemon was running after all", name)
		}
		// Retry the acquire exactly once.
		pidLock, contention, err = acquirePIDLock(pidPath)
		if err != nil {
			log.Printf("hostmux %s: pid lock (retry): %v", name, err)
			return 1
		}
		if contention {
			log.Printf("hostmux %s: --force: another daemon claimed the lock during takeover", name)
			return 1
		}
	}
	defer pidLock.Close()

	// HTTP listeners.
	handler := proxy.New(r)
	lc := listener.Config{TLS: tlsCfg}
	servers, err := listener.Build(lc, handler)
	if err != nil {
		log.Printf("listener: %v", err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start HTTP servers.
	for _, srv := range servers {
		srv := srv
		go func() {
			serr := srv.ListenAndServeTLS(tlsCfg.CertFile, tlsCfg.KeyFile)
			if serr != nil && serr != http.ErrServerClosed {
				log.Printf("http server: %v", serr)
				cancel()
			}
		}()
	}
	log.Printf("hostmux %s: TLS listening on %s", name, tlsCfg.Listen)
	if generatedTLS {
		log.Printf("hostmux %s: generated self-signed cert at %s and %s", name, tlsCfg.CertFile, tlsCfg.KeyFile)
	}

	// Unix socket server.
	sockSrv := sockserver.New(r, sockserver.Options{
		OnShutdown: cancel,
		Domain: func() string {
			return currentDomain.Load().(string)
		},
		PlainHTTP: lc.TLS == nil,
	})
	if err := sockSrv.Listen(sockPath); err != nil {
		log.Printf("sockserver: %v", err)
		return 1
	}
	go sockSrv.Serve()
	log.Printf("hostmux %s: socket listening on %s", name, sockPath)

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
				currentDomain.Store(c.Domain)
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
	log.Printf("hostmux %s: shutting down", name)

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

func startForegroundArgs(configPath, socketFlag string, force bool) []string {
	args := []string{"start", "--foreground"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	if socketFlag != "" {
		args = append(args, "--socket", socketFlag)
	}
	if force {
		args = append(args, "--force")
	}
	return args
}

func startForcedDetachedDaemon(sockPath string, spawnArgs []string) error {
	// Missing or stale PID data is fine here; 0 simply means "accept any
	// live replacement PID" while we still require socket reachability.
	oldPID, _ := readPIDFile(sockpath.PIDFilePathFor(sockPath))
	if err := daemon.SpawnDetached(spawnArgs...); err != nil {
		return err
	}
	return waitForReplacementDaemon(sockPath, sockpath.PIDFilePathFor(sockPath), oldPID, 8*time.Second)
}

func waitForReplacementDaemon(sockPath, pidPath string, oldPID int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pid, err := readPIDFile(pidPath); err == nil && pid != 0 && pid != oldPID {
			conn, err := net.DialTimeout("unix", sockPath, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				return nil
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for replacement daemon on %s", sockPath)
}

func resolveServeSocketPath(configPath, socketFlag string) (string, error) {
	resolvedConfigPath := resolveConfigPath(configPath)
	configSocket := ""
	cfg, err := loadOptionalConfig(resolvedConfigPath)
	if err != nil {
		return "", err
	}
	if cfg != nil {
		configSocket = cfg.Socket
	}

	return sockpath.ResolveServe(sockpath.Options{
		Flag:         socketFlag,
		ConfigSocket: configSocket,
	})
}

func resolveConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	return defaultConfigPath()
}

func loadOptionalConfig(path string) (*config.Config, error) {
	if path == "" {
		return nil, nil
	}
	if _, err := os.Stat(path); err == nil {
		return config.Load(path)
	} else if os.IsNotExist(err) {
		return nil, nil
	} else {
		return nil, err
	}
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

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readPIDFile(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, err
	}
	return pid, nil
}
