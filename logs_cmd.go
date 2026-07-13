package main

import "github.com/spf13/cobra"

var logsRunner = runLogs

func newLogsCmd() *cobra.Command {
	opts := logsOptions{}

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Print the detached daemon's log (~/.hostmux/hostmux.log)",
		Long: "Print the log of the background daemon started by `hostmux start`.\n" +
			"Useful when a detached start fails or to inspect access/proxy logs.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux logs [-f] [-n LINES]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return logsRunner(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Follow, "follow", "f", false, "stream new log lines as they are written")
	cmd.Flags().IntVarP(&opts.Lines, "lines", "n", 0, "print only the last N lines (0 = whole file)")

	return cmd
}
