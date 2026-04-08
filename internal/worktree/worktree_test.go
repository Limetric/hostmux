package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func setupRepoWithWorktree(t *testing.T) (mainDir, worktreeDir string) {
	t.Helper()
	root := t.TempDir()
	mainDir = filepath.Join(root, "main")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, mainDir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(mainDir, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, mainDir, "add", "README")
	mustGit(t, mainDir, "commit", "-q", "-m", "init")

	worktreeDir = filepath.Join(root, "feature-x")
	mustGit(t, mainDir, "worktree", "add", "-q", worktreeDir, "-b", "feature-x")
	return mainDir, worktreeDir
}

func TestDetectInPrimaryWorktreeReturnsEmpty(t *testing.T) {
	main, _ := setupRepoWithWorktree(t)
	got, err := Detect(main)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != "" {
		t.Fatalf("primary worktree got prefix %q, want empty", got)
	}
}

func TestDetectInSecondaryWorktreeReturnsName(t *testing.T) {
	_, wt := setupRepoWithWorktree(t)
	got, err := Detect(wt)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != "feature-x" {
		t.Fatalf("got %q want feature-x", got)
	}
}

func TestDetectOutsideGitReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	got, err := Detect(tmp)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != "" {
		t.Fatalf("non-git dir got prefix %q", got)
	}
}
