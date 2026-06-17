package main

import "github.com/spf13/cobra"

var trustRunner = runTrust

func newTrustCmd() *cobra.Command {
	opts := trustOptions{}
	cmd := &cobra.Command{
		Use:   "trust",
		Short: "Install hostmux's TLS certificate into the OS trust store",
		Long: `Install the managed hostmux TLS certificate into the operating system
trust store so browsers and HTTP clients accept https://*.<domain> without
manual exceptions. Supported on macOS, Linux, and Windows.

Linux and the macOS system store may prompt for elevation. The command is
idempotent: it exits 0 if the certificate is already trusted unless --force.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux trust [--config PATH] [--force]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			opts.Remove = false
			return trustRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "re-import even if already trusted")
	return cmd
}

func newUntrustCmd() *cobra.Command {
	opts := trustOptions{}
	cmd := &cobra.Command{
		Use:   "untrust",
		Short: "Remove hostmux's TLS certificate from the OS trust store",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux untrust [--config PATH]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			opts.Remove = true
			return trustRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file")
	return cmd
}
