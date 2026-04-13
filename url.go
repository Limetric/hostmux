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

	var daemonPort int
	if hostnames.HasBare(hosts) {
		sockPath, err := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath})
		if err == nil {
			domain, _, port, err := lookupDaemonInfo(sockPath)
			if err == nil {
				hosts = hostnames.Expand(hosts, domain)
				daemonPort = port
			} else {
				fmt.Fprintf(os.Stderr, "hostmux url: %v; using bare host unchanged\n", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "hostmux url: %v; using bare host unchanged\n", err)
		}
	}
	_ = daemonPort // wired up in Task 7

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
