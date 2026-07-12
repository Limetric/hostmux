package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode"

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
	// Every free-text field is sanitized: Method/Host/Path come straight from
	// the client, and Upstream is supplied by socket clients (OpRegister /
	// OpExpose), so any of them can carry line-breaking characters.
	src := sanitizeLogField(r.Source)
	if src == "" {
		src = "-"
	}
	up := sanitizeLogField(r.Upstream)
	if up == "" {
		up = "-"
	}
	errSuffix := ""
	if r.Err != "" {
		errSuffix = " error=" + sanitizeLogField(r.Err)
	}
	fmt.Fprintf(l.w, "access %s %s%s -> %d (%.1fms) %s src=%s%s\n",
		sanitizeLogField(r.Method), sanitizeLogField(r.Host), sanitizeLogField(r.Path),
		r.Status, durMs, up, src, errSuffix)
}

// sanitizeLogField escapes any character that could forge or corrupt a line
// in the text access log — ASCII C0 controls and DEL, C1 controls (including
// NEL U+0085), the Unicode line/paragraph separators (U+2028/U+2029), and
// format characters such as bidi overrides — as "\xNN" / "\uNNNN". Ordinary
// printable text (including multibyte UTF-8) passes through unchanged. The
// JSON format needs no equivalent because json.Marshal already escapes
// control characters.
func sanitizeLogField(s string) string {
	if strings.IndexFunc(s, isLogControl) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case !isLogControl(r):
			b.WriteRune(r)
		case r > 0xff:
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			fmt.Fprintf(&b, "\\x%02x", r)
		}
	}
	return b.String()
}

func isLogControl(r rune) bool {
	// U+2028 line separator and U+2029 paragraph separator are line breaks to
	// many viewers but are not in the Cc/Cf categories, so check them here.
	if r == '\u2028' || r == '\u2029' {
		return true
	}
	// unicode.IsControl covers C0 (incl. \n, \r, \t), DEL, and C1 (incl.
	// NEL U+0085); unicode.Cf covers format characters like bidi overrides.
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}
