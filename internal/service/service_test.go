package service

import (
	"strings"
	"testing"
)

func TestLaunchdPlist(t *testing.T) {
	out := LaunchdPlist(Params{BinPath: "/usr/local/bin/hostmux", ConfigPath: "/etc/hostmux.toml", LogPath: "/tmp/hostmux.log"})
	for _, want := range []string{
		"<plist version=\"1.0\">",
		"<string>com.limetric.hostmux</string>",
		"<string>/usr/local/bin/hostmux</string>",
		"<string>start</string>",
		"<string>--foreground</string>",
		"<string>--config</string>",
		"<string>/etc/hostmux.toml</string>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"/tmp/hostmux.log",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plist missing %q:\n%s", want, out)
		}
	}
}

func TestLaunchdPlistNoConfig(t *testing.T) {
	out := LaunchdPlist(Params{BinPath: "/bin/hostmux"})
	if strings.Contains(out, "--config") {
		t.Fatalf("expected no --config:\n%s", out)
	}
	if strings.Contains(out, "StandardOutPath") {
		t.Fatalf("expected no log path when unset:\n%s", out)
	}
}

func TestSystemdUnit(t *testing.T) {
	out := SystemdUnit(Params{BinPath: "/usr/bin/hostmux", ConfigPath: "/home/u/.config/hostmux/hostmux.toml"})
	for _, want := range []string{
		"[Unit]",
		"[Service]",
		"ExecStart=/usr/bin/hostmux start --foreground --config /home/u/.config/hostmux/hostmux.toml",
		"Restart=on-failure",
		"WantedBy=default.target",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("unit missing %q:\n%s", want, out)
		}
	}
}

func TestSystemdUnitNoConfig(t *testing.T) {
	out := SystemdUnit(Params{BinPath: "/usr/bin/hostmux"})
	if !strings.Contains(out, "ExecStart=/usr/bin/hostmux start --foreground\n") {
		t.Fatalf("expected no --config in ExecStart:\n%s", out)
	}
}
