package main

import "github.com/spf13/cobra"

type startOptions struct {
	ConfigPath string
	SocketPath string
	Force      bool
	Foreground bool
}

var startRunner = runStart

func newStartCmd() *cobra.Command {
	opts := startOptions{}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the hostmux daemon",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux start [--config PATH] [--socket PATH] [--force] [--foreground]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return startRunner(opts)
		},
	}

	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file (optional)")
	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "stop any existing daemon on this socket and take over")
	cmd.Flags().BoolVar(&opts.Foreground, "foreground", false, "run in the foreground instead of daemonizing")

	return cmd
}
