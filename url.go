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

	if hostnames.HasBare(hosts) {
		var domain, daemonWarning string
		sockPath, err := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath})
		if err == nil {
			d, lerr := lookupDaemonDomain(sockPath)
			if lerr == nil {
				domain = d
			} else {
				daemonWarning = lerr.Error()
			}
		} else {
			daemonWarning = err.Error()
		}

		if domain == "" {
			if d := readConfigDomain(defaultConfigPath()); d != "" {
				domain = d
				daemonWarning = ""
			}
		}

		if domain != "" {
			hosts = hostnames.Expand(hosts, domain)
		} else if daemonWarning != "" {
			fmt.Fprintf(os.Stderr, "hostmux url: %s; using bare host unchanged\n", daemonWarning)
		}
	}

	writer := opts.Writer
	if writer == nil {
		writer = os.Stdout
	}

	for _, host := range hosts {
		if _, err := fmt.Fprintf(writer, "https://%s\n", host); err != nil {
			return err
		}
	}
	return nil
}
