package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/sockpath"
)

func cmdGet(args []string) int {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	socketFlag := fs.String("socket", "", "override Unix socket path for daemon domain lookup")
	domainFlag := fs.String("domain", "", "expand bare subdomains using this base domain")
	prefixFlag := fs.String("prefix", "", "explicit hostname prefix (overrides worktree detection)")
	noPrefix := fs.Bool("no-prefix", false, "disable worktree auto-prefixing")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: hostmux get HOST [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}

	hosts := splitHosts(fs.Arg(0))
	if len(hosts) != 1 {
		fmt.Fprintln(os.Stderr, "hostmux get: HOST must be a single hostname")
		return 2
	}

	hosts, err := resolveRequestedHosts(hosts, hostResolveOptions{
		Domain:   *domainFlag,
		Prefix:   *prefixFlag,
		NoPrefix: *noPrefix,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux get: %v\n", err)
		return 1
	}

	if hostnames.HasBare(hosts) {
		sockPath, err := sockpath.Resolve(sockpath.Options{Flag: *socketFlag})
		if err == nil {
			domain, err := lookupDaemonDomain(sockPath)
			if err == nil {
				hosts = hostnames.Expand(hosts, domain)
			} else {
				fmt.Fprintf(os.Stderr, "hostmux get: %v; using bare host unchanged\n", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "hostmux get: %v; using bare host unchanged\n", err)
		}
	}

	fmt.Fprintf(os.Stdout, "https://%s\n", hosts[0])
	return 0
}
