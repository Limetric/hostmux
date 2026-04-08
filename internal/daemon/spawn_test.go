package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureRunningReturnsImmediatelyIfSocketExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sock")
	// Create a socket-like file: any existing path is treated as "already running".
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := EnsureRunning(ctx, path, EnsureOpts{}); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
}

func TestEnsureRunningTimesOutWhenNoSpawn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sock")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := EnsureRunning(ctx, path, EnsureOpts{Spawn: func() error { return nil }})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestEnsureRunningSucceedsWhenSpawnCreatesSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sock")
	spawned := false
	spawn := func() error {
		spawned = true
		// simulate the daemon coming up after a small delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = os.WriteFile(path, nil, 0o644)
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
