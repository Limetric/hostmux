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

func TestLaunchdPlistEscapesXML(t *testing.T) {
	out := LaunchdPlist(Params{BinPath: "/Users/x/R&D/host<mux", ConfigPath: "/c/\"weird\".toml"})
	// Raw metacharacters must not appear inside string content; entities must.
	if strings.Contains(out, "R&D") || strings.Contains(out, "host<mux") {
		t.Fatalf("XML metacharacters not escaped:\n%s", out)
	}
	if !strings.Contains(out, "R&amp;D") || !strings.Contains(out, "host&lt;mux") {
		t.Fatalf("expected escaped entities:\n%s", out)
	}
	// LogPath is also user-controlled and must be escaped.
	out2 := LaunchdPlist(Params{BinPath: "/bin/hostmux", LogPath: "/l/a&b>c.log"})
	if strings.Contains(out2, "a&b>c") {
		t.Fatalf("LogPath metacharacters not escaped:\n%s", out2)
	}
	if !strings.Contains(out2, "a&amp;b&gt;c") {
		t.Fatalf("expected escaped LogPath entities:\n%s", out2)
	}
}

func TestSystemdUnitQuotesSpacesAndEscapesPercent(t *testing.T) {
	out := SystemdUnit(Params{BinPath: "/opt/my apps/hostmux", ConfigPath: "/c/50%off.toml"})
	// A space-containing binary path must be quoted so it stays one argument.
	if !strings.Contains(out, `"/opt/my apps/hostmux"`) {
		t.Fatalf("space-containing path not quoted:\n%s", out)
	}
	// % must be doubled so systemd does not treat it as a specifier.
	if strings.Contains(out, "50%off") || !strings.Contains(out, "50%%off") {
		t.Fatalf("percent not escaped:\n%s", out)
	}
}

func TestSystemdUnitEscapesDollarAndSemicolon(t *testing.T) {
	out := SystemdUnit(Params{BinPath: "/opt/hostmux", ConfigPath: "/c/${HOME}/a;b.toml"})
	// The token must be quoted (contains $ and ;) and $ doubled to $$ so
	// systemd does not expand a variable. Assert the exact literal token.
	if !strings.Contains(out, `"/c/$${HOME}/a;b.toml"`) {
		t.Fatalf("token not quoted/escaped as a single literal arg:\n%s", out)
	}
	// The unescaped, expandable form must never appear as a bare argument.
	if strings.Contains(out, " ${HOME}") || strings.Contains(out, "=${HOME}") {
		t.Fatalf("unescaped $ expansion present:\n%s", out)
	}
}

func TestSystemdUnitRejectsNewlineInjection(t *testing.T) {
	out := SystemdUnit(Params{BinPath: "/bin/hostmux", ConfigPath: "/c.toml\nExecStartPost=/bin/evil"})
	// A newline in a path must be escaped, never emitted as a real line that
	// could inject an additional directive.
	if strings.Contains(out, "\nExecStartPost=") {
		t.Fatalf("newline injected a directive:\n%s", out)
	}
	if !strings.Contains(out, `\n`) {
		t.Fatalf("expected escaped newline:\n%s", out)
	}
}
