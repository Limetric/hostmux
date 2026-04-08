package listener

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
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
	if len(servers) != 1 {
		t.Fatalf("len = %d", len(servers))
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go servers[0].Serve(ln)
	defer servers[0].Shutdown(context.Background())

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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go servers[0].Serve(ln)
	defer servers[0].Shutdown(context.Background())

	// HTTP/2 client over plain TCP (h2c).
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

func TestBuildWithoutTLSReturnsOneServer(t *testing.T) {
	servers, err := Build(Config{Plain: ":8080"}, okHandler())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
}

func TestBuildWithTLSReturnsTwoServers(t *testing.T) {
	servers, err := Build(Config{
		Plain: ":8080",
		TLS:   &TLSConfig{Listen: ":8443", CertFile: "/dev/null", KeyFile: "/dev/null"},
	}, okHandler())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
}
