package main

import (
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/sockproto"
)

func TestResolveRequestedHosts_ExpandsBareLabelsWithDomain(t *testing.T) {
	got, err := resolveRequestedHosts([]string{"api"}, hostResolveOptions{
		Domain:   "example.com",
		NoPrefix: true,
	})
	if err != nil {
		t.Fatalf("resolveRequestedHosts: %v", err)
	}
	if want := []string{"api.example.com"}; !slices.Equal(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestResolveRequestedHosts_StripsTrailingDotFromDomain(t *testing.T) {
	got, err := resolveRequestedHosts([]string{"api"}, hostResolveOptions{
		Domain:   "example.com.",
		NoPrefix: true,
	})
	if err != nil {
		t.Fatalf("resolveRequestedHosts: %v", err)
	}
	if want := []string{"api.example.com"}; !slices.Equal(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestResolveRequestedHosts_AppliesPrefixBeforeExpansion(t *testing.T) {
	got, err := resolveRequestedHosts([]string{"svc"}, hostResolveOptions{
		Domain:   "x.test",
		Prefix:   "feat",
		NoPrefix: false,
	})
	if err != nil {
		t.Fatalf("resolveRequestedHosts: %v", err)
	}
	if want := []string{"feat-svc.x.test"}; !slices.Equal(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestResolveRequestedHosts_NoPrefixFlagSkipsWorktreePrefix(t *testing.T) {
	got, err := resolveRequestedHosts([]string{"api"}, hostResolveOptions{
		Domain:   "app.local",
		NoPrefix: true,
	})
	if err != nil {
		t.Fatalf("resolveRequestedHosts: %v", err)
	}
	if want := []string{"api.app.local"}; !slices.Equal(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestResolveRequestedHosts_PreservesFullHostname(t *testing.T) {
	got, err := resolveRequestedHosts([]string{"api.other.com"}, hostResolveOptions{
		Domain:   "ignored.example",
		NoPrefix: true,
	})
	if err != nil {
		t.Fatalf("resolveRequestedHosts: %v", err)
	}
	if want := []string{"api.other.com"}; !slices.Equal(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestLookupDaemonInfoClient_Success(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()

	go func() {
		defer c2.Close()
		dec := sockproto.NewDecoder(c2)
		enc := sockproto.NewEncoder(c2)
		msg, err := dec.Decode()
		if err != nil || msg.Op != sockproto.OpInfo {
			return
		}
		pub := true
		_ = enc.Encode(&sockproto.Message{Ok: true, Domain: "example.com", PublicHTTPS: &pub})
	}()

	domain, pub, err := lookupDaemonInfoClient(sockproto.NewEncoder(c1), sockproto.NewDecoder(c1))
	if err != nil {
		t.Fatalf("lookupDaemonInfoClient: %v", err)
	}
	if domain != "example.com" {
		t.Fatalf("domain = %q", domain)
	}
	if !pub {
		t.Fatal("PublicHTTPS = false, want true")
	}
}

func TestLookupDaemonInfoClient_PropagatesDaemonError(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()

	go func() {
		defer c2.Close()
		dec := sockproto.NewDecoder(c2)
		enc := sockproto.NewEncoder(c2)
		if _, err := dec.Decode(); err != nil {
			return
		}
		_ = enc.Encode(&sockproto.Message{Ok: false, Error: "refused"})
	}()

	_, _, err := lookupDaemonInfoClient(sockproto.NewEncoder(c1), sockproto.NewDecoder(c1))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Fatalf("error = %v", err)
	}
}

func TestLookupDaemonInfoClient_RejectedWithoutMessage(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()

	go func() {
		defer c2.Close()
		dec := sockproto.NewDecoder(c2)
		enc := sockproto.NewEncoder(c2)
		if _, err := dec.Decode(); err != nil {
			return
		}
		_ = enc.Encode(&sockproto.Message{Ok: false})
	}()

	_, _, err := lookupDaemonInfoClient(sockproto.NewEncoder(c1), sockproto.NewDecoder(c1))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "daemon rejected info lookup") {
		t.Fatalf("error = %v", err)
	}
}

func TestLookupDaemonInfoClient_PublicHTTPSFalseWhenSet(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()

	go func() {
		defer c2.Close()
		dec := sockproto.NewDecoder(c2)
		enc := sockproto.NewEncoder(c2)
		if _, err := dec.Decode(); err != nil {
			return
		}
		pub := false
		_ = enc.Encode(&sockproto.Message{Ok: true, Domain: "plain.local", PublicHTTPS: &pub})
	}()

	_, pub, err := lookupDaemonInfoClient(sockproto.NewEncoder(c1), sockproto.NewDecoder(c1))
	if err != nil {
		t.Fatalf("lookupDaemonInfoClient: %v", err)
	}
	if pub {
		t.Fatal("PublicHTTPS = true, want false")
	}
}

func TestLookupDaemonDomainClient_DelegatesToInfo(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()

	go func() {
		defer c2.Close()
		dec := sockproto.NewDecoder(c2)
		enc := sockproto.NewEncoder(c2)
		if _, err := dec.Decode(); err != nil {
			return
		}
		pub := true
		_ = enc.Encode(&sockproto.Message{Ok: true, Domain: "d.example", PublicHTTPS: &pub})
	}()

	domain, err := lookupDaemonDomainClient(sockproto.NewEncoder(c1), sockproto.NewDecoder(c1))
	if err != nil {
		t.Fatalf("lookupDaemonDomainClient: %v", err)
	}
	if domain != "d.example" {
		t.Fatalf("domain = %q", domain)
	}
}
