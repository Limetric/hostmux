package main

import (
	"os"

	"github.com/spf13/cobra"
)

var shareRunner = runShare

func newShareCmd() *cobra.Command {
	opts := shareOptions{}
	var names []string
	var qr, noQR bool

	cmd := &cobra.Command{
		Use:   "share [NAME]...",
		Short: "Print a route's public URL, details, and an optional QR code",
		Long: `Resolve a route's public URL (the same way "hostmux url" does), report
whether it is currently registered, and optionally render a terminal QR code
for scanning from a phone.

A QR code is shown by default when stdout is a terminal; use --qr / --no-qr to
force it on or off.`,
		Example: "  hostmux share api\n  hostmux share --qr --name api\n  hostmux share --all",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Names = append(append([]string(nil), names...), args...)
			opts.Writer = cmd.OutOrStdout()
			// Default QR on for interactive terminals; flags override.
			opts.QR = !opts.JSON && isTerminal(os.Stdout)
			if qr {
				opts.QR = true
			}
			if noQR {
				opts.QR = false
			}
			return shareRunner(opts)
		},
	}
	cmd.Flags().StringArrayVar(&names, "name", nil, "repeatable hostname to share (same as positional NAME)")
	cmd.Flags().BoolVar(&opts.All, "all", false, "share every registered route")
	cmd.Flags().BoolVar(&qr, "qr", false, "always render a QR code")
	cmd.Flags().BoolVar(&noQR, "no-qr", false, "never render a QR code")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "output as JSON")
	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "expand bare subdomains using this base domain")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "explicit hostname prefix (overrides worktree detection)")
	cmd.Flags().BoolVar(&opts.NoPrefix, "no-prefix", false, "disable worktree auto-prefixing")
	return cmd
}
