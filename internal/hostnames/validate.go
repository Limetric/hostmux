package hostnames

import (
	"net"
	"strings"
)

// ValidHostToken reports whether name is a valid bare label, hostname, or
// bracketed IPv6 literal suitable for route registration.
func ValidHostToken(name string) bool {
	if name == "" || strings.TrimSpace(name) != name {
		return false
	}
	if strings.HasPrefix(name, "[") || strings.HasSuffix(name, "]") {
		if !(strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]")) {
			return false
		}
		inner := strings.TrimPrefix(strings.TrimSuffix(name, "]"), "[")
		return strings.Contains(inner, ":") && net.ParseIP(inner) != nil
	}
	// Bare IPv6 literals (e.g. ::1) are rejected; use bracketed form ([::1]).
	if strings.Contains(name, ":") {
		return false
	}
	if ip := net.ParseIP(name); ip != nil {
		return ip.To4() != nil
	}
	if len(name) > 253 {
		return false
	}
	labels := strings.Split(name, ".")
	for _, label := range labels {
		if !ValidDNSLabel(label) {
			return false
		}
	}
	return true
}

// ValidDNSLabel reports whether label is a valid DNS label.
func ValidDNSLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	runes := []rune(label)
	for i, r := range runes {
		isAlphaNum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		switch {
		case isAlphaNum:
		case r == '-' && i != 0 && i != len(runes)-1:
		default:
			return false
		}
	}
	return true
}
