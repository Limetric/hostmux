package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func resolveRequestedNames(explicit []string) ([]string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return resolveRequestedNamesInDir(wd, explicit)
}

func resolveRequestedNamesInDir(dir string, explicit []string) ([]string, error) {
	if len(explicit) > 0 {
		return explicit, nil
	}
	dir = canonicalPath(dir)

	gitRoot, _ := gitTopLevel(dir)

	if name, err := inferPackageName(dir, gitRoot); err != nil {
		return nil, err
	} else if name != "" {
		return []string{name}, nil
	}
	if name := normalizeInferredName(filepath.Base(gitRoot)); name != "" {
		return []string{name}, nil
	}
	if name := normalizeInferredName(filepath.Base(dir)); name != "" {
		return []string{name}, nil
	}

	return nil, fmt.Errorf("could not infer a project name; pass --name explicitly")
}

func inferPackageName(dir, stop string) (string, error) {
	path, ok := findNearestPackageJSON(dir, stop)
	if !ok {
		return "", nil
	}

	var pkg struct {
		Name json.RawMessage `json:"name"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", nil
	}
	var name string
	if err := json.Unmarshal(pkg.Name, &name); err != nil {
		return "", nil
	}
	return normalizeInferredName(name), nil
}

func findNearestPackageJSON(dir, stop string) (string, bool) {
	for {
		path := filepath.Join(dir, "package.json")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
		if dir == stop {
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func gitTopLevel(dir string) (string, bool) {
	root, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", false
	}
	return canonicalPath(root), true
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func normalizeInferredName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(name, "@") {
		if _, rest, ok := strings.Cut(name, "/"); ok {
			name = rest
		}
	}

	var b strings.Builder
	prevDash := false
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}
