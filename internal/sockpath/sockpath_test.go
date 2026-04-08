package sockpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDefaultsToHomeWhenNothingSet(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	got, err := Resolve(Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(tmp, ".hostmux", "hostmux.sock")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveUsesXDGRuntimeDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", tmp)

	got, _ := Resolve(Options{})
	want := filepath.Join(tmp, "hostmux.sock")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveReadsDiscoveryFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	hostmuxDir := filepath.Join(tmp, ".hostmux")
	os.MkdirAll(hostmuxDir, 0o755)
	os.WriteFile(filepath.Join(hostmuxDir, "socket"), []byte("/custom/path.sock\n"), 0o644)

	got, _ := Resolve(Options{})
	if got != "/custom/path.sock" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveEnvOverridesDiscoveryFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("HOSTMUX_SOCKET", "/from/env.sock")

	hostmuxDir := filepath.Join(tmp, ".hostmux")
	os.MkdirAll(hostmuxDir, 0o755)
	os.WriteFile(filepath.Join(hostmuxDir, "socket"), []byte("/from/file.sock\n"), 0o644)

	got, _ := Resolve(Options{})
	if got != "/from/env.sock" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveFlagOverridesEverything(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("HOSTMUX_SOCKET", "/from/env.sock")

	got, _ := Resolve(Options{Flag: "/from/flag.sock"})
	if got != "/from/flag.sock" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveServeUsesConfigSocket(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	got, _ := ResolveServe(Options{ConfigSocket: "/from/config.sock"})
	if got != "/from/config.sock" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteAndRemoveDiscoveryFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := WriteDiscovery("/some/sock"); err != nil {
		t.Fatalf("WriteDiscovery: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(tmp, ".hostmux", "socket"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "/some/sock\n" {
		t.Fatalf("file = %q", b)
	}
	if err := RemoveDiscovery(); err != nil {
		t.Fatalf("RemoveDiscovery: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".hostmux", "socket")); !os.IsNotExist(err) {
		t.Fatal("file still exists after RemoveDiscovery")
	}
}

func TestPIDFilePathFor(t *testing.T) {
	cases := []struct {
		name string
		sock string
		want string
	}{
		{"dotSockSuffixReplaced", "/tmp/hm/hostmux.sock", "/tmp/hm/hostmux.pid"},
		{"noSockSuffixAppended", "/tmp/hm/custom", "/tmp/hm/custom.pid"},
		{"sockInMiddleNotTouched", "/tmp/hm/my.sock.thing", "/tmp/hm/my.sock.thing.pid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PIDFilePathFor(tc.sock)
			if got != tc.want {
				t.Fatalf("PIDFilePathFor(%q) = %q, want %q", tc.sock, got, tc.want)
			}
		})
	}
}

func TestIsExplicit(t *testing.T) {
	t.Setenv("HOSTMUX_SOCKET", "")
	if IsExplicit(Options{}) {
		t.Fatal("empty options should not be explicit")
	}
	if !IsExplicit(Options{Flag: "/x"}) {
		t.Fatal("flag should be explicit")
	}
	t.Setenv("HOSTMUX_SOCKET", "/y")
	if !IsExplicit(Options{}) {
		t.Fatal("env should be explicit")
	}
}
