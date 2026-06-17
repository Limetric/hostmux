package sockpath

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDefaultsToHomeWhenNothingSet(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
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
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", tmp)

	got, _ := Resolve(Options{})
	want := filepath.Join(tmp, "hostmux.sock")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLiveDiscoveryReturnsLiveSocket(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	discovered := filepath.Join(sockDir, "custom.sock")
	ln, err := net.Listen("unix", discovered)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	hostmuxDir := filepath.Join(tmp, ".hostmux")
	os.MkdirAll(hostmuxDir, 0o755)
	os.WriteFile(filepath.Join(hostmuxDir, "socket"), []byte(discovered+"\n"), 0o644)

	got, ok := LiveDiscovery()
	if !ok {
		t.Fatal("expected live discovery")
	}
	if got != discovered {
		t.Fatalf("got %q want %q", got, discovered)
	}
}

func TestLiveDiscoveryFalseWhenMissingOrStale(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	if _, ok := LiveDiscovery(); ok {
		t.Fatal("expected no discovery")
	}

	hostmuxDir := filepath.Join(tmp, ".hostmux")
	os.MkdirAll(hostmuxDir, 0o755)
	os.WriteFile(filepath.Join(hostmuxDir, "socket"), []byte("/missing/custom.sock\n"), 0o644)
	if _, ok := LiveDiscovery(); ok {
		t.Fatal("expected stale discovery to be ignored")
	}
}

func TestResolveReadsDiscoveryFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	discovered := filepath.Join(sockDir, "custom.sock")
	ln, err := net.Listen("unix", discovered)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	hostmuxDir := filepath.Join(tmp, ".hostmux")
	os.MkdirAll(hostmuxDir, 0o755)
	os.WriteFile(filepath.Join(hostmuxDir, "socket"), []byte(discovered+"\n"), 0o644)

	got, _ := Resolve(Options{})
	if got != discovered {
		t.Fatalf("got %q", got)
	}
}

func TestResolveIgnoresStaleDiscoveryFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	hostmuxDir := filepath.Join(tmp, ".hostmux")
	if err := os.MkdirAll(hostmuxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	discoveryPath := filepath.Join(hostmuxDir, "socket")
	if err := os.WriteFile(discoveryPath, []byte("/missing/custom.sock\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(tmp, ".hostmux", "hostmux.sock")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := os.Stat(discoveryPath); !os.IsNotExist(err) {
		t.Fatalf("stale discovery file still present, stat err=%v", err)
	}
}

func TestResolveIgnoresDiscoveryRegularFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	staleDir := t.TempDir()
	discovered := filepath.Join(staleDir, "not-a-socket.sock")
	if err := os.WriteFile(discovered, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	hostmuxDir := filepath.Join(tmp, ".hostmux")
	if err := os.MkdirAll(hostmuxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	discoveryPath := filepath.Join(hostmuxDir, "socket")
	if err := os.WriteFile(discoveryPath, []byte(discovered+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(tmp, ".hostmux", "hostmux.sock")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := os.Stat(discoveryPath); !os.IsNotExist(err) {
		t.Fatalf("stale discovery file still present, stat err=%v", err)
	}
}

func TestDiscoveryAliveDoesNotCreatePIDFile(t *testing.T) {
	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })

	sockPath := filepath.Join(sockDir, "dead.sock")
	if err := os.WriteFile(sockPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	pidPath := PIDFilePathFor(sockPath)

	if discoveryAlive(sockPath) {
		t.Fatal("expected dead socket to be not alive")
	}
	if _, err := os.Stat(pidPath); err == nil {
		t.Fatalf("pid file %q should not have been created", pidPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat pid file: %v", err)
	}
}

func TestResolveEnvOverridesDiscoveryFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
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
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "/from/env.sock")

	got, _ := Resolve(Options{Flag: "/from/flag.sock"})
	if got != "/from/flag.sock" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveServeUsesConfigSocket(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
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
	t.Setenv("USERPROFILE", tmp)
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
	// Build inputs/expectations with filepath.Join so the test asserts the
	// same separator the implementation produces on every OS (Windows uses
	// backslashes).
	dir := filepath.Join("base", "hm")
	cases := []struct {
		name string
		sock string
		want string
	}{
		{"dotSockSuffixReplaced", filepath.Join(dir, "hostmux.sock"), filepath.Join(dir, "hostmux.pid")},
		{"noSockSuffixAppended", filepath.Join(dir, "custom"), filepath.Join(dir, "custom.pid")},
		{"sockInMiddleNotTouched", filepath.Join(dir, "my.sock.thing"), filepath.Join(dir, "my.sock.thing.pid")},
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
