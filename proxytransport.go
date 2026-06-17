package main

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/listener"
)

// buildUpstreamTransport returns the RoundTripper the proxy should use for
// upstream requests. A nil ProxyBlock returns nil so the reverse proxy keeps
// using http.DefaultTransport — exactly hostmux's prior behavior. When a
// [proxy] block is present it clones the default transport and applies the
// configured timeouts and TLS verification controls.
func buildUpstreamTransport(p *config.ProxyBlock) http.RoundTripper {
	if p == nil {
		return nil
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	if d := p.DialTimeout.AsDuration(); d > 0 {
		base.DialContext = (&net.Dialer{Timeout: d, KeepAlive: 30 * time.Second}).DialContext
	}
	base.ResponseHeaderTimeout = p.ResponseHeaderTimeout.AsDuration()
	if p.UpstreamInsecureSkipVerify {
		if base.TLSClientConfig == nil {
			base.TLSClientConfig = &tls.Config{}
		}
		base.TLSClientConfig.InsecureSkipVerify = true
	}
	return base
}

// serverOptions maps the [proxy] config block to listener.ServerOptions. A
// nil block yields the zero value, preserving Go's net/http defaults.
func serverOptions(p *config.ProxyBlock) listener.ServerOptions {
	if p == nil {
		return listener.ServerOptions{}
	}
	return listener.ServerOptions{
		ReadHeaderTimeout: p.ReadHeaderTimeout.AsDuration(),
		IdleTimeout:       p.IdleTimeout.AsDuration(),
		MaxHeaderBytes:    p.MaxHeaderBytes,
	}
}
