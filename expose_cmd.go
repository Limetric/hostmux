package main

import "github.com/spf13/cobra"

var (
	exposeRunner   = runExpose
	unexposeRunner = runUnexpose
)

func newExposeCmd() *cobra.Command {
	opts := exposeOptions{}
	var names []string

	cmd := &cobra.Command{
		Use:   "expose --name NAME --upstream URL [--name NAME]...",
		Short: "Register a route for an already-running upstream",
		Long: `Register a host-based route for an upstream you started yourself
(IDE, Docker Compose, a manually launched dev server). Unlike "hostmux run",
no child process is started and the route persists until "hostmux unexpose"
removes it or the daemon restarts.

The first --name is the route's identifier for "hostmux unexpose".`,
		Example: "  hostmux expose --name api --upstream http://127.0.0.1:3000\n  hostmux expose --domain example.com --name admin --upstream http://127.0.0.1:9000",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux expose --name NAME --upstream URL")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Names = append([]string(nil), names...)
			opts.Writer = cmd.OutOrStdout()
			return exposeRunner(opts)
		},
	}
	cmd.Flags().StringArrayVar(&names, "name", nil, "repeatable hostname to register (first is the route id)")
	cmd.Flags().StringVar(&opts.Upstream, "upstream", "", "absolute upstream URL (e.g. http://127.0.0.1:3000)")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "expand bare subdomains using this base domain")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "repeatable key=value metadata attached to the route")
	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path")
	return cmd
}

func newUnexposeCmd() *cobra.Command {
	opts := unexposeOptions{}
	cmd := &cobra.Command{
		Use:     "unexpose NAME",
		Short:   "Remove a route created with hostmux expose",
		Example: "  hostmux unexpose api",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageErrorf("usage: hostmux unexpose NAME")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			return unexposeRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path")
	return cmd
}
