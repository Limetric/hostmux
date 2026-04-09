package main

import "github.com/spf13/cobra"

var runRunner = runCommand

func newRunCmd() *cobra.Command {
	opts := runOptions{}
	var names []string

	cmd := &cobra.Command{
		Use:   "run [--name NAME]... -- COMMAND [ARGS...]",
		Short: "Run a command and register its upstream",
		Args: func(cmd *cobra.Command, args []string) error {
			argsLenAtDash := cmd.ArgsLenAtDash()
			if argsLenAtDash != 0 || len(args) == 0 {
				return usageErrorf("usage: hostmux run [--name NAME]... [--socket PATH] [--domain DOMAIN] [--prefix NAME | --no-prefix] -- COMMAND [ARGS...]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLenAtDash := cmd.ArgsLenAtDash()
			opts.Names = append([]string(nil), names...)
			opts.Argv = append([]string(nil), args[argsLenAtDash:]...)
			return runRunner(opts)
		},
	}

	cmd.Flags().StringArrayVar(&names, "name", nil, "repeatable hostname to register")
	cmd.Flags().StringVar(&opts.SocketPath, "socket", "", "override Unix socket path")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "expand bare subdomains using this base domain")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "explicit hostname prefix (overrides worktree detection)")
	cmd.Flags().BoolVar(&opts.NoPrefix, "no-prefix", false, "disable worktree auto-prefixing")

	return cmd
}
