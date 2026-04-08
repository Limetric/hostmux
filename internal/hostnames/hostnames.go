package hostnames

import (
	"net"
	"strings"
)

// NormalizeDomain trims surrounding whitespace and a trailing root dot.
func NormalizeDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimSuffix(domain, ".")
	return domain
}

// HasBare reports whether any name is a bare subdomain label rather than a
// full hostname. Any name containing a dot is treated as a full hostname.
func HasBare(names []string) bool {
	for _, name := range names {
		if needsExpansion(strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// Expand converts bare subdomain labels into full hostnames using domain.
// Inputs that already contain a dot are preserved as full hostnames.
func Expand(names []string, domain string) ([]string, error) {
	domain = NormalizeDomain(domain)
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if needsExpansion(name) {
			if domain == "" {
				out = append(out, name)
				continue
			}
			name = name + "." + domain
		}
		out = append(out, name)
	}
	return out, nil
}

func needsExpansion(name string) bool {
	if name == "" || strings.Contains(name, ".") {
		return false
	}
	if strings.EqualFold(name, "localhost") {
		return false
	}
	trimmed := strings.Trim(name, "[]")
	return net.ParseIP(trimmed) == nil
}
