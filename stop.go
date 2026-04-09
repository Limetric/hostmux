package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Limetric/hostmux/internal/daemonctl"
	"github.com/Limetric/hostmux/internal/sockpath"
)

type stopOptions struct {
	SocketPath string
}

func runStop(opts stopOptions) error {
	sockPath, err := sockpath.Resolve(sockpath.Options{Flag: opts.SocketPath})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hostmux stop: %v\n", err)
		return exitError{code: 1}
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
		return exitError{code: 1}
	}
	if res.NotRunning {
		fmt.Fprintf(os.Stderr, "hostmux stop: no daemon running on %s\n", sockPath)
		return nil
	}
	switch {
	case res.UsedSIGKILL:
		fmt.Fprintf(os.Stderr, "hostmux stop: killed (SIGKILL) daemon on %s\n", sockPath)
	case res.UsedSocket:
		fmt.Fprintf(os.Stderr, "hostmux stop: stopped daemon on %s\n", sockPath)
	default:
		fmt.Fprintf(os.Stderr, "hostmux stop: stopped daemon on %s (SIGTERM)\n", sockPath)
	}
	return nil
}
