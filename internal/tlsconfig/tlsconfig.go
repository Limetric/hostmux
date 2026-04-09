package tlsconfig

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/filelock"
)

type Config struct {
	Listen   string
	CertFile string
	KeyFile  string
}

var (
	userHomeDir        = os.UserHomeDir
	beforeGeneratePair = func() {}
	generatePair       = generateSelfSignedPair
)

func Resolve(block *config.TLSBlock) (Config, error) {
	cfg := Config{Listen: config.DefaultTLSListen}
	var err error
	if block == nil {
		return fillManagedPaths(cfg)
	}
	if block.Listen != "" {
		cfg.Listen = block.Listen
	}
	if block.Cert != "" {
		cfg.CertFile, err = expandHome(block.Cert)
		if err != nil {
			return Config{}, err
		}
	}
	if block.Key != "" {
		cfg.KeyFile, err = expandHome(block.Key)
		if err != nil {
			return Config{}, err
		}
	}
	return fillManagedPaths(cfg)
}

func EnsurePair(cfg Config) error {
	lock, err := acquirePairLock(cfg)
	if err != nil {
		return err
	}
	defer lock.Close()

	certExists := fileExists(cfg.CertFile)
	keyExists := fileExists(cfg.KeyFile)

	switch {
	case certExists && keyExists:
		_, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return fmt.Errorf("load existing keypair %s %s: %w", cfg.CertFile, cfg.KeyFile, err)
		}
		return nil
	case certExists != keyExists:
		return fmt.Errorf("partial tls state: cert=%s exists=%t key=%s exists=%t", cfg.CertFile, certExists, cfg.KeyFile, keyExists)
	}

	for _, dir := range uniqueDirs(filepath.Dir(cfg.CertFile), filepath.Dir(cfg.KeyFile)) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create tls dir %s: %w", dir, err)
		}
	}

	beforeGeneratePair()
	certPEM, keyPEM, err := generatePair()
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfg.CertFile, certPEM, 0o644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(cfg.KeyFile, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	if _, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile); err != nil {
		return fmt.Errorf("load generated keypair %s %s: %w", cfg.CertFile, cfg.KeyFile, err)
	}
	return nil
}

func fillManagedPaths(cfg Config) (Config, error) {
	base, err := managedTLSDir()
	if err != nil {
		if cfg.CertFile != "" && cfg.KeyFile != "" {
			return cfg, nil
		}
		return Config{}, err
	}
	if cfg.CertFile == "" {
		cfg.CertFile = filepath.Join(base, "hostmux.crt")
	}
	if cfg.KeyFile == "" {
		cfg.KeyFile = filepath.Join(base, "hostmux.key")
	}
	return cfg, nil
}

func managedTLSDir() (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".hostmux", "tls"), nil
}

func expandHome(p string) (string, error) {
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~/")), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func acquirePairLock(cfg Config) (*os.File, error) {
	lockPath := cfg.CertFile + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("create tls lock dir: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open tls lock: %w", err)
	}
	if err := filelock.Lock(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock tls pair: %w", err)
	}
	return f, nil
}

func uniqueDirs(dirs ...string) []string {
	seen := make(map[string]struct{}, len(dirs))
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return out
}

func generateSelfSignedPair() ([]byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "hostmux.local",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create cert: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
