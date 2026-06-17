package sockserver

import (
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Limetric/hostmux/internal/router"
	"github.com/Limetric/hostmux/internal/sockproto"
)

// shortTempDir returns a short temporary directory path to avoid hitting
// the 104-char Unix socket path limit on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func startServer(t *testing.T) (path string, r *router.Router, srv *Server) {
	t.Helper()
	path = filepath.Join(shortTempDir(t), "t.sock")
	r = router.New()
	srv = New(r, Options{})
	if err := srv.Listen(path); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return
}

func dial(t *testing.T, path string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		c, err := net.Dial("unix", path)
		if err == nil {
			return c
		}
		if time.Now().After(deadline) {
			t.Fatalf("dial: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRegisterAddsToRouter(t *testing.T) {
	path, r, _ := startServer(t)
	c := dial(t, path)
	defer c.Close()

	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpRegister, Hosts: []string{"a.test"}, Upstream: "http://127.0.0.1:9000"}); err != nil {
		t.Fatal(err)
	}
	resp, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ok {
		t.Fatalf("not ok: %s", resp.Error)
	}
	if up, ok := r.Lookup("a.test"); !ok || up != "http://127.0.0.1:9000" {
		t.Fatalf("router missing entry: %q %v", up, ok)
	}
}

func TestRegisterCollisionReturnsError(t *testing.T) {
	path, r, _ := startServer(t)
	_ = r.Add("config", []string{"taken.test"}, "http://127.0.0.1:1")

	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	enc.Encode(&sockproto.Message{Op: sockproto.OpRegister, Hosts: []string{"taken.test"}, Upstream: "http://127.0.0.1:9999"})
	resp, _ := dec.Decode()
	if resp.Ok {
		t.Fatal("expected error response")
	}
	if resp.Error == "" {
		t.Fatal("error message is empty")
	}
}

func TestDisconnectCleansUpEphemeralEntries(t *testing.T) {
	path, r, _ := startServer(t)
	c := dial(t, path)
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	enc.Encode(&sockproto.Message{Op: sockproto.OpRegister, Hosts: []string{"ephemeral.test"}, Upstream: "http://127.0.0.1:9000"})
	dec.Decode()
	if _, ok := r.Lookup("ephemeral.test"); !ok {
		t.Fatal("not registered")
	}
	c.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := r.Lookup("ephemeral.test"); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("ephemeral entry not cleaned up after disconnect")
}

func TestListReturnsAllEntries(t *testing.T) {
	path, r, _ := startServer(t)
	_ = r.Add("config", []string{"api.local"}, "http://127.0.0.1:8080")

	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	enc.Encode(&sockproto.Message{Op: sockproto.OpList})
	resp, _ := dec.Decode()
	if !resp.Ok {
		t.Fatalf("not ok: %s", resp.Error)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].Hosts[0] != "api.local" {
		t.Fatalf("entries = %+v", resp.Entries)
	}
}

func TestServerCancellableViaContext(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "ctx.sock")
	r := router.New()
	srv := New(r, Options{})
	if err := srv.Listen(path); err != nil {
		t.Fatal(err)
	}
	var done atomic.Bool
	go func() {
		srv.Serve()
		done.Store(true)
	}()
	srv.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not exit after Close")
}

func TestShutdownOpInvokesCallback(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "sd.sock")
	r := router.New()
	var fired atomic.Bool
	srv := New(r, Options{OnShutdown: func() { fired.Store(true) }})
	if err := srv.Listen(path); err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpShutdown}); err != nil {
		t.Fatal(err)
	}
	resp, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("not ok: %s", resp.Error)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if fired.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("OnShutdown callback was not invoked within 1s")
}

func TestInfoReturnsDomain(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "info.sock")
	r := router.New()
	srv := New(r, Options{Domain: func() string { return "example.com" }})
	if err := srv.Listen(path); err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpInfo}); err != nil {
		t.Fatal(err)
	}
	resp, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("not ok: %s", resp.Error)
	}
	if resp.Domain != "example.com" {
		t.Fatalf("domain = %q", resp.Domain)
	}
	if resp.PublicHTTPS == nil || !*resp.PublicHTTPS {
		t.Fatalf("public_https = %v, want true", resp.PublicHTTPS)
	}
}

func TestInfoReturnsEmptyDomainWhenCallbackIsNil(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "info-empty.sock")
	r := router.New()
	srv := New(r, Options{})
	if err := srv.Listen(path); err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpInfo}); err != nil {
		t.Fatal(err)
	}
	resp, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("not ok: %s", resp.Error)
	}
	if resp.Domain != "" {
		t.Fatalf("domain = %q, want empty", resp.Domain)
	}
	if resp.PublicHTTPS == nil || !*resp.PublicHTTPS {
		t.Fatalf("public_https = %v, want true", resp.PublicHTTPS)
	}
}

func TestInfoReportsPlainHTTPWhenConfigured(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "info-plain.sock")
	r := router.New()
	srv := New(r, Options{PlainHTTP: true})
	if err := srv.Listen(path); err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpInfo}); err != nil {
		t.Fatal(err)
	}
	resp, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("not ok: %s", resp.Error)
	}
	if resp.PublicHTTPS == nil || *resp.PublicHTTPS {
		t.Fatalf("public_https = %v, want false", resp.PublicHTTPS)
	}
}

func TestInfoReturnsPublicPort(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "info-port.sock")
	r := router.New()
	srv := New(r, Options{PublicPort: 8443})
	if err := srv.Listen(path); err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpInfo}); err != nil {
		t.Fatal(err)
	}
	resp, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("not ok: %s", resp.Error)
	}
	if resp.PublicPort != 8443 {
		t.Fatalf("public_port = %d, want 8443", resp.PublicPort)
	}
}

func TestInfoOmitsPublicPortWhenUnset(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "info-noport.sock")
	r := router.New()
	srv := New(r, Options{})
	if err := srv.Listen(path); err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpInfo}); err != nil {
		t.Fatal(err)
	}
	resp, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("not ok: %s", resp.Error)
	}
	if resp.PublicPort != 0 {
		t.Fatalf("public_port = %d, want 0 (unset)", resp.PublicPort)
	}
}

func TestRegisterRoundTripsMetadata(t *testing.T) {
	path, _, _ := startServer(t)
	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{
		Op:       sockproto.OpRegister,
		Hosts:    []string{"meta.test"},
		Upstream: "http://127.0.0.1:9000",
		Labels:   map[string]string{"team": "web"},
		PID:      4242,
		Command:  "bun run dev",
		Cwd:      "/work/app",
	}); err != nil {
		t.Fatal(err)
	}
	if resp, err := dec.Decode(); err != nil || !resp.Ok {
		t.Fatalf("register: resp=%+v err=%v", resp, err)
	}

	enc.Encode(&sockproto.Message{Op: sockproto.OpList})
	resp, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("entries = %+v", resp.Entries)
	}
	e := resp.Entries[0]
	if e.Labels["team"] != "web" || e.PID != 4242 || e.Command != "bun run dev" || e.Cwd != "/work/app" {
		t.Fatalf("metadata not round-tripped: %+v", e)
	}
	if e.RegisteredAt == 0 {
		t.Fatalf("registered_at not stamped: %+v", e)
	}
}

func TestExposePersistsAcrossDisconnect(t *testing.T) {
	path, r, _ := startServer(t)
	c := dial(t, path)
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpExpose, Source: "api", Hosts: []string{"api.test"}, Upstream: "http://127.0.0.1:3000"}); err != nil {
		t.Fatal(err)
	}
	if resp, err := dec.Decode(); err != nil || !resp.Ok {
		t.Fatalf("expose: resp=%+v err=%v", resp, err)
	}
	c.Close()
	// The route must survive disconnect (unlike OpRegister routes).
	time.Sleep(100 * time.Millisecond)
	if up, ok := r.Lookup("api.test"); !ok || up != "http://127.0.0.1:3000" {
		t.Fatalf("exposed route should persist after disconnect: up=%q ok=%v", up, ok)
	}

	// Unexpose removes it.
	c2 := dial(t, path)
	defer c2.Close()
	enc2 := sockproto.NewEncoder(c2)
	dec2 := sockproto.NewDecoder(c2)
	enc2.Encode(&sockproto.Message{Op: sockproto.OpUnexpose, Source: "api"})
	if resp, err := dec2.Decode(); err != nil || !resp.Ok {
		t.Fatalf("unexpose: resp=%+v err=%v", resp, err)
	}
	if _, ok := r.Lookup("api.test"); ok {
		t.Fatal("route should be gone after unexpose")
	}
}

func TestUnexposeUnknownReturnsError(t *testing.T) {
	path, _, _ := startServer(t)
	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	enc.Encode(&sockproto.Message{Op: sockproto.OpUnexpose, Source: "nope"})
	resp, _ := dec.Decode()
	if resp.Ok {
		t.Fatal("expected error for unknown route")
	}
}

func TestExposeRejectsSourceSpoofing(t *testing.T) {
	path, r, _ := startServer(t)
	// A real config route the manual route must not be able to masquerade as.
	_ = r.Add("config", []string{"protected.test"}, "http://127.0.0.1:1")
	c := dial(t, path)
	defer c.Close()
	enc := sockproto.NewEncoder(c)
	dec := sockproto.NewDecoder(c)
	// "config" contains no colon but exposing must namespace it; a colon
	// in the name is rejected outright.
	enc.Encode(&sockproto.Message{Op: sockproto.OpExpose, Source: "socket:1", Hosts: []string{"x.test"}, Upstream: "http://127.0.0.1:2"})
	resp, _ := dec.Decode()
	if resp.Ok {
		t.Fatal("colon in name must be rejected")
	}
	// A benign name "config" is still namespaced to manual:config, so the
	// real config route is untouched.
	enc.Encode(&sockproto.Message{Op: sockproto.OpExpose, Source: "config", Hosts: []string{"y.test"}, Upstream: "http://127.0.0.1:3"})
	resp, _ = dec.Decode()
	if !resp.Ok {
		t.Fatalf("benign name should be accepted: %s", resp.Error)
	}
	enc.Encode(&sockproto.Message{Op: sockproto.OpList})
	resp, _ = dec.Decode()
	for _, e := range resp.Entries {
		if e.Source == "config" && len(e.Hosts) == 1 && e.Hosts[0] == "y.test" {
			t.Fatal("manual route masqueraded as config source")
		}
	}
}
