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
	"testing"
	"time"

	"golang.org/x/net/http2"

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
