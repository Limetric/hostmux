package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/sockpath"
)

type urlOptions struct {
	SocketPath string
	Domain     string
	Prefix     string
	NoPrefix   bool
	Names      []string
	Writer     io.Writer
}

func runURL(opts urlOptions) error {
	if err := validateExplicitNames(opts.Names); err != nil {
		return exitError{code: 2, text: fmt.Sprintf("hostmux url: %v", err)}
	}

	hosts, err := resolveRequestedNames(opts.Names)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux url: %v", err)}
	}

	hosts, err = resolveRequestedHosts(hosts, hostResolveOptions{
		Domain:   opts.Domain,
		Prefix:   opts.Prefix,
		NoPrefix: opts.NoPrefix,
	})
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux url: %v", err)}
	}

	// Ask the daemon for its port whenever a socket is reachable, even when
	// no bare host needs expansion — otherwise `hostmux url --domain X api`
	// silently drops the port and prints `https://api.X` for a daemon on
	// `:8443`. When the daemon is unreachable we fall back to the config
	// file for the domain (so bare hosts still expand offline), but the
	// port stays at 0 since only a live daemon knows the actual bound port.
	hasBare := hostnames.HasBare(hosts)
	var (
		daemonDomain  string
		daemonPort    int
		daemonWarning string
	)
	if sockPath, err := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath}); err != nil {
		daemonWarning = err.Error()
	} else if domain, _, port, err := lookupDaemonInfo(sockPath); err != nil {
		daemonWarning = err.Error()
	} else {
		daemonDomain = domain
		daemonPort = port
	}

	if hasBare {
		domain := daemonDomain
		var configWarning string
		if domain == "" {
			d, cerr := readConfigDomain(defaultConfigPath())
			if cerr != nil {
				configWarning = cerr.Error()
			}
			if d != "" {
				domain = d
				daemonWarning = ""
			}
		}

		if domain != "" {
			hosts = hostnames.Expand(hosts, domain)
		} else if configWarning != "" {
			fmt.Fprintf(os.Stderr, "hostmux url: read config: %s; using bare host unchanged\n", configWarning)
			daemonWarning = ""
		} else if daemonWarning != "" {
			fmt.Fprintf(os.Stderr, "hostmux url: %s; using bare host unchanged\n", daemonWarning)
			daemonWarning = ""
		}
	}

	// Surface the daemon-unreachable warning even when no bare host needed
	// expansion. Without this, `hostmux url --domain X api` silently prints
	// a portless URL when the daemon is down — the user has no signal that
	// the port may be missing because `--domain` pre-expands all names and
	// the bare-host branch above never runs. The branch clears
	// daemonWarning when it already printed it, so we don't double-report.
	if daemonWarning != "" {
		fmt.Fprintf(os.Stderr, "hostmux url: %s; URL may omit the daemon's listener port\n", daemonWarning)
	}

	writer := opts.Writer
	if writer == nil {
		writer = os.Stdout
	}

	for _, host := range hosts {
		if _, err := fmt.Fprintln(writer, formatPublicURL(host, "https", daemonPort)); err != nil {
			return err
		}
	}
	return nil
}
