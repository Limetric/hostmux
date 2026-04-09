package main

import "github.com/spf13/cobra"

var stopRunner = runStop

func newStopCmd() *cobra.Command {
	opts := stopOptions{}

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the hostmux daemon",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux stop [--socket PATH]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return stopRunner(opts)
		},
	}

	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path")

	return cmd
}
