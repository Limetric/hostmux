package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPSRedirectHandler(t *testing.T) {
	cases := []struct {
		name     string
		host     string
		target   string
		https    int
		hide     bool
		wantLoc  string
		wantCode int
	}{
		{"default port included", "app.example.com", "/path?q=1", 8443, false, "https://app.example.com:8443/path?q=1", http.StatusPermanentRedirect},
		{"host already has port", "app.example.com:8080", "/x", 8443, false, "https://app.example.com:8443/x", http.StatusPermanentRedirect},
		{"port 443 omitted", "app.example.com", "/", 443, false, "https://app.example.com/", http.StatusPermanentRedirect},
		{"hide_port omits port", "app.example.com", "/y", 8443, true, "https://app.example.com/y", http.StatusPermanentRedirect},
		{"ipv6 host", "[::1]:8080", "/z", 8443, false, "https://[::1]:8443/z", http.StatusPermanentRedirect},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHTTPSRedirectHandler(tc.https, tc.hide)
			req := httptest.NewRequest("GET", "http://"+tc.host+tc.target, nil)
			req.Host = tc.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if loc := rec.Header().Get("Location"); loc != tc.wantLoc {
				t.Fatalf("Location = %q, want %q", loc, tc.wantLoc)
			}
		})
	}
}

func TestHTTPSRedirectPreservesMethod(t *testing.T) {
	// 308 preserves POST (a 301/302 could turn it into GET).
	h := newHTTPSRedirectHandler(8443, false)
	req := httptest.NewRequest("POST", "http://api.example.com/submit", nil)
	req.Host = "api.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want 308", rec.Code)
	}
}
