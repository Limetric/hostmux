package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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

	// Discovery file + PID file.
	if err := sockpath.WriteDiscovery(sockPath); err != nil {
		log.Printf("sockpath: write discovery: %v", err)
	}
	pidPath := pidFilePath()
	if pidPath != "" {
		_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644)
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

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdownCtx)
	}
	_ = sockSrv.Close()
	_ = sockpath.RemoveDiscovery()
	if pidPath != "" {
		_ = os.Remove(pidPath)
	}
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

func pidFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".hostmux")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "hostmux.pid")
}
