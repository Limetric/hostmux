package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newCompletionCmd provides a self-managed completion command. Cobra's
// auto-generated one stays disabled (root sets DisableDefaultCmd) so this is
// the single, documented entry point. The hidden "__complete" runtime engine
// is wired up by Cobra independently, so generated scripts still work.
func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate a shell completion script",
		Long: `Generate a shell completion script for hostmux.

Load it for your shell:

Bash:
  # current shell
  source <(hostmux completion bash)
  # persist (Linux)
  hostmux completion bash > /etc/bash_completion.d/hostmux
  # persist (macOS, Homebrew)
  hostmux completion bash > $(brew --prefix)/etc/bash_completion.d/hostmux

Zsh:
  # ensure 'autoload -U compinit; compinit' is in ~/.zshrc, then:
  hostmux completion zsh > "${fpath[1]}/_hostmux"

Fish:
  hostmux completion fish > ~/.config/fish/completions/hostmux.fish

PowerShell:
  hostmux completion powershell | Out-String | Invoke-Expression
  # persist by adding the line above to your PowerShell profile`,
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
	return cmd
}
