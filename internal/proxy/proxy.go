// Package proxy provides an HTTP handler that reverse-proxies requests to
// the upstream returned by a router.Router lookup on the request's Host.
// The original Host header is preserved end-to-end so dev servers that
// check it (Vite, Next.js dev, Rails) see the same value the browser sent.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/Limetric/hostmux/internal/router"
)

// AccessRecord is one proxied-request observation handed to an AccessLogger.
// It deliberately omits request headers and bodies so no credentials or
// payloads are logged.
type AccessRecord struct {
	Method   string
	Host     string
	Path     string
	Status   int
	Bytes    int64
	Duration time.Duration
	Upstream string
	Source   string
	Err      string
}

// AccessLogger receives one record per proxied request when access logging
// is enabled.
type AccessLogger interface {
	LogAccess(AccessRecord)
}

// Options configures the proxy handler.
type Options struct {
	// Transport is the RoundTripper used for upstream requests. When nil,
	// httputil.ReverseProxy falls back to http.DefaultTransport, preserving
	// hostmux's prior behavior. Pass a configured transport to apply
	// upstream timeouts or TLS verification controls.
	Transport http.RoundTripper
	// Logger, if non-nil, receives one AccessRecord per request.
	Logger AccessLogger
}

// New returns an http.Handler that routes incoming requests to the upstream
// returned by r.Lookup for the request's Host. The original Host header is
// preserved end-to-end.
func New(r *router.Router) http.Handler {
	return NewWithOptions(r, Options{})
}

// NewWithOptions is New with explicit configuration (custom upstream
// transport, access logging, etc.).
func NewWithOptions(r *router.Router, opts Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var start time.Time
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		if opts.Logger != nil {
			start = time.Now()
			w = rec
		}
		host := stripPort(req.Host)
		upstream, source, ok := r.LookupSource(host)

		// finish emits the access record (when logging is on) exactly once.
		finish := func(errMsg string) {
			if opts.Logger == nil {
				return
			}
			opts.Logger.LogAccess(AccessRecord{
				Method:   req.Method,
				Host:     host,
				Path:     req.URL.Path,
				Status:   rec.status,
				Bytes:    rec.bytes,
				Duration: time.Since(start),
				Upstream: upstream,
				Source:   source,
				Err:      errMsg,
			})
		}

		if !ok {
			rec.status = http.StatusNotFound
			http.Error(w, fmt.Sprintf("no upstream for host %q (%d host(s) registered)", host, r.Count()), http.StatusNotFound)
			finish("")
			return
		}
		target, err := url.Parse(upstream)
		if err != nil {
			rec.status = http.StatusInternalServerError
			http.Error(w, "invalid upstream URL", http.StatusInternalServerError)
			finish(err.Error())
			return
		}
		originalHost := req.Host
		var proxyErr string
		// Build a fresh ReverseProxy per request so the Director closure is
		// race-free. Only the fields we actually need are set, so future
		// additions to httputil.ReverseProxy cannot silently break us.
		rp := &httputil.ReverseProxy{
			Transport: opts.Transport,
			ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
				proxyErr = err.Error()
				errorHandler(w, req, err)
			},
			Director: func(out *http.Request) {
				out.URL.Scheme = target.Scheme
				out.URL.Host = target.Host
				// Setting out.Host to a non-empty string prevents net/http
				// from falling back to out.URL.Host for the wire Host header,
				// preserving the original end-to-end.
				out.Host = originalHost
				if out.Header.Get("X-Forwarded-Host") == "" {
					out.Header.Set("X-Forwarded-Host", originalHost)
				}
				if out.Header.Get("X-Forwarded-Proto") == "" {
					if req.TLS != nil {
						out.Header.Set("X-Forwarded-Proto", "https")
					} else {
						out.Header.Set("X-Forwarded-Proto", "http")
					}
				}
			},
		}
		rp.ServeHTTP(w, req)
		finish(proxyErr)
	})
}

// statusRecorder wraps an http.ResponseWriter to capture the status code and
// byte count for access logging. It implements Unwrap so the standard
// http.ResponseController machinery (used by ReverseProxy for flushing and
// websocket hijacking) reaches the underlying writer — preserving streaming
// and websocket upgrades through the wrapper.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
	bytes  int64
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// stripPort removes the :PORT suffix from a Host header, handling IPv6
// bracketed addresses correctly.
func stripPort(hostPort string) string {
	if i := strings.LastIndex(hostPort, ":"); i != -1 {
		// For IPv6 ([::1]:8080), the last ":" is after the closing bracket.
		// For a plain v6 without a port ("[::1]"), there's no ":" after "]".
		// Only strip if there's no "]" after the ":".
		if !strings.Contains(hostPort[i:], "]") {
			return hostPort[:i]
		}
	}
	return hostPort
}

func errorHandler(w http.ResponseWriter, req *http.Request, err error) {
	// Map timeouts to 504 Gateway Timeout and everything else (refused
	// connections, DNS failures, resets) to 502 Bad Gateway, so operators
	// can tell "upstream too slow" apart from "upstream not answering".
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		http.Error(w, fmt.Sprintf("upstream timed out: %v", err), http.StatusGatewayTimeout)
		return
	}
	http.Error(w, fmt.Sprintf("upstream unreachable: %v", err), http.StatusBadGateway)
}
