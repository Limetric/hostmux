package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

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
	HostsArg   string
	Argv       []string
}

func cmdRun(args []string) int {
	cmd := newRunCmd()
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil {
		return 0
	}

	var exitErr exitError
	if errors.As(err, &exitErr) {
		if exitErr.text != "" {
			fmt.Fprintln(os.Stderr, exitErr.text)
		}
		return exitErr.code
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func runCommand(opts runOptions) error {
	if opts.HostsArg == "" || len(opts.Argv) == 0 {
		return usageErrorf("usage: hostmux run HOSTS [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix] -- COMMAND [ARGS...]")
	}

	hosts := splitHosts(opts.HostsArg)
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "hostmux run: HOSTS is empty")
		return exitError{code: 2}
	}

	hosts, err := resolveRequestedHosts(hosts, hostResolveOptions{
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
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
	var publicURL string
	if err == nil {
		scheme := "http"
		if publicHTTPS {
			scheme = "https"
		}
		publicURL = scheme + "://" + hosts[0]
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

	// Tell the user where to hit.
	for _, h := range hosts {
		fmt.Fprintf(os.Stderr, "→ %s → %s\n", h, upstream)
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

func splitHosts(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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
