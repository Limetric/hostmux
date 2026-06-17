package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func portOf(t *testing.T, addr string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	p, _ := strconv.Atoi(portStr)
	return p
}

func TestNormalizeWaitPath(t *testing.T) {
	cases := map[string]string{"": "", "/healthz": "/healthz", "healthz": "/healthz"}
	for in, want := range cases {
		if got := normalizeWaitPath(in); got != want {
			t.Errorf("normalizeWaitPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWaitForReadyTCPSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := portOf(t, ln.Addr().String())
	childDone := make(chan struct{}) // never closed
	res := waitForReady("127.0.0.1", port, "", time.Second, 20*time.Millisecond, childDone, nil)
	if res != readyOK {
		t.Fatalf("res = %v, want readyOK", res)
	}
}

func TestWaitForReadyHTTPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	port := portOf(t, srv.Listener.Addr().String())
	childDone := make(chan struct{})
	res := waitForReady("127.0.0.1", port, "/healthz", time.Second, 20*time.Millisecond, childDone, nil)
	if res != readyOK {
		t.Fatalf("res = %v, want readyOK", res)
	}
}

func TestWaitForReadyTimeout(t *testing.T) {
	// Nothing listening on this port (allocate then close).
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := portOf(t, ln.Addr().String())
	ln.Close()

	childDone := make(chan struct{}) // never closed
	start := time.Now()
	res := waitForReady("127.0.0.1", port, "", 150*time.Millisecond, 20*time.Millisecond, childDone, nil)
	if res != readyTimeout {
		t.Fatalf("res = %v, want readyTimeout", res)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("waited too long: %v", time.Since(start))
	}
}

func TestWaitForReadyChildExits(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := portOf(t, ln.Addr().String())
	ln.Close() // nothing ever becomes ready

	childDone := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(childDone)
	}()
	res := waitForReady("127.0.0.1", port, "", 5*time.Second, 20*time.Millisecond, childDone, nil)
	if res != readyChildExited {
		t.Fatalf("res = %v, want readyChildExited", res)
	}
}
