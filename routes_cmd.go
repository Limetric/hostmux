package main

import "github.com/spf13/cobra"

var routesRunner = runRoutes

func newRoutesCmd() *cobra.Command {
	opts := routesOptions{}

	cmd := &cobra.Command{
		Use:   "routes",
		Short: "List registered routes",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux routes [--socket PATH]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return routesRunner(opts)
		},
	}

	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path")

	return cmd
}
