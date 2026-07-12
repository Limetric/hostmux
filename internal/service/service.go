// Package service renders the per-platform service definitions hostmux can
// install as a user-level service: a launchd agent on macOS and a systemd
// user unit on Linux. The renderers are pure functions so the generated files
// are deterministic and unit-testable; installation/activation is handled by
// the caller.
package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

const (
	// DarwinLabel is the launchd label / plist filename stem.
	DarwinLabel = "com.limetric.hostmux"
	// LinuxUnit is the systemd user unit filename.
	LinuxUnit = "hostmux.service"
)

// Params describes the service to render.
type Params struct {
	// BinPath is the absolute path to the hostmux binary.
	BinPath string
	// ConfigPath is an optional --config argument. Empty omits the flag.
	ConfigPath string
	// LogPath is where stdout/stderr are written (launchd only). Empty omits.
	LogPath string
}

// args returns the ExecStart/ProgramArguments tokens after the binary.
func startArgs(p Params) []string {
	args := []string{"start", "--foreground"}
	if p.ConfigPath != "" {
		args = append(args, "--config", p.ConfigPath)
	}
	return args
}

// LaunchdPlist renders a launchd user-agent plist.
func LaunchdPlist(p Params) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString("<dict>\n")
	fmt.Fprintf(&b, "  <key>Label</key>\n  <string>%s</string>\n", xmlString(DarwinLabel))
	b.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	fmt.Fprintf(&b, "    <string>%s</string>\n", xmlString(p.BinPath))
	for _, a := range startArgs(p) {
		fmt.Fprintf(&b, "    <string>%s</string>\n", xmlString(a))
	}
	b.WriteString("  </array>\n")
	b.WriteString("  <key>RunAtLoad</key>\n  <true/>\n")
	b.WriteString("  <key>KeepAlive</key>\n  <true/>\n")
	if p.LogPath != "" {
		fmt.Fprintf(&b, "  <key>StandardOutPath</key>\n  <string>%s</string>\n", xmlString(p.LogPath))
		fmt.Fprintf(&b, "  <key>StandardErrorPath</key>\n  <string>%s</string>\n", xmlString(p.LogPath))
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

// SystemdUnit renders a systemd user service unit.
func SystemdUnit(p Params) string {
	tokens := append([]string{p.BinPath}, startArgs(p)...)
	quoted := make([]string, len(tokens))
	for i, t := range tokens {
		quoted[i] = systemdQuote(t)
	}
	execStart := strings.Join(quoted, " ")
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=hostmux host-routed reverse proxy\n")
	b.WriteString("After=network-online.target\n")
	b.WriteString("Wants=network-online.target\n\n")
	b.WriteString("[Service]\n")
	fmt.Fprintf(&b, "ExecStart=%s\n", execStart)
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=2\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=default.target\n")
	return b.String()
}

// xmlString escapes a value for inclusion in plist <string> element content,
// so a path containing &, <, >, or quotes cannot corrupt or inject into the
// generated plist.
func xmlString(s string) string {
	var buf bytes.Buffer
	// EscapeText only errors on write failures, which bytes.Buffer never
	// returns; ignore it.
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// systemdQuote renders one ExecStart token for a systemd command line.
// Systemd word-splits ExecStart on whitespace and treats % as a specifier
// introducer, so a token with a space, %, quote, backslash, or control
// character would otherwise split the command, be misinterpreted, or (via a
// newline) inject additional unit directives. Such tokens are wrapped in a
// double-quoted, C-escaped string; ordinary tokens (the common case) are
// emitted verbatim so the generated unit stays readable.
func systemdQuote(s string) string {
	// % introduces a specifier and $ introduces variable expansion (both even
	// inside double quotes); ; and whitespace affect word/command parsing.
	if s != "" && !strings.ContainsAny(s, " \t\r\n\"'\\%$;") {
		return s
	}
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
		"%", "%%",
		"$", "$$",
	)
	return `"` + r.Replace(s) + `"`
}
