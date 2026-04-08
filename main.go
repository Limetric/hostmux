package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	sub := os.Args[1]
	args := os.Args[2:]
	var code int
	switch sub {
	case "start":
		code = cmdStart(args)
	case "serve":
		code = cmdServe(args)
	case "run":
		code = cmdRun(args)
	case "get":
		code = cmdGet(args)
	case "list":
		code = cmdList(args)
	case "stop":
		code = cmdStop(args)
	case "version", "-v", "--version":
		code = cmdVersion(args)
	case "help", "-h", "--help":
		usage()
		code = 0
	default:
		fmt.Fprintf(os.Stderr, "hostmux: unknown subcommand %q\n", sub)
		usage()
		code = 2
	}
	os.Exit(code)
}

func usage() {
	fmt.Fprint(os.Stderr, `hostmux — host-routed reverse proxy

usage:
  hostmux start [--config PATH] [--socket PATH] [--force] [--foreground]
  hostmux serve [--config PATH] [--socket PATH] [--force]
  hostmux run HOSTS [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix] -- COMMAND [ARGS...]
  hostmux get HOST [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix]
  hostmux list [--socket PATH]
  hostmux stop [--socket PATH]
  hostmux version
`)
}
