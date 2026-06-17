package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/tlsconfig"
)

type doctorOptions struct {
	ConfigPath string
	SocketPath string
	JSON       bool
	Writer     io.Writer
	now        func() time.Time
}

// doctorFinding is one diagnostic result.
type doctorFinding struct {
	Severity config.Severity `json:"severity"`
	Check    string          `json:"check"`
	Message  string          `json:"message"`
}

// certExpiryWarnDays is how soon before expiry doctor warns.
const certExpiryWarnDays = 14

func runDoctor(opts doctorOptions) error {
	w := writerOr(opts.Writer)
	now := opts.now
	if now == nil {
		now = time.Now
	}

	var findings []doctorFinding
	add := func(sev config.Severity, check, format string, args ...any) {
		findings = append(findings, doctorFinding{Severity: sev, Check: check, Message: fmt.Sprintf(format, args...)})
	}

	// --- config ---
	configPath := resolveConfigPath(opts.ConfigPath)
	switch {
	case configPath == "":
		add(config.SeverityWarning, "config", "could not resolve a config path; using built-in defaults")
	default:
		if _, err := os.Stat(configPath); err == nil {
			add(config.SeverityInfo, "config", "config file: %s", configPath)
			if _, diags := config.Check(configPath); len(diags) > 0 {
				for _, d := range diags {
					add(d.Severity, "config", "%s", d.Message)
				}
			} else {
				add(config.SeverityInfo, "config", "config is valid")
			}
		} else {
			add(config.SeverityInfo, "config", "no config file at %s (using built-in defaults)", configPath)
		}
	}

	// --- socket ---
	sockPath, sockErr := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath})
	if sockErr != nil {
		add(config.SeverityError, "socket", "could not resolve socket path: %v", sockErr)
	} else {
		add(config.SeverityInfo, "socket", "socket path: %s", sockPath)
	}

	// --- daemon ---
	daemonOK := false
	var daemonDomain string
	var daemonPort int
	if sockErr == nil {
		if domain, https, port, err := lookupDaemonInfo(sockPath); err != nil {
			add(config.SeverityWarning, "daemon", "not reachable on %s; start it with `hostmux start`", sockPath)
		} else {
			daemonOK = true
			daemonDomain = domain
			daemonPort = port
			scheme := "https"
			if !https {
				scheme = "http"
			}
			add(config.SeverityInfo, "daemon", "running: domain=%q scheme=%s port=%d", domain, scheme, port)
		}
	}

	// --- TLS ---
	if tlsCfg, err := resolveServeTLS(opts.ConfigPath); err != nil {
		add(config.SeverityWarning, "tls", "could not resolve TLS material: %v", err)
	} else if _, statErr := os.Stat(tlsCfg.CertFile); statErr != nil {
		add(config.SeverityInfo, "tls", "certificate not generated yet (created on first `hostmux start`)")
	} else if info, ierr := tlsconfig.Inspect(tlsCfg.CertFile); ierr != nil {
		add(config.SeverityError, "tls", "certificate unreadable: %v", ierr)
	} else {
		switch {
		case info.Expired(now()):
			add(config.SeverityError, "tls", "certificate expired %s; run `hostmux cert renew`", info.NotAfter.UTC().Format(time.RFC3339))
		case info.NotAfter.Sub(now()) < certExpiryWarnDays*24*time.Hour:
			add(config.SeverityWarning, "tls", "certificate expires soon (%s); run `hostmux cert renew`", info.NotAfter.UTC().Format(time.RFC3339))
		default:
			days := int(info.NotAfter.Sub(now()).Hours() / 24)
			add(config.SeverityInfo, "tls", "certificate valid (%d days left)", days)
		}
	}

	// --- localhost + non-443 (live) ---
	if daemonOK && daemonDomain == "localhost" && daemonPort != 0 && daemonPort != 443 {
		add(config.SeverityWarning, "url", "daemon serves localhost on :%d; browsers expect :443 unless URLs include the port (see README \"HTTPS on port 443\")", daemonPort)
	}

	// --- cloudflare hint ---
	add(config.SeverityInfo, "tunnel", "for a Cloudflare Tunnel ingress snippet, run `hostmux cloudflare config`")

	return emitDoctor(w, findings, opts.JSON)
}

func emitDoctor(w io.Writer, findings []doctorFinding, asJSON bool) error {
	hasError := false
	for _, f := range findings {
		if f.Severity == config.SeverityError {
			hasError = true
		}
	}

	if asJSON {
		out := struct {
			OK       bool            `json:"ok"`
			Findings []doctorFinding `json:"findings"`
		}{OK: !hasError, Findings: findings}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
		if hasError {
			return exitError{code: 1}
		}
		return nil
	}

	// Print grouped by severity: errors, warnings, info.
	order := map[config.Severity]int{config.SeverityError: 0, config.SeverityWarning: 1, config.SeverityInfo: 2}
	sorted := append([]doctorFinding(nil), findings...)
	sort.SliceStable(sorted, func(i, j int) bool { return order[sorted[i].Severity] < order[sorted[j].Severity] })

	var errs, warns int
	for _, f := range sorted {
		fmt.Fprintf(w, "%-7s [%s] %s\n", f.Severity, f.Check, f.Message)
		switch f.Severity {
		case config.SeverityError:
			errs++
		case config.SeverityWarning:
			warns++
		}
	}
	fmt.Fprintf(w, "\n%d error(s), %d warning(s)\n", errs, warns)
	if hasError {
		return exitError{code: 1}
	}
	return nil
}
