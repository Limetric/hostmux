//go:build integration && !windows

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Limetric/hostmux/internal/sockproto"
)

func TestEndToEnd(t *testing.T) {
	env, _ := isolatedHostmuxEnv(t)

	// 1. Build the binary into a tmp dir.
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// 2. Use a SHORT tmp dir for the unix socket — macOS limit is ~104 bytes.
	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	// 3. Write a tiny config that lets the daemon pick its TLS port. Using
	// listen="127.0.0.1:0" avoids the TOCTOU race of pre-allocating a port,
	// closing it, and trusting nothing else grabs it before the daemon
	// rebinds — we learn the real bound port from OpInfo below.
	cfgPath := filepath.Join(binDir, "hostmux.toml")
	cfgBody := "[tls]\nlisten = \"127.0.0.1:0\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Spawn the daemon.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemonCmd := exec.CommandContext(ctx, bin, "start", "--foreground", "--config", cfgPath, "--socket", sockPath)
	daemonCmd.Env = env
	daemonCmd.Stdout = testWriter{t}
	daemonCmd.Stderr = testWriter{t}
	if err := daemonCmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = daemonCmd.Process.Kill()
		_ = daemonCmd.Wait()
	})

	// 5. Wait for the unix socket, then ask the daemon what port it bound.
	waitForSocket(t, sockPath, 5*time.Second)
	_, _, publicPort, err := lookupDaemonInfo(sockPath)
	if err != nil {
		t.Fatalf("lookup daemon info: %v", err)
	}
	if publicPort == 0 {
		t.Fatal("daemon did not report a public port")
	}
	proxyAddr := fmt.Sprintf("127.0.0.1:%d", publicPort)
	waitForTCP(t, proxyAddr, 5*time.Second)

	// 7. Start a fake upstream.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "upstream-says-hi")
	}))
	defer upstream.Close()

	// 8. Register e2e.test → upstream.URL via the daemon socket.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	enc := sockproto.NewEncoder(conn)
	dec := sockproto.NewDecoder(conn)
	if err := enc.Encode(&sockproto.Message{
		Op:       sockproto.OpRegister,
		Hosts:    []string{"e2e.test"},
		Upstream: upstream.URL,
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := dec.Decode()
	if err != nil || !resp.Ok {
		t.Fatalf("register: ok=%v err=%v %v", resp != nil && resp.Ok, err, resp)
	}

	// 9. HTTPS HTTP/1.1 round trip.
	{
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				TLSNextProto:    map[string]func(string, *tls.Conn) http.RoundTripper{},
			},
			Timeout: 5 * time.Second,
		}
		req, _ := http.NewRequest("GET", "https://"+proxyAddr+"/", nil)
		req.Host = "e2e.test"
		r, err := client.Do(req)
		if err != nil {
			t.Fatalf("http/1.1: %v", err)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if string(body) != "upstream-says-hi" {
			t.Fatalf("http/1.1 body = %q", body)
		}
		if r.ProtoMajor != 1 {
			t.Fatalf("http/1.1 proto = %d", r.ProtoMajor)
		}
	}

	// 10. HTTPS HTTP/2 round trip.
	{
		client := &http.Client{
			Transport: &http.Transport{
				ForceAttemptHTTP2: true,
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 5 * time.Second,
		}
		req, _ := http.NewRequest("GET", "https://"+proxyAddr+"/", nil)
		req.Host = "e2e.test"
		r, err := client.Do(req)
		if err != nil {
			t.Fatalf("https http/2: %v", err)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if string(body) != "upstream-says-hi" {
			t.Fatalf("https http/2 body = %q", body)
		}
		if r.ProtoMajor != 2 {
			t.Fatalf("https http/2 proto = %d", r.ProtoMajor)
		}
	}

	// 11. Close the registrar — entry should disappear within ~2s.
	conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	notFoundClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			TLSNextProto:    map[string]func(string, *tls.Conn) http.RoundTripper{},
		},
		Timeout: 5 * time.Second,
	}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", "https://"+proxyAddr+"/", nil)
		req.Host = "e2e.test"
		r, err := notFoundClient.Do(req)
		if err == nil {
			if r.StatusCode == http.StatusNotFound {
				r.Body.Close()
				return // success
			}
			r.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("entry was not cleaned up after registrar disconnect")
}

func TestRunInheritsDomainFromDaemonConfig(t *testing.T) {
	env, _ := isolatedHostmuxEnv(t)

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	cfgPath := filepath.Join(binDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte("listen = \"127.0.0.1:0\"\ndomain = \"example.com\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemonCmd := exec.CommandContext(ctx, bin, "start", "--foreground", "--config", cfgPath, "--socket", sockPath)
	daemonCmd.Env = env
	daemonCmd.Stdout = testWriter{t}
	daemonCmd.Stderr = testWriter{t}
	if err := daemonCmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = daemonCmd.Process.Kill()
		_ = daemonCmd.Wait()
	})
	waitForSocket(t, sockPath, 5*time.Second)

	run := exec.Command(bin, "run", "--socket", sockPath, "--name", "api", "--", "/bin/sh", "-c", "sleep 2")
	run.Env = env
	run.Stdout = testWriter{t}
	run.Stderr = testWriter{t}
	if err := run.Start(); err != nil {
		t.Fatalf("start run: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		routes := exec.Command(bin, "routes", "--socket", sockPath)
		routes.Env = env
		out, err := routes.CombinedOutput()
		if err == nil && strings.Contains(string(out), "api.example.com") {
			if err := run.Wait(); err != nil {
				t.Fatalf("run wait: %v", err)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	_ = run.Process.Kill()
	_ = run.Wait()
	t.Fatal("registered route did not include daemon domain")
}

func TestURLInheritsDomainFromDaemonConfig(t *testing.T) {
	env, _ := isolatedHostmuxEnv(t)

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	// Let the daemon pick the port: listen="127.0.0.1:0" exercises the
	// runForegroundDaemon path that captures the OS-assigned port post-bind
	// and is also race-free (no TOCTOU window between Listen/Close and the
	// daemon rebinding the same port).
	cfgPath := filepath.Join(binDir, "hostmux.toml")
	cfgBody := "listen = \"127.0.0.1:0\"\ndomain = \"example.com\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemonCmd := exec.CommandContext(ctx, bin, "start", "--foreground", "--config", cfgPath, "--socket", sockPath)
	daemonCmd.Env = env
	daemonCmd.Stdout = testWriter{t}
	daemonCmd.Stderr = testWriter{t}
	if err := daemonCmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = daemonCmd.Process.Kill()
		_ = daemonCmd.Wait()
	})
	waitForSocket(t, sockPath, 5*time.Second)

	urlCmd := exec.Command(bin, "url", "--socket", sockPath, "--no-prefix", "api")
	urlCmd.Env = env
	out, err := urlCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("url: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse url %q: %v", got, err)
	}
	if u.Scheme != "https" {
		t.Fatalf("scheme = %q, want https (full url: %q)", u.Scheme, got)
	}
	if u.Hostname() != "api.example.com" {
		t.Fatalf("hostname = %q, want api.example.com (full url: %q)", u.Hostname(), got)
	}
	if u.Port() == "" || u.Port() == "443" {
		t.Fatalf("port = %q, want non-default (full url: %q)", u.Port(), got)
	}
}

func TestURLOmitsPortWhenHidePortSet(t *testing.T) {
	env, _ := isolatedHostmuxEnv(t)

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	// Let the daemon pick the port: with listen="127.0.0.1:0" the real
	// listener gets a non-default ephemeral port, so a daemon WITHOUT
	// hide_port would advertise `https://api.example.com:<ephemeral>`.
	// With hide_port=true the printed URL must be portless regardless of
	// which port the OS hands out.
	cfgPath := filepath.Join(binDir, "hostmux.toml")
	cfgBody := "listen = \"127.0.0.1:0\"\ndomain = \"example.com\"\nhide_port = true\n"
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemonCmd := exec.CommandContext(ctx, bin, "start", "--foreground", "--config", cfgPath, "--socket", sockPath)
	daemonCmd.Env = env
	daemonCmd.Stdout = testWriter{t}
	daemonCmd.Stderr = testWriter{t}
	if err := daemonCmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = daemonCmd.Process.Kill()
		_ = daemonCmd.Wait()
	})
	waitForSocket(t, sockPath, 5*time.Second)

	urlCmd := exec.Command(bin, "url", "--socket", sockPath, "--no-prefix", "api")
	urlCmd.Env = env
	out, err := urlCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("url: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse url %q: %v", got, err)
	}
	if u.Scheme != "https" {
		t.Fatalf("scheme = %q, want https (full url: %q)", u.Scheme, got)
	}
	if u.Hostname() != "api.example.com" {
		t.Fatalf("hostname = %q, want api.example.com (full url: %q)", u.Hostname(), got)
	}
	if u.Port() != "" {
		t.Fatalf("port = %q, want empty (full url: %q)", u.Port(), got)
	}
}

func TestRunAutoStartUsesConfigSocket(t *testing.T) {
	env, home := isolatedHostmuxEnv(t)

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	cfgDir := filepath.Join(home, ".config", "hostmux")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgBody := fmt.Sprintf("listen = \"127.0.0.1:0\"\nsocket = %q\ndomain = \"example.com\"\n", sockPath)
	if err := os.WriteFile(filepath.Join(cfgDir, "hostmux.toml"), []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	run := exec.Command(bin, "run", "--name", "api", "--", "/bin/sh", "-c", "sleep 2")
	run.Env = env
	run.Stdout = testWriter{t}
	run.Stderr = testWriter{t}
	if err := run.Start(); err != nil {
		t.Fatalf("start run: %v", err)
	}
	t.Cleanup(func() {
		stop := exec.Command(bin, "stop", "--socket", sockPath)
		stop.Env = env
		_ = stop.Run()
	})

	waitForSocket(t, sockPath, 5*time.Second)

	defaultSock := filepath.Join(home, ".hostmux", "hostmux.sock")
	if _, err := os.Stat(defaultSock); err == nil {
		_ = run.Process.Kill()
		_ = run.Wait()
		t.Fatalf("default socket %q exists; expected custom socket only", defaultSock)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		routes := exec.Command(bin, "routes", "--socket", sockPath)
		routes.Env = env
		routesOut, err := routes.CombinedOutput()
		if err == nil && strings.Contains(string(routesOut), "api.example.com") {
			if err := run.Wait(); err != nil {
				t.Fatalf("run wait: %v", err)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	_ = run.Process.Kill()
	_ = run.Wait()
	t.Fatal("registered route did not appear on custom socket")
}

func waitForSocket(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.Dial("unix", path)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket %s never came up", path)
}

func waitForTCP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tcp %s never came up", addr)
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("daemon: %s", p)
	return len(p), nil
}

func TestStop(t *testing.T) {
	bin, sockPath, env, _, cleanup := startDaemonForStopTest(t)
	defer cleanup()

	// Run `hostmux stop` and assert it exits 0 and logs a "stopped" message.
	stop := exec.Command(bin, "stop", "--socket", sockPath)
	stop.Env = env
	out, err := stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if !containsSubstring(string(out), "stopped daemon") {
		t.Fatalf("expected 'stopped daemon' line, got: %s", out)
	}

	// Socket file should be gone shortly.
	waitForSocketGone(t, sockPath, 3*time.Second)

	// PID file flock should be releasable.
	pidPath := sockPath[:len(sockPath)-len(".sock")] + ".pid"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if canAcquireFlock(pidPath) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("pid file flock still held after stop")
}

func TestStopIdempotent(t *testing.T) {
	env, _ := isolatedHostmuxEnv(t)

	// Build the binary but do NOT start a daemon.
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	stop := exec.Command(bin, "stop", "--socket", sockPath)
	stop.Env = env
	out, err := stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if !containsSubstring(string(out), "no daemon running") {
		t.Fatalf("expected 'no daemon running', got: %s", out)
	}
}

func TestStartDetached(t *testing.T) {
	env, _ := isolatedHostmuxEnv(t)

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	cfgPath := filepath.Join(binDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte("listen = \"127.0.0.1:0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	start := exec.Command(bin, "start", "--config", cfgPath, "--socket", sockPath)
	start.Env = env
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start: %v\n%s", err, out)
	}

	waitForSocket(t, sockPath, 5*time.Second)

	stop := exec.Command(bin, "stop", "--socket", sockPath)
	stop.Env = env
	out, err := stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if !containsSubstring(string(out), "stopped daemon") {
		t.Fatalf("expected 'stopped daemon' line, got: %s", out)
	}
	waitForSocketGone(t, sockPath, 3*time.Second)
}

func TestStartForeground(t *testing.T) {
	env, _ := isolatedHostmuxEnv(t)

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	cfgPath := filepath.Join(binDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte("listen = \"127.0.0.1:0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "start", "--foreground", "--config", cfgPath, "--socket", sockPath)
	cmd.Env = env
	cmd.Stdout = testWriter{t}
	cmd.Stderr = testWriter{t}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start --foreground: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	waitForSocket(t, sockPath, 5*time.Second)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal start --foreground: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait start --foreground: %v", err)
	}
	waitForSocketGone(t, sockPath, 3*time.Second)
}

func TestStartForceReportsStartupFailure(t *testing.T) {
	bin, sockPath, env, _, cleanup := startDaemonForStopTest(t)
	defer cleanup()

	home := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, "HOME=") {
			home = strings.TrimPrefix(entry, "HOME=")
			break
		}
	}
	if home == "" {
		t.Fatal("HOME not found in test env")
	}
	tlsDir := filepath.Join(home, ".hostmux", "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "hostmux.crt"), []byte("bad cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "hostmux.key"), []byte("bad key"), 0o600); err != nil {
		t.Fatal(err)
	}

	start := exec.Command(bin, "start", "--force", "--socket", sockPath)
	start.Env = env
	out, err := start.CombinedOutput()
	if err == nil {
		t.Fatalf("start --force unexpectedly succeeded\n%s", out)
	}
	if !containsSubstring(string(out), "could not start daemon") {
		t.Fatalf("expected startup failure message, got: %s", out)
	}
}

func TestStartForceDetachedSuccess(t *testing.T) {
	bin, sockPath, env, firstWait, cleanup := startDaemonForStopTest(t)
	defer cleanup()

	logDir := t.TempDir()
	cfgPath := filepath.Join(logDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte("listen = \"127.0.0.1:0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	start := exec.Command(bin, "start", "--config", cfgPath, "--socket", sockPath, "--force")
	start.Env = env
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start --force: %v\n%s", err, out)
	}

	firstExited := make(chan struct{})
	go func() {
		firstWait()
		close(firstExited)
	}()
	select {
	case <-firstExited:
	case <-time.After(8 * time.Second):
		t.Fatal("first daemon did not exit after start --force")
	}

	routes := exec.Command(bin, "routes", "--socket", sockPath)
	routes.Env = env
	if out, err := routes.CombinedOutput(); err != nil {
		t.Fatalf("routes: %v\n%s", err, out)
	}
}

func TestServeForce(t *testing.T) {
	bin, sockPath, env, firstWait, cleanup := startDaemonForStopTest(t)
	defer cleanup()

	// Start a second foreground daemon with --force pointing at the same socket. Use
	// the same ephemeral-port config so we don't contend on :8080.
	logDir := t.TempDir()
	cfgPath := filepath.Join(logDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte("listen = \"127.0.0.1:0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	second := exec.CommandContext(ctx, bin, "start", "--foreground", "--config", cfgPath, "--socket", sockPath, "--force")
	second.Env = env
	second.Stdout = testWriter{t}
	second.Stderr = testWriter{t}
	if err := second.Start(); err != nil {
		t.Fatalf("start second: %v", err)
	}
	t.Cleanup(func() {
		_ = second.Process.Kill()
		_ = second.Wait()
	})

	// First daemon should exit on its own after --force takeover.
	// firstWait is serialized through sync.Once, so calling it from
	// this goroutine (and later from cleanup) is safe.
	firstExited := make(chan struct{})
	go func() {
		firstWait()
		close(firstExited)
	}()
	select {
	case <-firstExited:
	case <-time.After(8 * time.Second):
		t.Fatal("first daemon did not exit after start --foreground --force")
	}

	// Second daemon's socket should come up.
	waitForSocket(t, sockPath, 5*time.Second)

	// hostmux list against the second daemon should succeed.
	routes := exec.Command(bin, "routes", "--socket", sockPath)
	routes.Env = env
	if out, err := routes.CombinedOutput(); err != nil {
		t.Fatalf("routes: %v\n%s", err, out)
	}
}

func TestServeWithoutForceContention(t *testing.T) {
	bin, sockPath, env, _, cleanup := startDaemonForStopTest(t)
	defer cleanup()

	// Start a second foreground daemon WITHOUT --force. It should exit 0 immediately.
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	second := exec.CommandContext(ctx, bin, "start", "--foreground", "--socket", sockPath)
	second.Env = env
	second.Stdout = testWriter{t}
	second.Stderr = testWriter{t}
	if err := second.Run(); err != nil {
		t.Fatalf("second start --foreground exited non-zero: %v", err)
	}
	// If we got here, it exited 0 as expected.
}

func TestStopFallsBackFromStaleDiscovery(t *testing.T) {
	env, home := isolatedHostmuxEnv(t)

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cfgPath := filepath.Join(binDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte("listen = \"127.0.0.1:0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemonCmd := exec.CommandContext(ctx, bin, "start", "--foreground", "--config", cfgPath)
	daemonCmd.Env = env
	daemonCmd.Stdout = testWriter{t}
	daemonCmd.Stderr = testWriter{t}
	if err := daemonCmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = daemonCmd.Process.Kill()
		_ = daemonCmd.Wait()
	})

	defaultSock := filepath.Join(home, ".hostmux", "hostmux.sock")
	waitForSocket(t, defaultSock, 5*time.Second)

	discoveryPath := filepath.Join(home, ".hostmux", "socket")
	if err := os.WriteFile(discoveryPath, []byte(filepath.Join(t.TempDir(), "stale.sock")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stop := exec.Command(bin, "stop")
	stop.Env = env
	out, err := stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if !containsSubstring(string(out), "stopped daemon") {
		t.Fatalf("expected 'stopped daemon' line, got: %s", out)
	}
	waitForSocketGone(t, defaultSock, 3*time.Second)
}

// startDaemonForStopTest builds the binary, starts a daemon on a temp socket,
// waits for the socket to come up, and returns (bin, sockPath, waitOnce, cleanup).
// Used by stop/force tests that do NOT need the full TLS/config setup of TestEndToEnd.
//
// waitOnce blocks until the daemon exits and returns its error. It is gated
// through sync.Once so multiple callers (test code + cleanup) can safely ask
// for the exit status without racing on cmd.Wait — Go's stdlib documents
// concurrent Wait calls as unsafe.
//
// cleanup cancels the daemon's context and drains waitOnce (with a timeout).
//
// A tiny config file is written pointing the HTTP listener at 127.0.0.1:0 so
// these tests do not contend with anything already on :8080 and so parallel
// runs of the daemon/force tests do not collide on a fixed port.
func startDaemonForStopTest(t *testing.T) (bin, sockPath string, env []string, waitOnce func() error, cleanup func()) {
	t.Helper()
	env, _ = isolatedHostmuxEnv(t)
	binDir := t.TempDir()
	bin = filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath = filepath.Join(sockDir, "t.sock")

	cfgPath := filepath.Join(binDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte("listen = \"127.0.0.1:0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "start", "--foreground", "--config", cfgPath, "--socket", sockPath)
	cmd.Env = env
	cmd.Stdout = testWriter{t}
	cmd.Stderr = testWriter{t}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start daemon: %v", err)
	}
	waitForSocket(t, sockPath, 5*time.Second)

	// Gate cmd.Wait through a sync.Once so both tests and cleanup can
	// request the exit result safely without double-Wait.
	var once sync.Once
	var waitErr error
	waitOnce = func() error {
		once.Do(func() { waitErr = cmd.Wait() })
		return waitErr
	}

	cleanup = func() {
		cancel()
		done := make(chan struct{})
		go func() {
			waitOnce()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Logf("daemon wait timed out during cleanup")
		}
	}
	return
}

func isolatedHostmuxEnv(t *testing.T) ([]string, string) {
	t.Helper()
	home, err := os.MkdirTemp("", "hmhome")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	env := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		switch {
		case strings.HasPrefix(entry, "HOME="):
		case strings.HasPrefix(entry, "XDG_RUNTIME_DIR="):
		case strings.HasPrefix(entry, "XDG_CONFIG_HOME="):
		case strings.HasPrefix(entry, "HOSTMUX_SOCKET="):
		default:
			env = append(env, entry)
		}
	}
	env = append(env,
		"HOME="+home,
		"XDG_RUNTIME_DIR=",
		"XDG_CONFIG_HOME=",
		"HOSTMUX_SOCKET=",
	)
	return env, home
}

func containsSubstring(output, needle string) bool {
	return strings.Contains(output, needle)
}

func waitForSocketGone(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket %s still present after %v", path, timeout)
}

func canAcquireFlock(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return false
	}
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return true
}
