package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/filelock"
)

// persistExposedRoute appends an [[app]] block for the given (already
// domain-expanded) hosts and upstream to the config file at configPath
// (resolved to the default when empty). The write is validated and atomic: it
// renders the updated file to a temporary path, runs the same validation as
// `hostmux config check`, and only renames it into place if that passes — so a
// bad append never corrupts a working config. Returns the path written.
func persistExposedRoute(configPath string, hosts []string, upstream string, labels map[string]string) (string, error) {
	path := resolveConfigPath(configPath)
	if path == "" {
		return "", fmt.Errorf("could not determine a config path; pass --config")
	}
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create config dir: %w", err)
		}
	}

	// Serialize writers via an exclusive lock held across read→render→check→
	// rename, so two concurrent `expose --persist` runs can't each validate
	// against the same original and have the last rename lose the other's
	// route (a lost-update race).
	lf, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", fmt.Errorf("open config lock: %w", err)
	}
	defer lf.Close()
	if err := filelock.Lock(lf); err != nil {
		return "", fmt.Errorf("lock config: %w", err)
	}
	defer func() { _ = filelock.Unlock(lf) }()

	// Resolve symlinks so we write through to the real file (matching the
	// config watcher, which watches the resolved target). Otherwise renaming
	// over `path` would replace the symlink and orphan its target.
	target := path
	if resolved, rerr := filepath.EvalSymlinks(path); rerr == nil {
		target = resolved
	}
	targetDir := filepath.Dir(target)

	existing, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", target, err)
	}

	block, err := renderAppBlock(hosts, upstream, labels)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if len(existing) > 0 {
		buf.Write(existing)
		if !bytes.HasSuffix(existing, []byte("\n")) {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(block)

	// Write to a temp file in the target's directory, validate, then rename.
	tmp, err := os.CreateTemp(targetDir, ".hostmux-*.toml")
	if err != nil {
		return "", fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("write temp config: %w", err)
	}

	if _, diags := config.Check(tmpName); hasCheckError(diags) {
		return "", fmt.Errorf("persisting this route would make the config invalid (likely a duplicate host); config left unchanged: %s", diagSummary(diags))
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", fmt.Errorf("replace config: %w", err)
	}
	return path, nil
}

// renderAppBlock marshals a single [[app]] block as valid TOML.
func renderAppBlock(hosts []string, upstream string, labels map[string]string) (string, error) {
	var buf bytes.Buffer
	doc := struct {
		App []config.App `toml:"app"`
	}{App: []config.App{{Hosts: hosts, Upstream: upstream, Labels: labels}}}
	if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
		return "", fmt.Errorf("encode app block: %w", err)
	}
	return buf.String(), nil
}

func diagSummary(diags []config.Diagnostic) string {
	var msgs []string
	for _, d := range diags {
		if d.Severity == config.SeverityError {
			msgs = append(msgs, d.Message)
		}
	}
	return strings.Join(msgs, "; ")
}
