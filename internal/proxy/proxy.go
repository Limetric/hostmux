// Package proxy provides an HTTP handler that reverse-proxies requests to
// the upstream returned by a router.Router lookup on the request's Host.
// The original Host header is preserved end-to-end so dev servers that
// check it (Vite, Next.js dev, Rails) see the same value the browser sent.
package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/Limetric/hostmux/internal/router"
)

// New returns an http.Handler that routes incoming requests to the upstream
// returned by r.Lookup for the request's Host. The original Host header is
// preserved end-to-end.
func New(r *router.Router) http.Handler {
	rp := &httputil.ReverseProxy{
		Director:     func(req *http.Request) {}, // overridden per-request
		ErrorHandler: errorHandler,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		host := stripPort(req.Host)
		upstream, ok := r.Lookup(host)
		if !ok {
			http.Error(w, fmt.Sprintf("no upstream for host %q", host), http.StatusNotFound)
			return
		}
		target, err := url.Parse(upstream)
		if err != nil {
			http.Error(w, "invalid upstream URL", http.StatusInternalServerError)
			return
		}
		// Build a fresh proxy per request so we can install a Director that
		// closes over the resolved target without races.
		perReq := *rp
		originalHost := req.Host
		perReq.Director = func(out *http.Request) {
			out.URL.Scheme = target.Scheme
			out.URL.Host = target.Host
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
		}
		perReq.ServeHTTP(w, req)
	})
}

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
	http.Error(w, fmt.Sprintf("upstream unreachable: %v", err), http.StatusBadGateway)
}
