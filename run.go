package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/Limetric/hostmux/internal/childproc"
	"github.com/Limetric/hostmux/internal/daemon"
	"github.com/Limetric/hostmux/internal/hostnames"
	"github.com/Limetric/hostmux/internal/sockpath"
	"github.com/Limetric/hostmux/internal/sockproto"
	"github.com/Limetric/hostmux/internal/worktree"
)

type runOptions struct {
	SocketPath string
	Domain     string
	Prefix     string
	NoPrefix   bool
	Names      []string
	Argv       []string
}

func runCommand(opts runOptions) error {
	if len(opts.Argv) == 0 {
		return usageErrorf("usage: hostmux run [--name NAME]... [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix] -- COMMAND [ARGS...]")
	}

	if err := validateExplicitNames(opts.Names); err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return exitError{code: 2}
	}

	names, err := resolveRequestedNames(opts.Names)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return exitError{code: 1}
	}

	hosts, err := resolveRequestedHosts(names, hostResolveOptions{
		Domain:   opts.Domain,
		Prefix:   opts.Prefix,
		NoPrefix: opts.NoPrefix,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return exitError{code: 1}
	}

	// Resolve socket path and ensure daemon is running.
	sockOpts := sockpath.Options{Flag: opts.SocketPath}
	sockPath, err := sockpath.Resolve(sockOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return exitError{code: 1}
	}
	if !sockpath.IsExplicit(sockOpts) {
		// Same budget as `hostmux start`: child runs full daemon init before the socket accepts.
		ctx, cancel := context.WithTimeout(context.Background(), daemon.DefaultEnsureTimeout)
		if err := daemon.EnsureRunning(ctx, sockPath, daemon.EnsureOpts{}); err != nil {
			cancel()
			fmt.Fprintf(os.Stderr, "hostmux run: could not start daemon: %v\n", err)
			return exitError{code: 1}
		}
		cancel()
	} else {
		if _, err := os.Stat(sockPath); err != nil {
			fmt.Fprintf(os.Stderr, "hostmux run: socket %s not reachable; start hostmux first with `hostmux start`\n", sockPath)
			return exitError{code: 1}
		}
	}

	// Allocate free port BEFORE starting the child so we can register it first.
	port, err := childproc.AllocateFreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return exitError{code: 1}
	}

	// Connect & register.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: dial %s: %v\n", sockPath, err)
		return exitError{code: 1}
	}
	defer conn.Close()
	enc := sockproto.NewEncoder(conn)
	dec := sockproto.NewDecoder(conn)
	daemonDomain, publicHTTPS, err := lookupDaemonInfoClient(enc, dec)
	if hostnames.HasBare(hosts) {
		if err == nil {
			hosts = hostnames.Expand(hosts, daemonDomain)
		} else {
			fmt.Fprintf(os.Stderr, "hostmux run: %v; using bare hosts unchanged\n", err)
		}
	}
	// HOSTMUX_URL uses the first registered hostname only. Omit the variable
	// entirely unless OpInfo succeeded—otherwise bare-host fallback could
	// produce useless values like "https://api" with no domain.
	printScheme := "http"
	if err == nil && publicHTTPS {
		printScheme = "https"
	}
	var publicURL string
	if err == nil && len(hosts) > 0 {
		publicURL = (&url.URL{Scheme: printScheme, Host: hosts[0]}).String()
	}
	upstream := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpRegister, Hosts: hosts, Upstream: upstream}); err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: register: %v\n", err)
		return exitError{code: 1}
	}
	regResp, err := dec.Decode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: register response: %v\n", err)
		return exitError{code: 1}
	}
	if !regResp.Ok {
		fmt.Fprintf(os.Stderr, "hostmux run: register rejected: %s\n", regResp.Error)
		return exitError{code: 1}
	}

	// Tell the user where to hit (full URL so terminals linkify it).
	for _, h := range hosts {
		edge := (&url.URL{Scheme: printScheme, Host: h}).String()
		fmt.Fprintf(os.Stderr, "→ %s → %s\n", edge, upstream)
	}

	// Run the child to completion.
	code, err := childproc.Run(context.Background(), childproc.RunOpts{
		Port:       port,
		Host:       "127.0.0.1",
		HostmuxURL: publicURL,
		Argv:       opts.Argv,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: child: %v\n", err)
		return exitError{code: 1}
	}
	if code != 0 {
		return exitError{code: code}
	}
	return nil
}

func validateExplicitNames(names []string) error {
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("--name must be non-empty")
		}
		for _, r := range name {
			isAlphaNum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
			switch {
			case isAlphaNum:
			case r == '-' || r == '.' || r == ':' || r == '[' || r == ']':
			default:
				return fmt.Errorf("--name must be a valid bare label, hostname, or IP literal")
			}
		}
	}
	return nil
}

func resolvePrefix(flagValue string, disable bool) (string, error) {
	if disable {
		return "", nil
	}
	if flagValue != "" {
		return flagValue, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return worktree.Detect(cwd)
}
