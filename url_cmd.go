package main

import "github.com/spf13/cobra"

var urlRunner = runURL

func newURLCmd() *cobra.Command {
	opts := urlOptions{}
	var names []string

	cmd := &cobra.Command{
		Use:   "url [NAME]... [--name NAME]...",
		Short: "Print the public URL for a host",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Names = append(append([]string(nil), names...), args...)
			opts.Writer = cmd.OutOrStdout()
			return urlRunner(opts)
		},
	}

	cmd.Flags().StringArrayVar(&names, "name", nil, "repeatable hostname to print (same as positional NAME)")
	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path for daemon domain lookup")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "expand bare subdomains using this base domain")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "explicit hostname prefix (overrides worktree detection)")
	cmd.Flags().BoolVar(&opts.NoPrefix, "no-prefix", false, "disable worktree auto-prefixing")

	return cmd
}
