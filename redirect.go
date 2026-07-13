package main

import (
	"fmt"
	"net/http"
	"strings"
)

// newHTTPSRedirectHandler returns a handler that answers every plain-HTTP
// request with a permanent redirect to the equivalent HTTPS URL. httpsPort is
// the real TLS listener port; hidePort mirrors the config flag so the target
// omits the port when it would not appear in the public URL (443 or hidden).
func newHTTPSRedirectHandler(httpsPort int, hidePort bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := requestHostname(r.Host)
		if host == "" {
			http.Error(w, "missing Host header", http.StatusBadRequest)
			return
		}
		target := "https://" + host
		if p := advertisedPort(httpsPort, hidePort); p != 0 && p != 443 {
			target += fmt.Sprintf(":%d", p)
		}
		target += r.URL.RequestURI()
		// 308 preserves the request method (unlike 301, which can turn a POST
		// into a GET), which is the correct semantics for a transport upgrade.
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}

// requestHostname strips a trailing :port from a request Host, preserving a
// bracketed IPv6 literal (which stays bracketed so it is valid in a URL).
func requestHostname(hostport string) string {
	if strings.HasPrefix(hostport, "[") {
		if i := strings.Index(hostport, "]"); i != -1 {
			return hostport[:i+1]
		}
		return hostport
	}
	if i := strings.LastIndex(hostport, ":"); i != -1 {
		return hostport[:i]
	}
	return hostport
}
