package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateHomebrewFormula(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required to test the Homebrew formula generator")
	}

	script := filepath.ToSlash(filepath.Join("scripts", "generate-homebrew-formula.sh"))
	cmd := exec.Command("bash", script,
		"v1.2.3",
		"darwin-amd64-sha",
		"darwin-arm64-sha",
		"linux-amd64-sha",
		"linux-arm64-sha",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", script, err, out)
	}

	formula := string(out)
	wantSubstrings := []string{
		"class Hostmux < Formula",
		`desc "Local host-based reverse proxy for development"`,
		`homepage "https://github.com/Limetric/hostmux"`,
		`version "1.2.3"`,
		`license "Apache-2.0"`,
		`url "https://github.com/Limetric/hostmux/releases/download/v1.2.3/hostmux-darwin-arm64"`,
		`sha256 "darwin-arm64-sha"`,
		`url "https://github.com/Limetric/hostmux/releases/download/v1.2.3/hostmux-darwin-amd64"`,
		`sha256 "darwin-amd64-sha"`,
		`url "https://github.com/Limetric/hostmux/releases/download/v1.2.3/hostmux-linux-arm64"`,
		`sha256 "linux-arm64-sha"`,
		`url "https://github.com/Limetric/hostmux/releases/download/v1.2.3/hostmux-linux-amd64"`,
		`sha256 "linux-amd64-sha"`,
		`bin.install binary => "hostmux"`,
		`system "#{bin}/hostmux", "version"`,
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(formula, want) {
			t.Fatalf("formula missing %q\nformula:\n%s", want, formula)
		}
	}
}
