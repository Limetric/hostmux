package main

import (
	"fmt"
	"net"

	"github.com/BurntSushi/toml"

	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/sockproto"
)

type hostResolveOptions struct {
	Domain   string
	Prefix   string
	NoPrefix bool
}

func resolveRequestedHosts(hosts []string, opts hostResolveOptions) ([]string, error) {
	prefix, err := resolvePrefix(opts.Prefix, opts.NoPrefix)
	if err != nil {
		return nil, err
	}

	resolved := append([]string(nil), hosts...)
	if prefix != "" {
		for i, h := range resolved {
			resolved[i] = prefix + "-" + h
		}
	}

	return hostnames.Expand(resolved, hostnames.NormalizeDomain(opts.Domain)), nil
}

func lookupDaemonDomain(sockPath string) (string, error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", sockPath, err)
	}
	defer conn.Close()

	return lookupDaemonDomainClient(sockproto.NewEncoder(conn), sockproto.NewDecoder(conn))
}

func lookupDaemonInfoClient(enc *sockproto.Encoder, dec *sockproto.Decoder) (daemonDomain string, publicHTTPS bool, err error) {
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpInfo}); err != nil {
		return "", true, fmt.Errorf("info: %w", err)
	}
	resp, err := dec.Decode()
	if err != nil {
		return "", true, fmt.Errorf("info response: %w", err)
	}
	publicHTTPS = true
	if resp.PublicHTTPS != nil {
		publicHTTPS = *resp.PublicHTTPS
	}
	if !resp.Ok {
		if resp.Error != "" {
			return "", publicHTTPS, fmt.Errorf("daemon rejected info lookup: %s", resp.Error)
		}
		return "", publicHTTPS, fmt.Errorf("daemon rejected info lookup")
	}
	return hostnames.NormalizeDomain(resp.Domain), publicHTTPS, nil
}

func lookupDaemonDomainClient(enc *sockproto.Encoder, dec *sockproto.Decoder) (string, error) {
	domain, _, err := lookupDaemonInfoClient(enc, dec)
	return domain, err
}

// readConfigDomain returns the raw `domain` value from the TOML file at path,
// without applying daemon-side defaults like "localhost". Empty string on any
// error or when the field is unset, so callers can treat it as "no signal".
func readConfigDomain(path string) string {
	if path == "" {
		return ""
	}
	var raw struct {
		Domain string `toml:"domain"`
	}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return ""
	}
	return hostnames.NormalizeDomain(raw.Domain)
}
