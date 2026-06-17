package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/sockpath"
)

type cloudflareOptions struct {
	ConfigPath string
	SocketPath string
	Domain     string
	Writer     io.Writer
}

// cloudflareParams are the resolved inputs to the ingress snippet renderer.
type cloudflareParams struct {
	Domain      string
	Port        int
	HTTPSOrigin bool
	NoTLSVerify bool
}

// renderCloudflareIngress returns a copy-pasteable cloudflared ingress block
// for the given parameters. cloudflared requires a trailing catch-all rule.
func renderCloudflareIngress(p cloudflareParams) string {
	scheme := "http"
	if p.HTTPSOrigin {
		scheme = "https"
	}
	var b strings.Builder
	b.WriteString("ingress:\n")
	fmt.Fprintf(&b, "  - hostname: \"*.%s\"\n", p.Domain)
	fmt.Fprintf(&b, "    service: %s://127.0.0.1:%d\n", scheme, p.Port)
	if p.HTTPSOrigin {
		b.WriteString("    originRequest:\n")
		b.WriteString("      http2Origin: true\n")
		if p.NoTLSVerify {
			b.WriteString("      noTLSVerify: true\n")
		}
	}
	b.WriteString("  - service: http_status:404\n")
	return b.String()
}

func runCloudflareConfig(opts cloudflareOptions) error {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}

	cfg, err := loadOptionalConfig(resolveConfigPath(opts.ConfigPath))
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux cloudflare: config: %v", err)}
	}

	// Best-effort live daemon lookup to fill gaps the config doesn't cover.
	var daemonDomain string
	var daemonPort int
	if sockPath, derr := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath}); derr == nil {
		if d, _, port, ierr := lookupDaemonInfo(sockPath); ierr == nil {
			daemonDomain = d
			daemonPort = port
		}
	}

	// Domain: flag > config > daemon.
	domain := hostnames.NormalizeDomain(opts.Domain)
	if domain == "" && cfg != nil {
		domain = cfg.Domain
	}
	if domain == "" {
		domain = daemonDomain
	}
	if domain == "" {
		return exitError{code: 2, text: "hostmux cloudflare: no domain configured; set `domain` in config or pass --domain"}
	}

	// Service port: the real local listener. The config is authoritative
	// (the daemon hides its real port when hide_port is set), so prefer it.
	port := 0
	if cfg != nil {
		if p, perr := extractListenPort(effectiveListen(cfg)); perr == nil {
			port = p
		}
	}
	if port == 0 && daemonPort > 0 {
		port = daemonPort
	}
	if port == 0 {
		port = 8443 // documented default
	}

	noTLSVerify := true
	customCert := cfg != nil && cfg.TLS != nil && cfg.TLS.Cert != ""
	if customCert {
		noTLSVerify = false
	}

	fmt.Fprint(w, renderCloudflareIngress(cloudflareParams{
		Domain:      domain,
		Port:        port,
		HTTPSOrigin: true,
		NoTLSVerify: noTLSVerify,
	}))

	// Advisory notes to stderr so the YAML on stdout stays copy-pasteable.
	if customCert {
		fmt.Fprintln(os.Stderr, "note: a custom tls.cert is configured; noTLSVerify omitted. Keep it only if cloudflared trusts that certificate.")
	}
	if cfg != nil && !cfg.HidePort && port != 443 {
		fmt.Fprintln(os.Stderr, "note: set `hide_port = true` so hostmux-printed URLs omit the listener port behind the tunnel.")
	}
	return nil
}

// effectiveListen returns the listen address the TLS edge binds: the
// tls.listen override when set, otherwise the top-level listen.
func effectiveListen(cfg *config.Config) string {
	if cfg.TLS != nil && cfg.TLS.Listen != "" {
		return cfg.TLS.Listen
	}
	return cfg.Listen
}
