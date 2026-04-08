//go:build integration

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/sys/unix"

	"github.com/Limetric/hostmux/internal/sockproto"
)

func TestEndToEnd(t *testing.T) {
	// 1. Build the binary into a tmp dir.
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "hostmux")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// 2. Allocate a free TCP port for the proxy listen.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxyAddr := l.Addr().String()
	l.Close()

	// 3. Use a SHORT tmp dir for the unix socket — macOS limit is ~104 bytes.
	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "t.sock")

	// 4. Write a tiny config that points the listener at proxyAddr.
	cfgPath := filepath.Join(binDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("listen = %q\n", proxyAddr)), 0o644); err != nil {
		t.Fatal(err)
	}

	// 5. Spawn the daemon.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemonCmd := exec.CommandContext(ctx, bin, "serve", "--config", cfgPath, "--socket", sockPath)
	daemonCmd.Stdout = testWriter{t}
	daemonCmd.Stderr = testWriter{t}
	if err := daemonCmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = daemonCmd.Process.Kill()
		_ = daemonCmd.Wait()
	})

	// 6. Wait for the unix socket file AND for the proxy port to be reachable.
	waitForSocket(t, sockPath, 5*time.Second)
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

	// 9. HTTP/1.1 round trip.
	{
		req, _ := http.NewRequest("GET", "http://"+proxyAddr+"/", nil)
		req.Host = "e2e.test"
		r, err := http.DefaultClient.Do(req)
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

	// 10. h2c round trip.
	{
		client := &http.Client{
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, network, addr)
				},
			},
			Timeout: 5 * time.Second,
		}
		req, _ := http.NewRequest("GET", "http://"+proxyAddr+"/", nil)
		req.Host = "e2e.test"
		r, err := client.Do(req)
		if err != nil {
			t.Fatalf("h2c: %v", err)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if string(body) != "upstream-says-hi" {
			t.Fatalf("h2c body = %q", body)
		}
		if r.ProtoMajor != 2 {
			t.Fatalf("h2c proto = %d", r.ProtoMajor)
		}
	}

	// 11. Close the registrar — entry should disappear within ~2s.
	conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", "http://"+proxyAddr+"/", nil)
		req.Host = "e2e.test"
		r, err := http.DefaultClient.Do(req)
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
	bin, sockPath, _, cleanup := startDaemonForStopTest(t)
	defer cleanup()

	// Run `hostmux stop` and assert it exits 0 and logs a "stopped" message.
	stop := exec.Command(bin, "stop", "--socket", sockPath)
	out, err := stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if !containsLine(string(out), "stopped daemon") {
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
	out, err := stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if !containsLine(string(out), "no daemon running") {
		t.Fatalf("expected 'no daemon running', got: %s", out)
	}
}

func TestServeForce(t *testing.T) {
	bin, sockPath, firstCmd, cleanup := startDaemonForStopTest(t)
	defer cleanup()

	// Start a second serve with --force pointing at the same socket. Use
	// the same ephemeral-port config so we don't contend on :8080.
	logDir := t.TempDir()
	cfgPath := filepath.Join(logDir, "hostmux.toml")
	if err := os.WriteFile(cfgPath, []byte("listen = \"127.0.0.1:0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	second := exec.CommandContext(ctx, bin, "serve", "--config", cfgPath, "--socket", sockPath, "--force")
	second.Stdout = testWriter{t}
	second.Stderr = testWriter{t}
	if err := second.Start(); err != nil {
		t.Fatalf("start second: %v", err)
	}
	t.Cleanup(func() {
		_ = second.Process.Kill()
		_ = second.Wait()
	})

	// First daemon should exit on its own.
	exited := make(chan error, 1)
	go func() { exited <- firstCmd.Wait() }()
	select {
	case <-exited:
	case <-time.After(8 * time.Second):
		t.Fatal("first daemon did not exit after serve --force")
	}

	// Second daemon's socket should come up.
	waitForSocket(t, sockPath, 5*time.Second)

	// hostmux list against the second daemon should succeed.
	list := exec.Command(bin, "list", "--socket", sockPath)
	if out, err := list.CombinedOutput(); err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
}

func TestServeWithoutForceContention(t *testing.T) {
	bin, sockPath, _, cleanup := startDaemonForStopTest(t)
	defer cleanup()

	// Start a second serve WITHOUT --force. It should exit 0 immediately.
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	second := exec.CommandContext(ctx, bin, "serve", "--socket", sockPath)
	second.Stdout = testWriter{t}
	second.Stderr = testWriter{t}
	if err := second.Run(); err != nil {
		t.Fatalf("second serve exited non-zero: %v", err)
	}
	// If we got here, it exited 0 as expected.
}

// startDaemonForStopTest builds the binary, starts a daemon on a temp socket,
// waits for the socket to come up, and returns (bin, sockPath, daemonCmd, cleanup).
// The cleanup func cancels the daemon's context and waits. Used by stop/force tests
// that do NOT need the full TLS/config setup of TestEndToEnd.
//
// A tiny config file is written pointing the HTTP listener at 127.0.0.1:0 so
// these tests do not contend with anything already on :8080 and so parallel
// runs of the serve/force tests do not collide on a fixed port.
func startDaemonForStopTest(t *testing.T) (bin, sockPath string, cmd *exec.Cmd, cleanup func()) {
	t.Helper()
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
	cmd = exec.CommandContext(ctx, bin, "serve", "--config", cfgPath, "--socket", sockPath)
	cmd.Stdout = testWriter{t}
	cmd.Stderr = testWriter{t}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start daemon: %v", err)
	}
	waitForSocket(t, sockPath, 5*time.Second)
	cleanup = func() {
		cancel()
		_ = cmd.Wait()
	}
	return
}

func containsLine(output, needle string) bool {
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
