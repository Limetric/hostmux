package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Limetric/hostmux/internal/service"
)

// Injectable seams so service operations are testable without touching the
// real launchd/systemd or the user's home directory.
var (
	serviceGOOS    = func() string { return runtime.GOOS }
	serviceHomeDir = os.UserHomeDir
	serviceBinPath = os.Executable
	serviceRunner  = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
)

type serviceOptions struct {
	ConfigPath string
	Writer     io.Writer
}

// servicePaths resolves the per-platform service file path (and macOS log
// path) from the user's home directory.
func servicePaths(goos, home string) (file, logPath string, ok bool) {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", service.DarwinLabel+".plist"),
			filepath.Join(home, "Library", "Logs", "hostmux.log"), true
	case "linux":
		return filepath.Join(home, ".config", "systemd", "user", service.LinuxUnit), "", true
	default:
		return "", "", false
	}
}

func serviceParams(opts serviceOptions, logPath string) (service.Params, error) {
	bin, err := serviceBinPath()
	if err != nil {
		return service.Params{}, fmt.Errorf("resolve binary path: %w", err)
	}
	abs, err := filepath.Abs(bin)
	if err == nil {
		bin = abs
	}
	cfg := ""
	if opts.ConfigPath != "" {
		cfg = opts.ConfigPath
	} else {
		cfg = defaultConfigPath()
		// Only reference a config that actually exists; otherwise run with
		// built-in defaults.
		if cfg != "" {
			if _, statErr := os.Stat(cfg); statErr != nil {
				cfg = ""
			}
		}
	}
	return service.Params{BinPath: bin, ConfigPath: cfg, LogPath: logPath}, nil
}

func runServiceInstall(opts serviceOptions) error {
	w := writerOr(opts.Writer)
	goos := serviceGOOS()
	home, err := serviceHomeDir()
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux service install: %v", err)}
	}
	file, logPath, ok := servicePaths(goos, home)
	if !ok {
		return unsupportedServiceErr(goos, "install")
	}
	params, err := serviceParams(opts, logPath)
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux service install: %v", err)}
	}

	var content string
	switch goos {
	case "darwin":
		content = service.LaunchdPlist(params)
	case "linux":
		content = service.SystemdUnit(params)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux service install: %v", err)}
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux service install: write %s: %v", file, err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	switch goos {
	case "darwin":
		// Reload if already loaded, then load enabled.
		_, _ = serviceRunner(ctx, "launchctl", "unload", file)
		if out, lerr := serviceRunner(ctx, "launchctl", "load", "-w", file); lerr != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux service install: launchctl load: %v: %s", lerr, out)}
		}
	case "linux":
		if out, lerr := serviceRunner(ctx, "systemctl", "--user", "daemon-reload"); lerr != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux service install: daemon-reload: %v: %s", lerr, out)}
		}
		if out, lerr := serviceRunner(ctx, "systemctl", "--user", "enable", "--now", service.LinuxUnit); lerr != nil {
			return exitError{code: 1, text: fmt.Sprintf("hostmux service install: enable: %v: %s", lerr, out)}
		}
	}
	fmt.Fprintf(w, "installed and started hostmux service: %s\n", file)
	return nil
}

func runServiceUninstall(opts serviceOptions) error {
	w := writerOr(opts.Writer)
	goos := serviceGOOS()
	home, err := serviceHomeDir()
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux service uninstall: %v", err)}
	}
	file, _, ok := servicePaths(goos, home)
	if !ok {
		return unsupportedServiceErr(goos, "uninstall")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	switch goos {
	case "darwin":
		_, _ = serviceRunner(ctx, "launchctl", "unload", file)
	case "linux":
		_, _ = serviceRunner(ctx, "systemctl", "--user", "disable", "--now", service.LinuxUnit)
	}
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return exitError{code: 1, text: fmt.Sprintf("hostmux service uninstall: remove %s: %v", file, err)}
	}
	if goos == "linux" {
		_, _ = serviceRunner(ctx, "systemctl", "--user", "daemon-reload")
	}
	fmt.Fprintf(w, "removed hostmux service: %s\n", file)
	return nil
}

func runServiceStatus(opts serviceOptions) error {
	w := writerOr(opts.Writer)
	goos := serviceGOOS()
	home, err := serviceHomeDir()
	if err != nil {
		return exitError{code: 1, text: fmt.Sprintf("hostmux service status: %v", err)}
	}
	file, _, ok := servicePaths(goos, home)
	if !ok {
		return unsupportedServiceErr(goos, "status")
	}

	installed := false
	if _, statErr := os.Stat(file); statErr == nil {
		installed = true
	}
	fmt.Fprintf(w, "service file: %s\n", file)
	fmt.Fprintf(w, "installed:    %t\n", installed)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	running := false
	switch goos {
	case "darwin":
		if _, lerr := serviceRunner(ctx, "launchctl", "list", service.DarwinLabel); lerr == nil {
			running = true
		}
	case "linux":
		if out, _ := serviceRunner(ctx, "systemctl", "--user", "is-active", service.LinuxUnit); string(trimNL(out)) == "active" {
			running = true
		}
	}
	fmt.Fprintf(w, "running:      %t\n", running)
	return nil
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

func unsupportedServiceErr(goos, verb string) error {
	if goos == "windows" {
		return exitError{code: 1, text: "hostmux service " + verb + ": native Windows service install is not supported. " +
			"Run `hostmux start` from a startup script, or use Task Scheduler / a wrapper like nssm to run `hostmux start --foreground` at logon."}
	}
	return exitError{code: 1, text: fmt.Sprintf("hostmux service %s: unsupported platform %q", verb, goos)}
}
