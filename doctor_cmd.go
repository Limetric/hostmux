package main

import "github.com/spf13/cobra"

var doctorRunner = runDoctor

func newDoctorCmd() *cobra.Command {
	opts := doctorOptions{}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose hostmux setup and runtime problems",
		Long: `Run setup and runtime diagnostics: config validity, socket and daemon
reachability, TLS certificate presence and expiry, and common URL/tunnel
mismatches. Exits non-zero when any error-level problem is found.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux doctor [--config PATH] [--json]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return doctorRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file")
	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "output findings as JSON")
	return cmd
}
