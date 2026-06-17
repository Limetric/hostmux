package main

import "github.com/spf13/cobra"

var (
	serviceInstallRunner   = runServiceInstall
	serviceUninstallRunner = runServiceUninstall
	serviceStatusRunner    = runServiceStatus
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Install hostmux as a user service (macOS launchd, Linux systemd)",
	}
	cmd.AddCommand(newServiceInstallCmd(), newServiceUninstallCmd(), newServiceStatusCmd())
	return cmd
}

func newServiceInstallCmd() *cobra.Command {
	opts := serviceOptions{}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and start the hostmux user service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return serviceInstallRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "config path baked into the service (default: standard location)")
	return cmd
}

func newServiceUninstallCmd() *cobra.Command {
	opts := serviceOptions{}
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the hostmux user service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return serviceUninstallRunner(opts)
		},
	}
	return cmd
}

func newServiceStatusCmd() *cobra.Command {
	opts := serviceOptions{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report whether the hostmux service is installed and running",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return serviceStatusRunner(opts)
		},
	}
	return cmd
}
