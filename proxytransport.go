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

// Default server-side timeouts applied even when no [proxy] block is
// configured. Go's net/http defaults leave ReadHeaderTimeout and IdleTimeout
// unset (effectively unlimited), which allows a Slowloris-style DoS on a
// listener that may be reachable over a tunnel. These conservative defaults
// close that hole while staying generous for local development; a [proxy]
// block can override either.
const (
	defaultReadHeaderTimeout = 10 * time.Second
	defaultIdleTimeout       = 120 * time.Second
)

// serverOptions maps the [proxy] config block to listener.ServerOptions,
// falling back to hostmux's safe default timeouts for any field the block
// does not set. A nil block yields the defaults.
func serverOptions(p *config.ProxyBlock) listener.ServerOptions {
	opts := listener.ServerOptions{
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}
	if p == nil {
		return opts
	}
	if v := p.ReadHeaderTimeout.AsDuration(); v > 0 {
		opts.ReadHeaderTimeout = v
	}
	if v := p.IdleTimeout.AsDuration(); v > 0 {
		opts.IdleTimeout = v
	}
	opts.MaxHeaderBytes = p.MaxHeaderBytes
	return opts
}
