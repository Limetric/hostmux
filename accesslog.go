package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/Limetric/hostmux/internal/config"
	"github.com/Limetric/hostmux/internal/proxy"
)

// accessLogger implements proxy.AccessLogger. It serializes records to an
// io.Writer in either a compact human format or one JSON object per line.
// A mutex guards concurrent writes so log lines never interleave.
type accessLogger struct {
	mu     sync.Mutex
	w      io.Writer
	asJSON bool
}

// newAccessLogger builds an accessLogger for the given format. Any value
// other than "json" produces the text format.
func newAccessLogger(w io.Writer, format string) *accessLogger {
	return &accessLogger{w: w, asJSON: format == config.LogFormatJSON}
}

// accessLogFormatName normalizes the configured format to a display name.
func accessLogFormatName(format string) string {
	if format == config.LogFormatJSON {
		return config.LogFormatJSON
	}
	return config.LogFormatText
}

// accessLogLine is the JSON shape of one access record. Duration is reported
// in milliseconds for readability; empty strings/zero values are omitted.
type accessLogLine struct {
	Type       string  `json:"type"`
	Method     string  `json:"method,omitempty"`
	Host       string  `json:"host,omitempty"`
	Path       string  `json:"path,omitempty"`
	Status     int     `json:"status,omitempty"`
	Bytes      int64   `json:"bytes,omitempty"`
	DurationMs float64 `json:"duration_ms"`
	Upstream   string  `json:"upstream,omitempty"`
	Source     string  `json:"source,omitempty"`
	Error      string  `json:"error,omitempty"`
}

func (l *accessLogger) LogAccess(r proxy.AccessRecord) {
	durMs := float64(r.Duration.Microseconds()) / 1000.0
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.asJSON {
		line := accessLogLine{
			Type:       "access",
			Method:     r.Method,
			Host:       r.Host,
			Path:       r.Path,
			Status:     r.Status,
			Bytes:      r.Bytes,
			DurationMs: durMs,
			Upstream:   r.Upstream,
			Source:     r.Source,
			Error:      r.Err,
		}
		b, err := json.Marshal(line)
		if err != nil {
			return
		}
		fmt.Fprintf(l.w, "%s\n", b)
		return
	}
	// Text format: "access METHOD host path -> status (Nms) upstream src=...".
	src := r.Source
	if src == "" {
		src = "-"
	}
	up := r.Upstream
	if up == "" {
		up = "-"
	}
	errSuffix := ""
	if r.Err != "" {
		errSuffix = " error=" + r.Err
	}
	fmt.Fprintf(l.w, "access %s %s%s -> %d (%.1fms) %s src=%s%s\n",
		r.Method, r.Host, r.Path, r.Status, durMs, up, src, errSuffix)
}
