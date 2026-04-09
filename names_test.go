package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveRequestedNamesReturnsExplicitNamesUnchanged(t *testing.T) {
	explicit := []string{"API", " Admin Service "}

	got, err := resolveRequestedNamesInDir(t.TempDir(), explicit)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if !reflect.DeepEqual(got, explicit) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, explicit)
	}
}

func TestResolveRequestedNamesUsesNearestPackageJSON(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "apps", "web", "src"))
	mustWriteFile(t, filepath.Join(root, "package.json"), `{"name":"root-app"}`)
	mustWriteFile(t, filepath.Join(root, "apps", "web", "package.json"), `{"name":"@scope/Web App"}`)
	mustRunGit(t, root, "init")

	got, err := resolveRequestedNamesInDir(filepath.Join(root, "apps", "web", "src"), nil)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if want := []string{"web-app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, want)
	}
}

func TestResolveRequestedNamesUsesGitRepoRootBasename(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Repo.Name")
	mustMkdirAll(t, filepath.Join(root, "pkg"))
	mustRunGit(t, root, "init")

	got, err := resolveRequestedNamesInDir(filepath.Join(root, "pkg"), nil)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if want := []string{"repo-name"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, want)
	}
}

func TestResolveRequestedNamesIgnoresInvalidPackageJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Repo.Name")
	mustMkdirAll(t, filepath.Join(root, "pkg"))
	mustWriteFile(t, filepath.Join(root, "pkg", "package.json"), `{not json}`)
	mustRunGit(t, root, "init")

	got, err := resolveRequestedNamesInDir(filepath.Join(root, "pkg"), nil)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if want := []string{"repo-name"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, want)
	}
}

func TestResolveRequestedNamesIgnoresUnusablePackageName(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "My App!!!")
	mustMkdirAll(t, wd)
	mustWriteFile(t, filepath.Join(wd, "package.json"), `{"name":123}`)

	got, err := resolveRequestedNamesInDir(wd, nil)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if want := []string{"my-app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, want)
	}
}

func TestResolveRequestedNamesFallsBackToCWDBasename(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "My App!!!")
	mustMkdirAll(t, wd)

	got, err := resolveRequestedNamesInDir(wd, nil)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if want := []string{"my-app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, want)
	}
}

func TestResolveRequestedNamesDoesNotWalkPastStartDirOutsideGit(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	wd := filepath.Join(workspace, "My App!!!")
	mustMkdirAll(t, wd)
	mustWriteFile(t, filepath.Join(workspace, "package.json"), `{"name":"outer-workspace"}`)

	got, err := resolveRequestedNamesInDir(wd, nil)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if want := []string{"my-app"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, want)
	}
}

func TestResolveRequestedNamesDoesNotPrintGitErrorsOutsideRepo(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "My App!!!")
	mustMkdirAll(t, wd)

	stderr, restoreStderr := captureRootFileOutput(t, &os.Stderr)
	_, err := resolveRequestedNamesInDir(wd, nil)
	restoreStderr()
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr", stderr.String())
	}
}

func TestResolveRequestedNamesErrorsWhenInferenceNormalizesEmpty(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "---")
	mustMkdirAll(t, wd)
	mustWriteFile(t, filepath.Join(wd, "package.json"), `{"name":"@@@"}`)

	_, err := resolveRequestedNamesInDir(wd, nil)
	if err == nil {
		t.Fatal("resolveRequestedNames() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("resolveRequestedNames() error = %q, want mention of --name", err)
	}
}

func TestResolveRequestedNamesDoesNotEscapeGitRootForPackageJSON(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(workspace, "Repo.Name")
	mustMkdirAll(t, filepath.Join(repo, "pkg"))
	mustWriteFile(t, filepath.Join(workspace, "package.json"), `{"name":"outer-workspace"}`)
	mustRunGit(t, repo, "init")

	got, err := resolveRequestedNamesInDir(filepath.Join(repo, "pkg"), nil)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if want := []string{"repo-name"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, want)
	}
}

func TestResolveRequestedNamesNormalizesToASCIISafeHostLabel(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "proj")
	mustMkdirAll(t, wd)
	mustWriteFile(t, filepath.Join(wd, "package.json"), `{"name":"@scope/Caf\u00e9 東京 123"}`)

	got, err := resolveRequestedNamesInDir(wd, nil)
	if err != nil {
		t.Fatalf("resolveRequestedNames() error = %v", err)
	}
	if want := []string{"caf-123"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveRequestedNames() = %v, want %v", got, want)
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v error = %v, stderr = %q", args, err, stderr.String())
	}
}
