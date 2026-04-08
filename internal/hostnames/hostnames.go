package hostnames

import (
	"fmt"
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
		if isBare(name) {
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
		if isBare(name) {
			if domain == "" {
				return nil, fmt.Errorf("subdomain %q requires a domain", name)
			}
			name = name + "." + domain
		}
		out = append(out, name)
	}
	return out, nil
}

func isBare(name string) bool {
	return name != "" && !strings.Contains(name, ".")
}
