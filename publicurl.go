package main

import (
	"net/url"
	"strconv"
)

// formatPublicURL returns "<scheme>://<host>" when port is zero or equal to
// the scheme's default (443 for https, 80 for http), and
// "<scheme>://<host>:<port>" otherwise. A zero port means "unspecified" —
// e.g. an older daemon that did not report public_port in OpInfo — and is
// treated identically to the scheme default so URL formatting degrades to
// today's port-less behavior rather than printing a misleading default.
func formatPublicURL(host, scheme string, port int) string {
	u := url.URL{Scheme: scheme, Host: host}
	if port != 0 && !isSchemeDefaultPort(scheme, port) {
		u.Host = host + ":" + strconv.Itoa(port)
	}
	return u.String()
}

func isSchemeDefaultPort(scheme string, port int) bool {
	switch scheme {
	case "https":
		return port == 443
	case "http":
		return port == 80
	}
	return false
}
