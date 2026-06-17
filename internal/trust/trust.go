// Package trust installs and removes hostmux's managed TLS certificate from
// the operating system trust store so browsers and HTTP clients accept it
// without manual exceptions. It shells out to the platform's standard tool
// (security on macOS, certutil on Windows, update-ca-* on Linux). The command
// runner is injectable so the per-OS command construction is unit-testable
// without mutating a real trust store.
//
// Note: hostmux currently trusts the self-signed leaf certificate directly.
// A local-CA model (issue #20) is a planned follow-up; the API here is
// CA-agnostic so that change stays internal.
package trust

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Runner executes a command and returns its combined output. Injectable for
// tests; the default runs the real binary.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Options configures a trust operation.
type Options struct {
	// GOOS overrides the target platform (defaults to runtime.GOOS).
	GOOS string
	// Run executes commands (defaults to the real exec runner).
	Run Runner
	// HomeDir overrides the user's home directory (for the macOS login
	// keychain path). Defaults to os.UserHomeDir.
	HomeDir string
}

func (o Options) goos() string {
	if o.GOOS != "" {
		return o.GOOS
	}
	return runtime.GOOS
}

func (o Options) runner() Runner {
	if o.Run != nil {
		return o.Run
	}
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
}

func (o Options) home() string {
	if o.HomeDir != "" {
		return o.HomeDir
	}
	h, _ := os.UserHomeDir()
	return h
}

// Supported reports whether trust operations are implemented for goos.
func Supported(goos string) bool {
	switch goos {
	case "darwin", "linux", "windows":
		return true
	default:
		return false
	}
}

// Trust installs certPath into the OS trust store. Idempotent at the CLI
// layer (callers check IsTrusted first), but harmless to call when already
// trusted.
func Trust(ctx context.Context, certPath string, opts Options) error {
	switch opts.goos() {
	case "darwin":
		return trustDarwin(ctx, certPath, opts)
	case "linux":
		return trustLinux(ctx, certPath, opts)
	case "windows":
		return trustWindows(ctx, certPath, opts)
	default:
		return unsupported(opts.goos())
	}
}

// Untrust removes the hostmux certificate from the OS trust store.
func Untrust(ctx context.Context, certPath string, opts Options) error {
	switch opts.goos() {
	case "darwin":
		return untrustDarwin(ctx, certPath, opts)
	case "linux":
		return untrustLinux(ctx, certPath, opts)
	case "windows":
		return untrustWindows(ctx, certPath, opts)
	default:
		return unsupported(opts.goos())
	}
}

// IsTrusted reports whether certPath is currently trusted by the OS store.
func IsTrusted(ctx context.Context, certPath string, opts Options) (bool, error) {
	switch opts.goos() {
	case "darwin":
		return isTrustedDarwin(ctx, certPath, opts)
	case "linux":
		return isTrustedLinux(ctx, certPath, opts)
	case "windows":
		return isTrustedWindows(ctx, certPath, opts)
	default:
		return false, unsupported(opts.goos())
	}
}

func unsupported(goos string) error {
	return fmt.Errorf("trust: unsupported platform %q; install the certificate manually", goos)
}

// --- macOS ---

func loginKeychain(opts Options) string {
	home := opts.home()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Keychains", "login.keychain-db")
}

func trustDarwin(ctx context.Context, certPath string, opts Options) error {
	args := []string{"add-trusted-cert", "-r", "trustRoot"}
	if kc := loginKeychain(opts); kc != "" {
		args = append(args, "-k", kc)
	}
	args = append(args, certPath)
	if out, err := opts.runner()(ctx, "security", args...); err != nil {
		return fmt.Errorf("trust: security add-trusted-cert: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func untrustDarwin(ctx context.Context, certPath string, opts Options) error {
	if out, err := opts.runner()(ctx, "security", "remove-trusted-cert", certPath); err != nil {
		return fmt.Errorf("trust: security remove-trusted-cert: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isTrustedDarwin(ctx context.Context, certPath string, opts Options) (bool, error) {
	// verify-cert exits 0 when the cert chains to a trusted anchor.
	_, err := opts.runner()(ctx, "security", "verify-cert", "-c", certPath, "-p", "ssl")
	return err == nil, nil
}

// --- Linux ---

// linuxAnchor describes a distro's CA anchor directory and update command.
type linuxAnchor struct {
	dir       string
	updateCmd []string
}

// detectLinuxAnchor maps /etc/os-release to the anchor dir + update command.
// Falls back to the Debian/Ubuntu layout, which most derivatives share.
func detectLinuxAnchor(osRelease string) linuxAnchor {
	id := parseOSReleaseIDs(osRelease)
	switch {
	case id["arch"]:
		return linuxAnchor{dir: "/etc/ca-certificates/trust-source/anchors", updateCmd: []string{"update-ca-trust"}}
	case id["fedora"] || id["rhel"] || id["centos"] || id["rocky"] || id["almalinux"]:
		return linuxAnchor{dir: "/etc/pki/ca-trust/source/anchors", updateCmd: []string{"update-ca-trust"}}
	case id["opensuse"] || id["sles"] || id["suse"]:
		return linuxAnchor{dir: "/etc/pki/trust/anchors", updateCmd: []string{"update-ca-certificates"}}
	default: // debian, ubuntu, and the long tail of derivatives
		return linuxAnchor{dir: "/usr/local/share/ca-certificates", updateCmd: []string{"update-ca-certificates"}}
	}
}

// parseOSReleaseIDs returns the set of identifiers found in ID and ID_LIKE.
func parseOSReleaseIDs(osRelease string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(osRelease, "\n") {
		line = strings.TrimSpace(line)
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if key != "ID" && key != "ID_LIKE" {
			continue
		}
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		for _, f := range strings.Fields(val) {
			out[strings.ToLower(f)] = true
		}
	}
	return out
}

// linuxAnchorFor reads /etc/os-release (best effort) and returns the anchor.
func linuxAnchorFor() linuxAnchor {
	b, _ := os.ReadFile("/etc/os-release")
	return detectLinuxAnchor(string(b))
}

// linuxAnchorFile is the unique filename hostmux installs into the anchor dir.
const linuxAnchorFile = "hostmux-ca.crt"

func trustLinux(ctx context.Context, certPath string, opts Options) error {
	anchor := linuxAnchorFor()
	dest := filepath.Join(anchor.dir, linuxAnchorFile)
	run := opts.runner()
	// Installing into a system anchor dir requires root; use sudo so a
	// non-root user gets a single password prompt rather than EACCES.
	if out, err := run(ctx, "sudo", "cp", certPath, dest); err != nil {
		return fmt.Errorf("trust: copy cert to %s: %v: %s", dest, err, strings.TrimSpace(string(out)))
	}
	args := append([]string{}, anchor.updateCmd...)
	if out, err := run(ctx, "sudo", args...); err != nil {
		return fmt.Errorf("trust: %s: %v: %s", strings.Join(anchor.updateCmd, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func untrustLinux(ctx context.Context, certPath string, opts Options) error {
	anchor := linuxAnchorFor()
	dest := filepath.Join(anchor.dir, linuxAnchorFile)
	run := opts.runner()
	if out, err := run(ctx, "sudo", "rm", "-f", dest); err != nil {
		return fmt.Errorf("trust: remove %s: %v: %s", dest, err, strings.TrimSpace(string(out)))
	}
	if out, err := run(ctx, "sudo", anchor.updateCmd...); err != nil {
		return fmt.Errorf("trust: %s: %v: %s", strings.Join(anchor.updateCmd, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isTrustedLinux(_ context.Context, certPath string, _ Options) (bool, error) {
	anchor := linuxAnchorFor()
	dest := filepath.Join(anchor.dir, linuxAnchorFile)
	installed, err := os.ReadFile(dest)
	if err != nil {
		return false, nil // not present == not trusted
	}
	want, err := os.ReadFile(certPath)
	if err != nil {
		return false, err
	}
	return bytes.Equal(bytes.TrimSpace(installed), bytes.TrimSpace(want)), nil
}

// --- Windows ---

func trustWindows(ctx context.Context, certPath string, opts Options) error {
	if out, err := opts.runner()(ctx, "certutil", "-addstore", "-user", "Root", certPath); err != nil {
		return fmt.Errorf("trust: certutil -addstore: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func untrustWindows(ctx context.Context, certPath string, opts Options) error {
	cn, err := certCommonName(certPath)
	if err != nil {
		return err
	}
	if cn == "" {
		return fmt.Errorf("trust: certificate has no common name to delete by")
	}
	if out, err := opts.runner()(ctx, "certutil", "-delstore", "-user", "Root", cn); err != nil {
		return fmt.Errorf("trust: certutil -delstore: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isTrustedWindows(ctx context.Context, certPath string, opts Options) (bool, error) {
	fp, err := certSHA1Fingerprint(certPath)
	if err != nil {
		return false, err
	}
	out, err := opts.runner()(ctx, "certutil", "-store", "-user", "Root")
	if err != nil {
		// A failed store listing is treated as "not trusted" rather than a
		// hard error so callers can proceed to install.
		return false, nil
	}
	// certutil prints fingerprints with spaces and mixed case; normalize.
	haystack := strings.ToLower(strings.ReplaceAll(string(out), " ", ""))
	return strings.Contains(haystack, strings.ToLower(fp)), nil
}

// --- cert helpers ---

func parseLeaf(certPath string) (*x509.Certificate, error) {
	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("trust: read cert: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("trust: %s is not PEM", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("trust: parse cert: %w", err)
	}
	return cert, nil
}

func certCommonName(certPath string) (string, error) {
	cert, err := parseLeaf(certPath)
	if err != nil {
		return "", err
	}
	return cert.Subject.CommonName, nil
}

func certSHA1Fingerprint(certPath string) (string, error) {
	cert, err := parseLeaf(certPath)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(cert.Raw)
	return hex.EncodeToString(sum[:]), nil
}
