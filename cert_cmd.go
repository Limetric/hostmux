package main

import "github.com/spf13/cobra"

var (
	certInfoRunner  = runCertInfo
	certRenewRunner = runCertRenew
	certPathRunner  = runCertPath
)

func newCertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cert",
		Short: "Inspect and manage hostmux TLS certificates",
	}
	cmd.AddCommand(newCertInfoCmd(), newCertRenewCmd(), newCertPathCmd())
	return cmd
}

func newCertInfoCmd() *cobra.Command {
	opts := certOptions{}
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show certificate subject, SANs, validity, and expiry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return certInfoRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "output as JSON")
	return cmd
}

func newCertRenewCmd() *cobra.Command {
	opts := certOptions{}
	cmd := &cobra.Command{
		Use:   "renew",
		Short: "Regenerate the managed certificate",
		Long: `Regenerate the managed hostmux certificate in place. Restart the daemon
afterward to serve it. Refuses to overwrite a custom tls.cert unless --force.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return certRenewRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "renew even when a custom tls.cert is configured")
	return cmd
}

func newCertPathCmd() *cobra.Command {
	opts := certOptions{}
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Print the resolved certificate and key paths",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return certPathRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file")
	return cmd
}
