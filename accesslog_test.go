package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Limetric/hostmux/internal/proxy"
)

func sampleRecord() proxy.AccessRecord {
	return proxy.AccessRecord{
		Method:   "GET",
		Host:     "api.example.com",
		Path:     "/v1/users",
		Status:   200,
		Bytes:    1234,
		Duration: 5 * time.Millisecond,
		Upstream: "http://127.0.0.1:8080",
		Source:   "socket:3",
	}
}

func TestAccessLoggerText(t *testing.T) {
	var buf bytes.Buffer
	l := newAccessLogger(&buf, "text")
	l.LogAccess(sampleRecord())
	out := buf.String()
	for _, want := range []string{"access", "GET", "api.example.com", "/v1/users", "200", "http://127.0.0.1:8080", "src=socket:3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("text output missing %q\n%s", want, out)
		}
	}
}

func TestAccessLoggerTextEscapesControlChars(t *testing.T) {
	var buf bytes.Buffer
	l := newAccessLogger(&buf, "text")
	rec := sampleRecord()
	// A percent-decoded path containing a newline must not forge a log line.
	rec.Path = "/\naccess GET evil.example/ -> 200 (0.0ms) http://attacker src=socket:9"
	l.LogAccess(rec)
	out := buf.String()
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("expected exactly one line, got %d:\n%q", strings.Count(out, "\n"), out)
	}
	if strings.Contains(out, "\naccess") {
		t.Fatalf("newline was not escaped:\n%q", out)
	}
	if !strings.Contains(out, `\x0a`) {
		t.Fatalf("expected escaped newline \\x0a in output:\n%q", out)
	}
}

func TestAccessLoggerTextEscapesUnicodeSeparatorsAndUpstream(t *testing.T) {
	var buf bytes.Buffer
	l := newAccessLogger(&buf, "text")
	rec := sampleRecord()
	rec.Host = "a\u2028b.example" // U+2028 line separator
	rec.Upstream = "http://x\ny"  // client-supplied upstream with a newline
	l.LogAccess(rec)
	out := buf.String()
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("expected exactly one line, got %d:\n%q", strings.Count(out, "\n"), out)
	}
	if strings.ContainsRune(out, '\u2028') {
		t.Fatalf("U+2028 not escaped:\n%q", out)
	}
	if !strings.Contains(out, `\u2028`) {
		t.Fatalf("expected escaped \\u2028 in output:\n%q", out)
	}
	if !strings.Contains(out, `\x0a`) {
		t.Fatalf("expected upstream newline escaped as \\x0a:\n%q", out)
	}
}

func TestAccessLoggerJSON(t *testing.T) {
	var buf bytes.Buffer
	l := newAccessLogger(&buf, "json")
	rec := sampleRecord()
	rec.Err = "boom"
	l.LogAccess(rec)

	var got accessLogLine
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if got.Type != "access" || got.Method != "GET" || got.Host != "api.example.com" || got.Status != 200 {
		t.Fatalf("got = %+v", got)
	}
	if got.Upstream != "http://127.0.0.1:8080" || got.Source != "socket:3" || got.Error != "boom" {
		t.Fatalf("got = %+v", got)
	}
	if got.DurationMs <= 0 {
		t.Fatalf("duration_ms = %v", got.DurationMs)
	}
}

func TestAccessLoggerDefaultsToText(t *testing.T) {
	var buf bytes.Buffer
	l := newAccessLogger(&buf, "") // empty == text
	l.LogAccess(sampleRecord())
	if !strings.HasPrefix(buf.String(), "access ") {
		t.Fatalf("expected text format, got %q", buf.String())
	}
}
