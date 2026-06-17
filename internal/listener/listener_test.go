package listener

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "proto="+r.Proto)
	})
}

func TestBuildPlainHTTP1Server(t *testing.T) {
	servers, err := Build(Config{Plain: ":0"}, okHandler())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if servers.Plain == nil {
		t.Fatal("Plain = nil, want non-nil")
	}
	if servers.TLS != nil {
		t.Fatal("TLS = non-nil, want nil")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go servers.Plain.Serve(ln)
	defer servers.Plain.Shutdown(context.Background())

	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), "proto=HTTP/1.1") {
		t.Fatalf("body = %q", body)
	}
}

func TestPlainListenerSpeaksH2C(t *testing.T) {
	servers, err := Build(Config{Plain: ":0"}, okHandler())
	if err != nil {
		t.Fatal(err)
	}
	if servers.Plain == nil {
		t.Fatal("Plain = nil, want non-nil")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go servers.Plain.Serve(ln)
	defer servers.Plain.Shutdown(context.Background())

	// HTTP/2 client over plain TCP (prior-knowledge h2c).
	client := &http.Client{
		Transport: &http.Transport{
			Protocols: func() *http.Protocols {
				p := new(http.Protocols)
				p.SetUnencryptedHTTP2(true)
				return p
			}(),
		},
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatalf("h2c Get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), "proto=HTTP/2") {
		t.Fatalf("body = %q", body)
	}
}

func TestBuildWithoutTLSReturnsPlainOnly(t *testing.T) {
	servers, err := Build(Config{Plain: ":8080"}, okHandler())
	if err != nil {
		t.Fatal(err)
	}
	if servers.Plain == nil {
		t.Fatal("Plain = nil, want non-nil")
	}
	if servers.TLS != nil {
		t.Fatal("TLS = non-nil, want nil")
	}
}

func TestBuildWithBothReturnsBoth(t *testing.T) {
	servers, err := Build(Config{
		Plain: ":8080",
		TLS:   &TLSConfig{Listen: ":8443", CertFile: "/dev/null", KeyFile: "/dev/null"},
	}, okHandler())
	if err != nil {
		t.Fatal(err)
	}
	if servers.Plain == nil || servers.TLS == nil {
		t.Fatalf("Plain=%v TLS=%v, want both non-nil", servers.Plain, servers.TLS)
	}
	if got := servers.All(); len(got) != 2 || got[0] != servers.Plain || got[1] != servers.TLS {
		t.Fatalf("All() = %v, want [Plain, TLS]", got)
	}
}

func TestBuildWithOnlyTLSReturnsTLSOnly(t *testing.T) {
	servers, err := Build(Config{
		TLS: &TLSConfig{Listen: ":8443", CertFile: "/dev/null", KeyFile: "/dev/null"},
	}, okHandler())
	if err != nil {
		t.Fatal(err)
	}
	if servers.TLS == nil {
		t.Fatal("TLS = nil, want non-nil")
	}
	if servers.Plain != nil {
		t.Fatal("Plain = non-nil, want nil")
	}
}

func TestBuildAppliesServerOptions(t *testing.T) {
	opts := ServerOptions{
		ReadHeaderTimeout: 7 * time.Second,
		IdleTimeout:       11 * time.Second,
		MaxHeaderBytes:    4096,
	}
	servers, err := Build(Config{
		Plain:  "127.0.0.1:0",
		TLS:    &TLSConfig{Listen: "127.0.0.1:0"},
		Server: opts,
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if err != nil {
		t.Fatal(err)
	}
	for _, srv := range servers.All() {
		if srv.ReadHeaderTimeout != 7*time.Second {
			t.Errorf("ReadHeaderTimeout = %v", srv.ReadHeaderTimeout)
		}
		if srv.IdleTimeout != 11*time.Second {
			t.Errorf("IdleTimeout = %v", srv.IdleTimeout)
		}
		if srv.MaxHeaderBytes != 4096 {
			t.Errorf("MaxHeaderBytes = %d", srv.MaxHeaderBytes)
		}
	}
}

func TestBuildZeroServerOptionsKeepsDefaults(t *testing.T) {
	servers, err := Build(Config{TLS: &TLSConfig{Listen: "127.0.0.1:0"}}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if err != nil {
		t.Fatal(err)
	}
	if servers.TLS.ReadHeaderTimeout != 0 || servers.TLS.MaxHeaderBytes != 0 {
		t.Fatalf("expected Go defaults, got rht=%v mhb=%d", servers.TLS.ReadHeaderTimeout, servers.TLS.MaxHeaderBytes)
	}
}
