package main

import "github.com/spf13/cobra"

func newURLCmd() *cobra.Command {
	opts := urlOptions{}

	cmd := &cobra.Command{
		Use:   "url HOST",
		Short: "Print the public URL for a host",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageErrorf("usage: hostmux url HOST [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.HostArg = args[0]
			opts.Writer = cmd.OutOrStdout()
			return runURL(opts)
		},
	}

	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path for daemon domain lookup")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "expand bare subdomains using this base domain")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "explicit hostname prefix (overrides worktree detection)")
	cmd.Flags().BoolVar(&opts.NoPrefix, "no-prefix", false, "disable worktree auto-prefixing")

	return cmd
}
