package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type exitError struct {
	code int
	text string
}

func (e exitError) Error() string {
	if e.text != "" {
		return e.text
	}
	return fmt.Sprintf("exit %d", e.code)
}

func usageErrorf(format string, args ...any) error {
	return exitError{code: 2, text: fmt.Sprintf(format, args...)}
}

func legacyExit(fn func([]string) int, args []string) error {
	if code := fn(args); code != 0 {
		return exitError{code: code}
	}
	return nil
}

func execute() int {
	err := newRootCmd().Execute()
	if err == nil {
		return 0
	}

	var exitErr exitError
	if errors.As(err, &exitErr) {
		if exitErr.text != "" {
			fmt.Fprintln(os.Stderr, exitErr.text)
		}
		return exitErr.code
	}

	if normalized, ok := normalizeCLIError(err); ok {
		fmt.Fprintln(os.Stderr, normalized.text)
		return normalized.code
	}

	fmt.Fprintln(os.Stderr, err)
	return 1
}

func newRootCmd() *cobra.Command {
	var showVersion bool

	cmd := &cobra.Command{
		Use:           "hostmux",
		Short:         "Host-routed reverse proxy",
		Long:          "hostmux manages host-routed local development routes.",
		Example:       "  hostmux start\n  hostmux start --foreground\n  hostmux run HOSTS -- COMMAND [ARGS...]\n  hostmux url HOST\n  hostmux routes\n  hostmux stop\n  hostmux version",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "hostmux %s\n", Version)
				return err
			}
			return usageErrorf("usage: hostmux <command>\nRun 'hostmux help' for usage.")
		},
	}
	cmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Print the build version")
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageErrorf("hostmux: %s\nRun 'hostmux help' for usage.", err)
	})

	cmd.AddCommand(
		newStartCmd(),
		newRunCmd(),
		newURLCmd(),
		newRoutesCmd(),
		newStopCmd(),
		newVersionCmd(),
	)

	return cmd
}

func normalizeCLIError(err error) (exitError, bool) {
	if err == nil {
		return exitError{}, false
	}

	message := err.Error()
	if strings.HasPrefix(message, "unknown command ") {
		unknown := strings.TrimPrefix(message, "unknown command ")
		if idx := strings.Index(unknown, " for "); idx >= 0 {
			unknown = unknown[:idx]
		}
		return exitError{code: 2, text: fmt.Sprintf("hostmux: unknown subcommand %s\nRun 'hostmux help' for usage.", unknown)}, true
	}

	return exitError{}, false
}
