package main

import "github.com/spf13/cobra"

var cloudflareConfigRunner = runCloudflareConfig

func newCloudflareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cloudflare",
		Aliases: []string{"tunnel"},
		Short:   "Cloudflare Tunnel helpers",
	}
	cmd.AddCommand(newCloudflareConfigCmd())
	return cmd
}

func newCloudflareConfigCmd() *cobra.Command {
	opts := cloudflareOptions{}
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print a cloudflared ingress snippet for this hostmux config",
		Long: `Print a copy-pasteable cloudflared ingress block that points at the
hostmux HTTPS listener. Works offline from the config file and, when the
daemon is reachable, fills in gaps from live daemon info.`,
		Example: "  hostmux cloudflare config\n  hostmux cloudflare config --domain example.com",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux cloudflare config [--config PATH] [--domain DOMAIN]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return cloudflareConfigRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "override the base domain")
	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path for daemon lookup")
	return cmd
}
