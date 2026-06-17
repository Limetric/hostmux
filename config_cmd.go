package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Limetric/hostmux/internal/config"
)

type configCheckOptions struct {
	ConfigPath string
	JSON       bool
	Writer     io.Writer
}

var configCheckRunner = runConfigCheck

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and validate hostmux configuration",
	}
	cmd.AddCommand(newConfigCheckCmd())
	return cmd
}

func newConfigCheckCmd() *cobra.Command {
	opts := configCheckOptions{}
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate the hostmux config file without starting the daemon",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("usage: hostmux config check [--config PATH] [--json]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Writer = cmd.OutOrStdout()
			return configCheckRunner(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to TOML config file (default: standard location)")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "output diagnostics as JSON")
	return cmd
}

// configCheckResult is the stable JSON shape of `config check --json`.
type configCheckResult struct {
	Path        string              `json:"path"`
	OK          bool                `json:"ok"`
	Diagnostics []config.Diagnostic `json:"diagnostics"`
}

func runConfigCheck(opts configCheckOptions) error {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	path := resolveConfigPath(opts.ConfigPath)
	if path == "" {
		return exitError{code: 2, text: "hostmux config check: no config path (--config unset and default location unavailable)"}
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return exitError{code: 2, text: fmt.Sprintf("hostmux config check: config file not found: %s", path)}
		}
		return exitError{code: 1, text: fmt.Sprintf("hostmux config check: %v", err)}
	}

	_, diags := config.Check(path)
	hasError := false
	for _, d := range diags {
		if d.Severity == config.SeverityError {
			hasError = true
			break
		}
	}

	if opts.JSON {
		res := configCheckResult{Path: path, OK: !hasError, Diagnostics: diags}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			return err
		}
		if hasError {
			return exitError{code: 1}
		}
		return nil
	}

	for _, d := range diags {
		fmt.Fprintf(w, "%-7s %s\n", d.Severity, d.Message)
	}
	if hasError {
		fmt.Fprintf(w, "FAIL %s\n", path)
		return exitError{code: 1}
	}
	if len(diags) == 0 {
		fmt.Fprintf(w, "OK %s\n", path)
	} else {
		fmt.Fprintf(w, "OK %s (with warnings)\n", path)
	}
	return nil
}
