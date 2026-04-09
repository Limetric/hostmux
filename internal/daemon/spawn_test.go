package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortSockDir avoids macOS sun_path length limits for Unix sockets (paths from
// t.TempDir() can exceed ~104 bytes when the test name is long).
func shortSockDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hmu")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestEnsureRunningReturnsImmediatelyIfSocketExists(t *testing.T) {
	dir := shortSockDir(t)
	path := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go acceptLoop(ln)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := EnsureRunning(ctx, path, EnsureOpts{}); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
}

func TestEnsureRunningTimesOutWhenNoSpawn(t *testing.T) {
	dir := shortSockDir(t)
	path := filepath.Join(dir, "s")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := EnsureRunning(ctx, path, EnsureOpts{Spawn: func() error { return nil }})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestEnsureRunningSucceedsWhenSpawnCreatesSocket(t *testing.T) {
	dir := shortSockDir(t)
	path := filepath.Join(dir, "s")
	spawned := false
	spawn := func() error {
		spawned = true
		go func() {
			time.Sleep(50 * time.Millisecond)
			ln, err := net.Listen("unix", path)
			if err != nil {
				return
			}
			go acceptLoop(ln)
		}()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := EnsureRunning(ctx, path, EnsureOpts{Spawn: spawn}); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if !spawned {
		t.Fatal("Spawn was never called")
	}
}

func TestEnsureRunningSpawnsWhenSocketFileNotDialable(t *testing.T) {
	dir := shortSockDir(t)
	path := filepath.Join(dir, "s")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	spawned := false
	spawn := func() error {
		spawned = true
		if err := os.Remove(path); err != nil {
			return err
		}
		go func() {
			time.Sleep(50 * time.Millisecond)
			ln, err := net.Listen("unix", path)
			if err != nil {
				return
			}
			go acceptLoop(ln)
		}()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := EnsureRunning(ctx, path, EnsureOpts{Spawn: spawn}); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if !spawned {
		t.Fatal("Spawn was never called")
	}
}

func acceptLoop(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		_ = c.Close()
	}
}
