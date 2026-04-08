package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Limetric/hostmux/internal/daemonctl"
	"github.com/Limetric/hostmux/internal/sockpath"
)

func cmdStop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	socketFlag := fs.String("socket", "", "override Unix socket path")
	fs.Parse(args)

	sockPath, err := sockpath.Resolve(sockpath.Options{Flag: *socketFlag})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux stop: %v\n", err)
		return 1
	}
	pidPath := sockpath.PIDFilePathFor(sockPath)

	res, err := daemonctl.Stop(daemonctl.StopOptions{
		SockPath:        sockPath,
		PIDPath:         pidPath,
		GracefulTimeout: 5 * time.Second,
		KillTimeout:     2 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux stop: %v\n", err)
		return 1
	}
	if res.NotRunning {
		fmt.Fprintf(os.Stderr, "hostmux stop: no daemon running on %s\n", sockPath)
		return 0
	}
	switch {
	case res.UsedSIGKILL:
		fmt.Fprintf(os.Stderr, "hostmux stop: killed (SIGKILL) daemon on %s\n", sockPath)
	case res.UsedSocket:
		fmt.Fprintf(os.Stderr, "hostmux stop: stopped daemon on %s\n", sockPath)
	default:
		fmt.Fprintf(os.Stderr, "hostmux stop: stopped daemon on %s (SIGTERM)\n", sockPath)
	}
	return 0
}
