package main

import (
	"context"
	"fmt"
	"net"
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
		return usageErrorf("usage: hostmux run [--name NAME]... [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix] [--] COMMAND [ARGS...]")
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
	sockPath, err := resolveRunSocketPath(opts.SocketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return exitError{code: 1}
	}
	if !sockpath.IsExplicit(sockpath.Options{Flag: opts.SocketPath}) {
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
	daemonDomain, publicHTTPS, daemonPort, err := lookupDaemonInfoClient(enc, dec)
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
		publicURL = formatPublicURL(hosts[0], printScheme, daemonPort)
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
		edge := formatPublicURL(h, printScheme, daemonPort)
		fmt.Fprintf(os.Stderr, "→ %s → %s\n", edge, upstream)
	}

	// Run the child to completion. Frameworks that ignore PORT (Vite, Astro,
	// etc.) get --port/--host injected like portless does.
	const bindHost = "127.0.0.1"
	argv := childproc.InjectFrameworkArgs(opts.Argv, port, bindHost)
	code, err := childproc.Run(context.Background(), childproc.RunOpts{
		Port:       port,
		Host:       bindHost,
		HostmuxURL: publicURL,
		Argv:       argv,
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

func resolveRunSocketPath(socketFlag string) (string, error) {
	sockOpts := sockpath.Options{Flag: socketFlag}
	if sockpath.IsExplicit(sockOpts) {
		return sockpath.Resolve(sockOpts)
	}

	if p, ok := sockpath.LiveDiscovery(); ok {
		return p, nil
	}

	cfg, err := loadOptionalConfig(defaultConfigPath())
	if err != nil {
		return "", err
	}
	if cfg != nil && cfg.Socket != "" {
		return sockpath.ResolveServe(sockpath.Options{ConfigSocket: cfg.Socket})
	}
	return sockpath.Resolve(sockOpts)
}

func validateExplicitNames(names []string) error {
	for _, name := range names {
		if name == "" {
			return fmt.Errorf("--name must be non-empty")
		}
		if !hostnames.ValidHostToken(name) {
			return fmt.Errorf("--name must be a valid bare label, hostname, or IP literal")
		}
	}
	return nil
}

func validateResolvedPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if !hostnames.ValidDNSLabel(prefix) {
		return fmt.Errorf("prefix must be a valid DNS label")
	}
	return nil
}

func resolvePrefix(flagValue string, disable bool) (prefix string, explicit bool, err error) {
	if disable {
		return "", false, nil
	}
	if flagValue != "" {
		return flagValue, true, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	detected, err := worktree.Detect(cwd)
	if err != nil {
		return "", false, err
	}
	return sanitizeWorktreePrefix(detected), false, nil
}

func sanitizeWorktreePrefix(prefix string) string {
	if prefix == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range prefix {
		isAlphaNum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		switch {
		case isAlphaNum || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "worktree"
	}
	runes := []rune(s)
	if len(runes) > 63 {
		s = strings.Trim(string(runes[:63]), "-")
	}
	return s
}
