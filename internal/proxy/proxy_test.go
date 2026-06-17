package proxy

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestUpstreamResponseHeaderTimeoutReturns504(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		io.WriteString(w, "late")
	}))
	defer slow.Close()

	r := router.New()
	_ = r.Add("test", []string{"slow.local"}, slow.URL)
	transport := &http.Transport{ResponseHeaderTimeout: 20 * time.Millisecond}
	h := NewWithOptions(r, Options{Transport: transport})

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "slow.local"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", rec.Code)
	}
}

func TestUpstreamTLSVerification(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "secure ok")
	}))
	defer upstream.Close()

	r := router.New()
	_ = r.Add("test", []string{"sec.local"}, upstream.URL) // https://127.0.0.1:PORT

	// Default transport verifies the (untrusted self-signed) cert and fails.
	verifying := New(r)
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "sec.local"
	rec := httptest.NewRecorder()
	verifying.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("verifying transport: status = %d, want 502", rec.Code)
	}

	// With verification disabled, the request succeeds.
	skip := NewWithOptions(r, Options{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}})
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Host = "sec.local"
	rec2 := httptest.NewRecorder()
	skip.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("skip-verify transport: status = %d body=%q, want 200", rec2.Code, rec2.Body.String())
	}
}
