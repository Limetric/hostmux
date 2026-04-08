package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/router"
)

func newProxyWith(t *testing.T, host, upstream string) http.Handler {
	t.Helper()
	r := router.New()
	if err := r.Add("test", []string{host}, upstream); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return New(r)
}

func TestProxiesToUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello from upstream")
	}))
	defer upstream.Close()

	h := newProxyWith(t, "myapp.local", upstream.URL)
	req := httptest.NewRequest("GET", "/anything", nil)
	req.Host = "myapp.local"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "hello from upstream") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestPreservesHostHeader(t *testing.T) {
	var seenHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHost = r.Host
	}))
	defer upstream.Close()

	h := newProxyWith(t, "myapp.local", upstream.URL)
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "myapp.local"
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seenHost != "myapp.local" {
		t.Fatalf("upstream saw Host=%q, want myapp.local", seenHost)
	}
}

func TestSetsForwardedHeaders(t *testing.T) {
	var seen http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	}))
	defer upstream.Close()

	h := newProxyWith(t, "myapp.local", upstream.URL)
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "myapp.local"
	req.RemoteAddr = "203.0.113.1:1234"
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen.Get("X-Forwarded-Host") != "myapp.local" {
		t.Fatalf("X-Forwarded-Host = %q", seen.Get("X-Forwarded-Host"))
	}
	if seen.Get("X-Forwarded-Proto") == "" {
		t.Fatal("X-Forwarded-Proto unset")
	}
	if !strings.Contains(seen.Get("X-Forwarded-For"), "203.0.113.1") {
		t.Fatalf("X-Forwarded-For = %q", seen.Get("X-Forwarded-For"))
	}
}

func TestUnknownHostReturns404(t *testing.T) {
	r := router.New()
	h := New(r)
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "ghost.local"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestStripsPortFromHostHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()
	h := newProxyWith(t, "myapp.local", upstream.URL)
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "myapp.local:8080"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpstreamUnreachableReturns502(t *testing.T) {
	r := router.New()
	_ = r.Add("test", []string{"dead.local"}, "http://127.0.0.1:1") // port 1 = blackhole
	h := New(r)
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "dead.local"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rec.Code)
	}
}
