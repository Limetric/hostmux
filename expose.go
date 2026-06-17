package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/sockproto"
)

type exposeOptions struct {
	SocketPath string
	Domain     string
	Upstream   string
	Names      []string
	Labels     []string
	Writer     interface{ Write([]byte) (int, error) }
}

// runExpose registers a manual route that outlives the command. Unlike
// `hostmux run`, no child process is started; the route persists in the
// daemon until `hostmux unexpose` removes it (or the daemon restarts).
func runExpose(opts exposeOptions) error {
	if len(opts.Names) == 0 {
		return exitError{code: 2, text: "hostmux expose: at least one --name is required"}
	}
	if err := validateExplicitNames(opts.Names); err != nil {
		return exitError{code: 2, text: fmt.Sprintf("hostmux expose: %v", err)}
	}
	if err := validateUpstreamFlag(opts.Upstream); err != nil {
		return exitError{code: 2, text: fmt.Sprintf("hostmux expose: %v", err)}
	}
	labels, err := parseLabels(opts.Labels)
	if err != nil {
		return exitError{code: 2, text: fmt.Sprintf("hostmux expose: %v", err)}
	}

	sockPath, err := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath})
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux expose: %v", err)}
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux expose: dial %s: %v; is the daemon running? (hostmux start)", sockPath, err)}
	}
	defer conn.Close()
	enc := sockproto.NewEncoder(conn)
	dec := sockproto.NewDecoder(conn)

	daemonDomain, publicHTTPS, daemonPort, infoErr := lookupDaemonInfoClient(enc, dec)

	hosts := append([]string(nil), opts.Names...)
	if hostnames.HasBare(hosts) {
		domain := opts.Domain
		if domain == "" {
			if infoErr != nil {
				return exitError{code: 1, text: fmt.Sprintf("hostmux expose: %v; pass --domain to expand bare names", infoErr)}
			}
			domain = daemonDomain
		}
		hosts = hostnames.Expand(hosts, hostnames.NormalizeDomain(domain))
	}

	// The route's primary name (first --name) is its stable identifier for
	// `hostmux unexpose`.
	routeName := opts.Names[0]

	if err := enc.Encode(&sockproto.Message{
		Op:       sockproto.OpExpose,
		Source:   routeName,
		Hosts:    hosts,
		Upstream: opts.Upstream,
		Labels:   labels,
	}); err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux expose: %v", err)}
	}
	resp, err := dec.Decode()
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux expose: %v", err)}
	}
	if !resp.Ok {
		return exitError{code: 1, text: fmt.Sprintf("hostmux expose: %s", resp.Error)}
	}

	scheme := "https"
	if infoErr == nil && !publicHTTPS {
		scheme = "http"
	}
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	for _, h := range hosts {
		fmt.Fprintf(w, "%s → %s\n", formatPublicURL(h, scheme, daemonPort), opts.Upstream)
	}
	fmt.Fprintf(w, "exposed %q (remove with: hostmux unexpose %s)\n", routeName, routeName)
	return nil
}

type unexposeOptions struct {
	SocketPath string
	Name       string
}

// runUnexpose removes a manual route previously created with `hostmux expose`.
func runUnexpose(opts unexposeOptions) error {
	if opts.Name == "" {
		return exitError{code: 2, text: "hostmux unexpose: a route NAME is required"}
	}
	sockPath, err := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath})
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux unexpose: %v", err)}
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux unexpose: dial %s: %v", sockPath, err)}
	}
	defer conn.Close()
	enc := sockproto.NewEncoder(conn)
	dec := sockproto.NewDecoder(conn)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpUnexpose, Source: opts.Name}); err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux unexpose: %v", err)}
	}
	resp, err := dec.Decode()
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux unexpose: %v", err)}
	}
	if !resp.Ok {
		return exitError{code: 1, text: fmt.Sprintf("hostmux unexpose: %s", resp.Error)}
	}
	fmt.Fprintf(os.Stderr, "unexposed %q\n", opts.Name)
	return nil
}

// validateUpstreamFlag mirrors the daemon's upstream URL rule for the
// --upstream flag: an absolute http or https URL with a host.
func validateUpstreamFlag(raw string) error {
	if raw == "" {
		return fmt.Errorf("--upstream is required (e.g. http://127.0.0.1:3000)")
	}
	if strings.TrimSpace(raw) != raw {
		return fmt.Errorf("--upstream must not contain surrounding whitespace")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("--upstream must be a valid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Host == "" || (scheme != "http" && scheme != "https") {
		return fmt.Errorf("--upstream must be an absolute http or https URL")
	}
	return nil
}
