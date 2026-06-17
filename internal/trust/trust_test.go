package trust

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordedCall struct {
	name string
	args []string
}

func recorder(calls *[]recordedCall, out []byte, err error) Runner {
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, recordedCall{name: name, args: args})
		return out, err
	}
}

// writeTestCert generates a self-signed cert with the given CN and returns
// its PEM path and lowercase SHA-1 fingerprint.
func writeTestCert(t *testing.T, cn string) (path, fingerprint string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "cert.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum(der)
	return path, hex.EncodeToString(sum[:])
}

func joinArgs(c recordedCall) string {
	return c.name + " " + strings.Join(c.args, " ")
}

func TestTrustDarwinCommand(t *testing.T) {
	certPath, _ := writeTestCert(t, "hostmux.local")
	var calls []recordedCall
	err := Trust(context.Background(), certPath, Options{GOOS: "darwin", HomeDir: "/Users/dev", Run: recorder(&calls, nil, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %+v", calls)
	}
	got := joinArgs(calls[0])
	for _, want := range []string{"security add-trusted-cert", "-r trustRoot", "/Users/dev/Library/Keychains/login.keychain-db", certPath} {
		if !strings.Contains(got, want) {
			t.Fatalf("command %q missing %q", got, want)
		}
	}
}

func TestUntrustDarwinCommand(t *testing.T) {
	certPath, _ := writeTestCert(t, "hostmux.local")
	var calls []recordedCall
	if err := Untrust(context.Background(), certPath, Options{GOOS: "darwin", Run: recorder(&calls, nil, nil)}); err != nil {
		t.Fatal(err)
	}
	if got := joinArgs(calls[0]); !strings.Contains(got, "security remove-trusted-cert") {
		t.Fatalf("got %q", got)
	}
}

func TestIsTrustedDarwin(t *testing.T) {
	certPath, _ := writeTestCert(t, "hostmux.local")
	// Exit 0 -> trusted.
	ok, _ := IsTrusted(context.Background(), certPath, Options{GOOS: "darwin", Run: recorder(new([]recordedCall), nil, nil)})
	if !ok {
		t.Fatal("expected trusted when verify-cert succeeds")
	}
	// Non-zero -> not trusted.
	ok, _ = IsTrusted(context.Background(), certPath, Options{GOOS: "darwin", Run: recorder(new([]recordedCall), nil, errors.New("not trusted"))})
	if ok {
		t.Fatal("expected not trusted when verify-cert fails")
	}
}

func TestTrustWindowsCommands(t *testing.T) {
	certPath, fp := writeTestCert(t, "hostmux Local CA")
	var calls []recordedCall
	if err := Trust(context.Background(), certPath, Options{GOOS: "windows", Run: recorder(&calls, nil, nil)}); err != nil {
		t.Fatal(err)
	}
	if got := joinArgs(calls[0]); !strings.Contains(got, "certutil -addstore -user Root") {
		t.Fatalf("addstore: %q", got)
	}

	calls = nil
	if err := Untrust(context.Background(), certPath, Options{GOOS: "windows", Run: recorder(&calls, nil, nil)}); err != nil {
		t.Fatal(err)
	}
	if got := joinArgs(calls[0]); !strings.Contains(got, "certutil -delstore -user Root") || !strings.Contains(got, "hostmux Local CA") {
		t.Fatalf("delstore should delete by CN: %q", got)
	}

	// isTrusted matches on fingerprint substring (certutil formats with spaces).
	store := []byte("Cert Hash(sha1): " + spaceEvery2(fp) + "\n")
	ok, _ := IsTrusted(context.Background(), certPath, Options{GOOS: "windows", Run: recorder(new([]recordedCall), store, nil)})
	if !ok {
		t.Fatalf("expected trusted when fingerprint %q present in store", fp)
	}
}

func spaceEvery2(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		end := i + 2
		if end > len(s) {
			end = len(s)
		}
		b.WriteString(s[i:end])
	}
	return b.String()
}

func TestDetectLinuxAnchor(t *testing.T) {
	cases := []struct {
		osRelease string
		wantDir   string
		wantCmd   string
	}{
		{`ID=ubuntu`, "/usr/local/share/ca-certificates", "update-ca-certificates"},
		{`ID=debian`, "/usr/local/share/ca-certificates", "update-ca-certificates"},
		{`ID=arch`, "/etc/ca-certificates/trust-source/anchors", "update-ca-trust"},
		{`ID=fedora`, "/etc/pki/ca-trust/source/anchors", "update-ca-trust"},
		{"ID=rocky\nID_LIKE=\"rhel centos fedora\"", "/etc/pki/ca-trust/source/anchors", "update-ca-trust"},
		{`ID=opensuse-leap`, "/usr/local/share/ca-certificates", "update-ca-certificates"}, // not matched -> default
		{`ID=suse`, "/etc/pki/trust/anchors", "update-ca-certificates"},
		{``, "/usr/local/share/ca-certificates", "update-ca-certificates"},
	}
	for _, tc := range cases {
		a := detectLinuxAnchor(tc.osRelease)
		if a.dir != tc.wantDir {
			t.Errorf("osRelease %q: dir = %q, want %q", tc.osRelease, a.dir, tc.wantDir)
		}
		if strings.Join(a.updateCmd, " ") != tc.wantCmd {
			t.Errorf("osRelease %q: cmd = %v, want %q", tc.osRelease, a.updateCmd, tc.wantCmd)
		}
	}
}

func TestTrustLinuxUsesSudoAndAnchor(t *testing.T) {
	certPath, _ := writeTestCert(t, "hostmux.local")
	var calls []recordedCall
	if err := Trust(context.Background(), certPath, Options{GOOS: "linux", Run: recorder(&calls, nil, nil)}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected cp + update, got %+v", calls)
	}
	if got := joinArgs(calls[0]); !strings.Contains(got, "sudo cp") || !strings.Contains(got, linuxAnchorFile) {
		t.Fatalf("copy command: %q", got)
	}
	if got := joinArgs(calls[1]); !strings.HasPrefix(got, "sudo update-ca") {
		t.Fatalf("update command: %q", got)
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	certPath, _ := writeTestCert(t, "x")
	if err := Trust(context.Background(), certPath, Options{GOOS: "plan9", Run: recorder(new([]recordedCall), nil, nil)}); err == nil {
		t.Fatal("expected unsupported error")
	}
	if Supported("plan9") {
		t.Fatal("plan9 should be unsupported")
	}
}

func TestCertHelpers(t *testing.T) {
	certPath, fp := writeTestCert(t, "hostmux.local")
	cn, err := certCommonName(certPath)
	if err != nil || cn != "hostmux.local" {
		t.Fatalf("cn = %q err = %v", cn, err)
	}
	got, err := certSHA1Fingerprint(certPath)
	if err != nil || got != fp {
		t.Fatalf("fingerprint = %q want %q err %v", got, fp, err)
	}
}
