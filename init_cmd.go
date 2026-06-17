package main

import "github.com/spf13/cobra"

var initRunner = runInit

func newInitCmd() *cobra.Command {
	opts := initOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter hostmux config file",
		Long: `Write a starter hostmux config to the standard location (or --config).

Flags drive the values so init is scriptable; an existing file is never
overwritten without --force. Use --tunnel for the Cloudflare workflow (sets
hide_port and prints a matching cloudflared ingress snippet).`,
		Example: "  hostmux init --domain example.com\n  hostmux init --tunnel --domain example.com\n  hostmux init --domain localhost --listen :8443",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux init [--domain DOMAIN] [--listen ADDR] [--hide-port] [--tunnel] [--force]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return initRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "config path to write (default: standard location)")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "base domain (default: localhost)")
	cmd.Flags().StringVar(&opts.Listen, "listen", "", "TLS listen address (default: :8443)")
	cmd.Flags().BoolVar(&opts.HidePort, "hide-port", false, "omit the listener port from printed URLs")
	cmd.Flags().BoolVar(&opts.Tunnel, "tunnel", false, "Cloudflare workflow: set hide_port and print an ingress snippet")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite an existing config file")
	return cmd
}
