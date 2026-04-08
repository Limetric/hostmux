// Package worktree detects whether a directory is inside a non-primary
// git worktree and returns the worktree's name. Used by `hostmux run` to
// automatically prefix hostnames in secondary worktrees so multiple
// checkouts of the same project can register distinct hostnames.
package worktree

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
)

// Detect returns the worktree name if dir is inside a non-primary git
// worktree. Returns "" for the primary worktree, "" if not in a git repo,
// and an error only on unexpected failures (not for absence of git).
func Detect(dir string) (string, error) {
	gitDir, err := runGit(dir, "rev-parse", "--git-dir")
	if err != nil {
		// Not in a git repo (or git missing) — treat as no prefix.
		return "", nil
	}
	commonDir, err := runGit(dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", nil
	}
	gitDirAbs, err := absPath(dir, gitDir)
	if err != nil {
		return "", err
	}
	commonDirAbs, err := absPath(dir, commonDir)
	if err != nil {
		return "", err
	}
	if gitDirAbs == commonDirAbs {
		return "", nil // primary worktree
	}
	return filepath.Base(gitDirAbs), nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func absPath(base, p string) (string, error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Clean(filepath.Join(base, p)), nil
}
