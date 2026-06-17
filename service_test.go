package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeService wires the service seams to a temp home, fake binary, and a
// recording runner, and restores them after the test. The returned bin path
// is OS-absolute (under a temp dir) so it survives filepath.Abs unchanged on
// every platform.
func fakeService(t *testing.T, goos, runnerOut string) (home, bin string, calls *[]string) {
	t.Helper()
	home = t.TempDir()
	bin = filepath.Join(t.TempDir(), "hostmux")
	recorded := &[]string{}
	origGOOS, origHome, origBin, origRun := serviceGOOS, serviceHomeDir, serviceBinPath, serviceRunner
	serviceGOOS = func() string { return goos }
	serviceHomeDir = func() (string, error) { return home, nil }
	serviceBinPath = func() (string, error) { return bin, nil }
	serviceRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		*recorded = append(*recorded, name+" "+strings.Join(args, " "))
		return []byte(runnerOut), nil
	}
	t.Cleanup(func() {
		serviceGOOS, serviceHomeDir, serviceBinPath, serviceRunner = origGOOS, origHome, origBin, origRun
	})
	return home, bin, recorded
}

func TestServiceInstallDarwin(t *testing.T) {
	home, bin, calls := fakeService(t, "darwin", "")
	var buf bytes.Buffer
	if err := runServiceInstall(serviceOptions{Writer: &buf}); err != nil {
		t.Fatalf("install: %v", err)
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", "com.limetric.hostmux.plist")
	body, err := os.ReadFile(plist)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if !strings.Contains(string(body), bin) {
		t.Fatalf("plist missing binary path %q:\n%s", bin, body)
	}
	if !containsCall(*calls, "launchctl load -w") {
		t.Fatalf("expected launchctl load, got %v", *calls)
	}
}

func TestServiceInstallLinux(t *testing.T) {
	home, bin, calls := fakeService(t, "linux", "")
	var buf bytes.Buffer
	if err := runServiceInstall(serviceOptions{Writer: &buf}); err != nil {
		t.Fatalf("install: %v", err)
	}
	unit := filepath.Join(home, ".config", "systemd", "user", "hostmux.service")
	body, err := os.ReadFile(unit)
	if err != nil {
		t.Fatalf("unit not written: %v", err)
	}
	if !strings.Contains(string(body), "ExecStart="+bin+" start --foreground") {
		t.Fatalf("unit ExecStart wrong (bin %q):\n%s", bin, body)
	}
	if !containsCall(*calls, "systemctl --user daemon-reload") || !containsCall(*calls, "systemctl --user enable --now hostmux.service") {
		t.Fatalf("expected systemctl calls, got %v", *calls)
	}
}

func TestServiceStatusLinux(t *testing.T) {
	home, _, _ := fakeService(t, "linux", "active\n")
	// Pre-create the unit file so installed=true.
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "hostmux.service"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runServiceStatus(serviceOptions{Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "installed:    true") || !strings.Contains(out, "running:      true") {
		t.Fatalf("status output = %q", out)
	}
}

func TestServiceUninstallRemovesFile(t *testing.T) {
	home, _, calls := fakeService(t, "darwin", "")
	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(plistDir, "com.limetric.hostmux.plist")
	if err := os.WriteFile(plist, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runServiceUninstall(serviceOptions{Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Fatal("plist should be removed")
	}
	if !containsCall(*calls, "launchctl unload") {
		t.Fatalf("expected launchctl unload, got %v", *calls)
	}
}

func TestServiceWindowsUnsupported(t *testing.T) {
	fakeService(t, "windows", "")
	err := runServiceInstall(serviceOptions{Writer: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("expected Windows-unsupported message, got %v", err)
	}
}

func containsCall(calls []string, sub string) bool {
	for _, c := range calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}
