package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Limetric/hostmux/internal/tlsconfig"
)

type certOptions struct {
	ConfigPath string
	JSON       bool
	Force      bool
	Writer     io.Writer
	now        func() time.Time
}

// certManaged reports whether the daemon would serve a hostmux-managed cert
// (no custom tls.cert configured) for the given config path.
func certManaged(configPath string) (bool, error) {
	cfg, err := loadOptionalConfig(resolveConfigPath(configPath))
	if err != nil {
		return false, err
	}
	block := tlsBlockFromConfig(cfg)
	return block == nil || block.Cert == "", nil
}

func runCertPath(opts certOptions) error {
	w := writerOr(opts.Writer)
	tlsCfg, err := resolveServeTLS(opts.ConfigPath)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux cert path: %v", err)}
	}
	fmt.Fprintf(w, "cert: %s\n", tlsCfg.CertFile)
	fmt.Fprintf(w, "key:  %s\n", tlsCfg.KeyFile)
	return nil
}

// certInfoJSON is the stable JSON shape of `hostmux cert info --json`.
type certInfoJSON struct {
	CertPath    string   `json:"cert_path"`
	KeyPath     string   `json:"key_path"`
	Managed     bool     `json:"managed"`
	Exists      bool     `json:"exists"`
	CommonName  string   `json:"common_name,omitempty"`
	DNSNames    []string `json:"dns_names,omitempty"`
	IPAddresses []string `json:"ip_addresses,omitempty"`
	NotBefore   string   `json:"not_before,omitempty"`
	NotAfter    string   `json:"not_after,omitempty"`
	Serial      string   `json:"serial,omitempty"`
	Expired     bool     `json:"expired"`
	DaysLeft    int      `json:"days_left"`
}

func runCertInfo(opts certOptions) error {
	w := writerOr(opts.Writer)
	now := opts.now
	if now == nil {
		now = time.Now
	}
	tlsCfg, err := resolveServeTLS(opts.ConfigPath)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux cert info: %v", err)}
	}
	managed, _ := certManaged(opts.ConfigPath)

	out := certInfoJSON{CertPath: tlsCfg.CertFile, KeyPath: tlsCfg.KeyFile, Managed: managed}
	if _, statErr := os.Stat(tlsCfg.CertFile); statErr == nil {
		out.Exists = true
		info, ierr := tlsconfig.Inspect(tlsCfg.CertFile)
		if ierr != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux cert info: %v", ierr)}
		}
		out.CommonName = info.CommonName
		out.DNSNames = info.DNSNames
		out.IPAddresses = info.IPAddresses
		out.NotBefore = info.NotBefore.UTC().Format(time.RFC3339)
		out.NotAfter = info.NotAfter.UTC().Format(time.RFC3339)
		out.Serial = info.Serial
		out.Expired = info.Expired(now())
		out.DaysLeft = int(info.NotAfter.Sub(now()).Hours() / 24)
	}

	if opts.JSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Fprintf(w, "cert path: %s\n", out.CertPath)
	fmt.Fprintf(w, "key path:  %s\n", out.KeyPath)
	fmt.Fprintf(w, "managed:   %t\n", out.Managed)
	if !out.Exists {
		fmt.Fprintln(w, "status:    not generated yet (run `hostmux start` or `hostmux trust`)")
		return nil
	}
	fmt.Fprintf(w, "subject:   CN=%s\n", out.CommonName)
	if len(out.DNSNames) > 0 {
		fmt.Fprintf(w, "dns SANs:  %s\n", strings.Join(out.DNSNames, ", "))
	}
	if len(out.IPAddresses) > 0 {
		fmt.Fprintf(w, "ip SANs:   %s\n", strings.Join(out.IPAddresses, ", "))
	}
	fmt.Fprintf(w, "not before: %s\n", out.NotBefore)
	fmt.Fprintf(w, "not after:  %s\n", out.NotAfter)
	if out.Expired {
		fmt.Fprintln(w, "status:    EXPIRED — renew with `hostmux cert renew`")
	} else {
		fmt.Fprintf(w, "status:    valid (%d days left)\n", out.DaysLeft)
	}
	return nil
}

func runCertRenew(opts certOptions) error {
	w := writerOr(opts.Writer)
	managed, err := certManaged(opts.ConfigPath)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux cert renew: %v", err)}
	}
	if !managed && !opts.Force {
		return exitError{code: 1, text: "hostmux cert renew: a custom tls.cert is configured; renewing would overwrite it. Re-run with --force to proceed."}
	}
	tlsCfg, err := resolveServeTLS(opts.ConfigPath)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux cert renew: %v", err)}
	}
	if err := tlsconfig.Renew(tlsCfg); err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux cert renew: %v", err)}
	}
	fmt.Fprintf(w, "renewed %s\n", tlsCfg.CertFile)
	fmt.Fprintln(w, "Restart the daemon to serve the new certificate: hostmux start --force")
	return nil
}

func writerOr(w io.Writer) io.Writer {
	if w == nil {
		return os.Stdout
	}
	return w
}
