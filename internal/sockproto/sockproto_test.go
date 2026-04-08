package sockproto

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncodeDecodeRegister(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(&Message{Op: OpRegister, Hosts: []string{"a.test", "b.test"}, Upstream: "http://127.0.0.1:9000"}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec := NewDecoder(&buf)
	got, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Op != OpRegister {
		t.Fatalf("op = %q", got.Op)
	}
	if len(got.Hosts) != 2 || got.Hosts[0] != "a.test" {
		t.Fatalf("hosts = %v", got.Hosts)
	}
	if got.Upstream != "http://127.0.0.1:9000" {
		t.Fatalf("upstream = %q", got.Upstream)
	}
}

func TestEncodeDecodeOk(t *testing.T) {
	var buf bytes.Buffer
	NewEncoder(&buf).Encode(&Message{Ok: true})
	got, err := NewDecoder(&buf).Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Ok {
		t.Fatal("ok was lost")
	}
}

func TestEncodeDecodeError(t *testing.T) {
	var buf bytes.Buffer
	NewEncoder(&buf).Encode(&Message{Ok: false, Error: "host taken"})
	got, _ := NewDecoder(&buf).Decode()
	if got.Ok || got.Error != "host taken" {
		t.Fatalf("got %+v", got)
	}
}

func TestEncodeDecodeInfo(t *testing.T) {
	https := true
	var buf bytes.Buffer
	NewEncoder(&buf).Encode(&Message{Ok: true, Domain: "x.test", PublicHTTPS: &https})
	got, err := NewDecoder(&buf).Decode()
	if err != nil {
		t.Fatal(err)
	}
	if got.Domain != "x.test" || got.PublicHTTPS == nil || !*got.PublicHTTPS {
		t.Fatalf("got %+v", got)
	}
}

func TestEncodeDecodeList(t *testing.T) {
	var buf bytes.Buffer
	NewEncoder(&buf).Encode(&Message{
		Ok: true,
		Entries: []Entry{
			{Source: "config", Hosts: []string{"api.local"}, Upstream: "http://127.0.0.1:8080"},
		},
	})
	got, _ := NewDecoder(&buf).Decode()
	if len(got.Entries) != 1 || got.Entries[0].Hosts[0] != "api.local" {
		t.Fatalf("entries = %+v", got.Entries)
	}
}

func TestDecoderRejectsMalformed(t *testing.T) {
	dec := NewDecoder(strings.NewReader("{not json}\n"))
	if _, err := dec.Decode(); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestDecoderEOF(t *testing.T) {
	dec := NewDecoder(strings.NewReader(""))
	if _, err := dec.Decode(); err == nil {
		t.Fatal("expected EOF")
	}
}
