package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/tlsconfig"
	"github.com/Limetric/hostmux/internal/trust"
)

// currentGOOS reports the running OS. A function var so tests can stub it.
var currentGOOS = func() string { return runtime.GOOS }

// baseTrustOptions returns the trust.Options used for trust operations. A
// function var so tests can inject a fake command runner instead of touching
// the real OS trust store.
var baseTrustOptions = func() trust.Options { return trust.Options{} }

type trustOptions struct {
	ConfigPath string
	Force      bool
	Remove     bool
	Writer     io.Writer
}

// tlsBlockFromConfig builds the effective TLS block the daemon would serve,
// mirroring the listen-fallback logic. Shared by serve and trust so the
// certificate they operate on is always the same one.
func tlsBlockFromConfig(cfg *config.Config) *config.TLSBlock {
	if cfg == nil {
		return nil
	}
	if cfg.TLS != nil {
		block := *cfg.TLS
		if block.Listen == "" && cfg.Listen != "" {
			block.Listen = cfg.Listen
		}
		return &block
	}
	if cfg.Listen != "" {
		return &config.TLSBlock{Listen: cfg.Listen}
	}
	return nil
}

// resolveServeTLS resolves the TLS material the daemon would serve for the
// given config path (managed paths under ~/.hostmux/tls by default).
func resolveServeTLS(configPath string) (tlsconfig.Config, error) {
	cfg, err := loadOptionalConfig(resolveConfigPath(configPath))
	if err != nil {
		return tlsconfig.Config{}, err
	}
	return tlsconfig.Resolve(tlsBlockFromConfig(cfg))
}

func runTrust(opts trustOptions) error {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	name := "trust"
	if opts.Remove {
		name = "untrust"
	}
	if !trust.Supported(currentGOOS()) {
		return exitError{code: 1, text: fmt.Sprintf("hostmux %s: unsupported platform %q; install/remove the certificate manually", name, currentGOOS())}
	}

	tlsCfg, err := resolveServeTLS(opts.ConfigPath)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux %s: %v", name, err)}
	}
	if err := tlsconfig.EnsurePair(tlsCfg); err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux %s: ensure cert: %v", name, err)}
	}
	certPath := tlsCfg.CertFile

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	topts := baseTrustOptions()

	if opts.Remove {
		if err := trust.Untrust(ctx, certPath, topts); err != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux untrust: %v", err)}
		}
		fmt.Fprintf(w, "removed %s from the trust store\n", certPath)
		return nil
	}

	if !opts.Force {
		if trusted, _ := trust.IsTrusted(ctx, certPath, topts); trusted {
			fmt.Fprintf(w, "already trusted: %s\n", certPath)
			return nil
		}
	}
	if err := trust.Trust(ctx, certPath, topts); err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux trust: %v", err)}
	}
	fmt.Fprintf(w, "trusted %s\n", certPath)
	fmt.Fprintln(w, "Restart browsers if a tab was already open to hostmux.")
	return nil
}

// maybeAutoTrust installs the managed cert into the OS trust store when
// auto-trust is enabled (config tls.auto_trust or HOSTMUX_TLS_AUTO_TRUST=1)
// and the cert is not already trusted. Best effort: failures are logged, not
// fatal, so the daemon still starts.
func maybeAutoTrust(cfg *config.Config, certPath string, logf func(string, ...any)) {
	enabled := os.Getenv("HOSTMUX_TLS_AUTO_TRUST") == "1"
	if cfg != nil && cfg.TLS != nil && cfg.TLS.AutoTrust {
		enabled = true
	}
	if !enabled {
		return
	}
	if !trust.Supported(currentGOOS()) {
		logf("auto-trust: unsupported platform %q; skipping", currentGOOS())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	topts := baseTrustOptions()
	if trusted, _ := trust.IsTrusted(ctx, certPath, topts); trusted {
		return
	}
	if err := trust.Trust(ctx, certPath, topts); err != nil {
		logf("auto-trust failed: %v", err)
		return
	}
	logf("auto-trust: installed %s into the OS trust store", certPath)
}
