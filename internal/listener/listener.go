// Package listener builds the HTTP servers the hostmux daemon listens on.
// Optionally returns a plain HTTP/1.1 + h2c listener for direct clients.
// Optionally also returns a TLS listener that negotiates HTTP/2 via ALPN for
// origins such as cloudflared that require HTTPS for HTTP/2-to-origin.
package listener

import (
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// Config configures the daemon's HTTP listeners.
type Config struct {
	// Plain is the optional listen address for the HTTP/1.1 + h2c listener.
	Plain string
	// TLS, if non-nil, enables an HTTP/1.1 + HTTP/2 listener on the configured
	// TLS port.
	TLS *TLSConfig
}

// TLSConfig configures the TLS listener.
type TLSConfig struct {
	Listen   string
	CertFile string
	KeyFile  string
}

// Build returns the http.Servers that should be started. The caller is
// responsible for calling Serve / ListenAndServeTLS as appropriate.
func Build(cfg Config, h http.Handler) ([]*http.Server, error) {
	servers := make([]*http.Server, 0, 2)

	if cfg.Plain != "" {
		servers = append(servers, &http.Server{
			Addr:    cfg.Plain,
			Handler: h2c.NewHandler(h, &http2.Server{}),
		})
	}

	if cfg.TLS != nil {
		tlsSrv := &http.Server{
			Addr:    cfg.TLS.Listen,
			Handler: h,
		}
		// Default TLS config negotiates HTTP/2 via ALPN.
		if err := http2.ConfigureServer(tlsSrv, &http2.Server{}); err != nil {
			return nil, err
		}
		servers = append(servers, tlsSrv)
	}
	return servers, nil
}
