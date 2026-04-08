package main

import (
	"context"
	"flag"
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

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	socketFlag := fs.String("socket", "", "override Unix socket path")
	domainFlag := fs.String("domain", "", "expand bare subdomains using this base domain")
	prefixFlag := fs.String("prefix", "", "explicit hostname prefix (overrides worktree detection)")
	noPrefix := fs.Bool("no-prefix", false, "disable worktree auto-prefixing")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: hostmux run HOSTS [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix] -- COMMAND [ARGS...]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return 2
	}
	hostsArg := fs.Arg(0)
	cmdArgv := fs.Args()[1:]
	if len(cmdArgv) > 0 && cmdArgv[0] == "--" {
		cmdArgv = cmdArgv[1:]
	}
	if len(cmdArgv) == 0 {
		fs.Usage()
		return 2
	}

	hosts := splitHosts(hostsArg)
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "hostmux run: HOSTS is empty")
		return 2
	}

	hosts, err := resolveRequestedHosts(hosts, hostResolveOptions{
		Domain:   *domainFlag,
		Prefix:   *prefixFlag,
		NoPrefix: *noPrefix,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return 1
	}

	// Resolve socket path and ensure daemon is running.
	sockOpts := sockpath.Options{Flag: *socketFlag}
	sockPath, err := sockpath.Resolve(sockOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return 1
	}
	if !sockpath.IsExplicit(sockOpts) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := daemon.EnsureRunning(ctx, sockPath, daemon.EnsureOpts{}); err != nil {
			cancel()
			fmt.Fprintf(os.Stderr, "hostmux run: could not start daemon: %v\n", err)
			return 1
		}
		cancel()
	} else {
		if _, err := os.Stat(sockPath); err != nil {
			fmt.Fprintf(os.Stderr, "hostmux run: socket %s not reachable; start hostmux first with `hostmux start`\n", sockPath)
			return 1
		}
	}

	// Allocate free port BEFORE starting the child so we can register it first.
	port, err := childproc.AllocateFreePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: %v\n", err)
		return 1
	}

	// Connect & register.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: dial %s: %v\n", sockPath, err)
		return 1
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
	scheme := "http"
	if publicHTTPS {
		scheme = "https"
	}
	publicURL := scheme + "://" + hosts[0]
	upstream := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpRegister, Hosts: hosts, Upstream: upstream}); err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: register: %v\n", err)
		return 1
	}
	regResp, err := dec.Decode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: register response: %v\n", err)
		return 1
	}
	if !regResp.Ok {
		fmt.Fprintf(os.Stderr, "hostmux run: register rejected: %s\n", regResp.Error)
		return 1
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
		Argv:       cmdArgv,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux run: child: %v\n", err)
		return 1
	}
	return code
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
