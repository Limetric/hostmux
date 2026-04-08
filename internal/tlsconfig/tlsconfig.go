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
)

type Config struct {
	Listen   string
	CertFile string
	KeyFile  string
}

func Resolve(block *config.TLSBlock) (Config, error) {
	cfg, err := defaultConfig()
	if err != nil {
		return Config{}, err
	}
	if block == nil {
		return cfg, nil
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
	return cfg, nil
}

func EnsurePair(cfg Config) error {
	certExists := fileExists(cfg.CertFile)
	keyExists := fileExists(cfg.KeyFile)

	switch {
	case certExists && keyExists:
		_, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return fmt.Errorf("load existing keypair: %w", err)
		}
		return nil
	case certExists != keyExists:
		return fmt.Errorf("partial tls state: cert=%t key=%t", certExists, keyExists)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.CertFile), 0o700); err != nil {
		return fmt.Errorf("create tls dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.KeyFile), 0o700); err != nil {
		return fmt.Errorf("create tls dir: %w", err)
	}

	certPEM, keyPEM, err := generateSelfSignedPair()
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
		return fmt.Errorf("load generated keypair: %w", err)
	}
	return nil
}

func defaultConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	base := filepath.Join(home, ".hostmux", "tls")
	return Config{
		Listen:   ":8443",
		CertFile: filepath.Join(base, "hostmux.crt"),
		KeyFile:  filepath.Join(base, "hostmux.key"),
	}, nil
}

func expandHome(p string) (string, error) {
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~/")), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
