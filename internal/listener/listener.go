// Package listener builds the HTTP servers the hostmux daemon listens on.
// Optionally returns a plain HTTP/1.1 + unencrypted HTTP/2 listener for
// direct clients. Optionally also returns a TLS listener that negotiates
// HTTP/2 via ALPN for origins such as cloudflared that require HTTPS for
// HTTP/2-to-origin.
package listener

import (
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// Config configures the daemon's HTTP listeners.
type Config struct {
	// Plain is the optional listen address for the HTTP/1.1 + unencrypted HTTP/2 listener.
	Plain string
	// TLS, if non-nil, enables an HTTP/1.1 + HTTP/2 listener on the configured
	// TLS port.
	TLS *TLSConfig
	// Server carries optional server-side hardening knobs applied to every
	// built http.Server. Zero values keep Go's defaults.
	Server ServerOptions
}

// ServerOptions holds server-side timeout and size limits. All zero values
// preserve net/http's defaults (effectively unlimited timeouts, 1 MiB
// headers).
type ServerOptions struct {
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// TLSConfig configures the TLS listener.
type TLSConfig struct {
	Listen   string
	CertFile string
	KeyFile  string
}

// Servers groups the per-role HTTP servers the daemon should start.
// Either field may be nil if the corresponding Config entry was empty.
// The named fields replace the previous positional contract (TLS last),
// so callers that need to distinguish them — e.g. to hand a pre-bound
// listener to only the TLS server — can do so without a brittle
// `i == len(servers)-1` check.
type Servers struct {
	Plain *http.Server
	TLS   *http.Server
}

// All returns the non-nil servers in startup order (Plain first, TLS
// last). Convenient for loops that don't care which role a server has,
// such as shutdown.
func (s Servers) All() []*http.Server {
	out := make([]*http.Server, 0, 2)
	if s.Plain != nil {
		out = append(out, s.Plain)
	}
	if s.TLS != nil {
		out = append(out, s.TLS)
	}
	return out
}

// Build returns the per-role http.Servers that should be started. The
// caller is responsible for calling Serve / ListenAndServeTLS on each
// non-nil field as appropriate.
func Build(cfg Config, h http.Handler) (Servers, error) {
	var s Servers

	if cfg.Plain != "" {
		plainSrv := &http.Server{
			Addr:    cfg.Plain,
			Handler: h,
		}
		cfg.Server.apply(plainSrv)
		plainSrv.Protocols = new(http.Protocols)
		plainSrv.Protocols.SetHTTP1(true)
		plainSrv.Protocols.SetUnencryptedHTTP2(true)
		s.Plain = plainSrv
	}

	if cfg.TLS != nil {
		tlsSrv := &http.Server{
			Addr:    cfg.TLS.Listen,
			Handler: h,
		}
		cfg.Server.apply(tlsSrv)
		// Default TLS config negotiates HTTP/2 via ALPN.
		if err := http2.ConfigureServer(tlsSrv, &http2.Server{}); err != nil {
			return Servers{}, err
		}
		s.TLS = tlsSrv
	}
	return s, nil
}

// apply sets the non-zero server options on srv, leaving Go's defaults in
// place for any field left at its zero value.
func (o ServerOptions) apply(srv *http.Server) {
	if o.ReadHeaderTimeout > 0 {
		srv.ReadHeaderTimeout = o.ReadHeaderTimeout
	}
	if o.IdleTimeout > 0 {
		srv.IdleTimeout = o.IdleTimeout
	}
	if o.MaxHeaderBytes > 0 {
		srv.MaxHeaderBytes = o.MaxHeaderBytes
	}
}
