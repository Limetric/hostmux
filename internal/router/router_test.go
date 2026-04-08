package router

import (
	"sync"
	"testing"
)

func TestLookupEmpty(t *testing.T) {
	r := New()
	if _, ok := r.Lookup("nope.test"); ok {
		t.Fatal("expected miss on empty router")
	}
}

func TestAddAndLookup(t *testing.T) {
	r := New()
	if err := r.Add("config", []string{"a.test"}, "http://127.0.0.1:8080"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, ok := r.Lookup("a.test")
	if !ok {
		t.Fatal("expected hit")
	}
	if got != "http://127.0.0.1:8080" {
		t.Fatalf("upstream = %q", got)
	}
}

func TestAddMultiHostAtomicSuccess(t *testing.T) {
	r := New()
	if err := r.Add("socket:1", []string{"a.test", "b.test"}, "http://127.0.0.1:9000"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	for _, h := range []string{"a.test", "b.test"} {
		if _, ok := r.Lookup(h); !ok {
			t.Fatalf("missing host %q", h)
		}
	}
}

func TestAddRejectsCrossSourceCollision(t *testing.T) {
	r := New()
	_ = r.Add("config", []string{"a.test"}, "http://127.0.0.1:8080")
	err := r.Add("socket:1", []string{"a.test"}, "http://127.0.0.1:9000")
	if err == nil {
		t.Fatal("expected collision error")
	}
}

func TestAddMultiHostPartialCollisionRollsBack(t *testing.T) {
	r := New()
	_ = r.Add("config", []string{"taken.test"}, "http://127.0.0.1:8080")
	err := r.Add("socket:1", []string{"new.test", "taken.test"}, "http://127.0.0.1:9000")
	if err == nil {
		t.Fatal("expected collision error")
	}
	if _, ok := r.Lookup("new.test"); ok {
		t.Fatal("partial registration leaked: new.test should not be present")
	}
	got, _ := r.Lookup("taken.test")
	if got != "http://127.0.0.1:8080" {
		t.Fatalf("taken.test was overwritten: %q", got)
	}
}

func TestAddSameSourceIsIdempotentForOwnedHosts(t *testing.T) {
	r := New()
	_ = r.Add("socket:1", []string{"a.test"}, "http://127.0.0.1:9000")
	if err := r.Add("socket:1", []string{"a.test", "b.test"}, "http://127.0.0.1:9001"); err != nil {
		t.Fatalf("expected same-source re-add to succeed, got: %v", err)
	}
	got, _ := r.Lookup("a.test")
	if got != "http://127.0.0.1:9001" {
		t.Fatalf("a.test upstream = %q, want refreshed value", got)
	}
}

func TestRemoveBySource(t *testing.T) {
	r := New()
	_ = r.Add("config", []string{"a.test"}, "http://127.0.0.1:8080")
	_ = r.Add("socket:1", []string{"b.test"}, "http://127.0.0.1:9000")
	r.RemoveBySource("config")
	if _, ok := r.Lookup("a.test"); ok {
		t.Fatal("a.test should be removed")
	}
	if _, ok := r.Lookup("b.test"); !ok {
		t.Fatal("b.test should remain")
	}
}

func TestReplaceSourceInPlaceUpdate(t *testing.T) {
	r := New()
	_ = r.Add("config", []string{"api.local"}, "http://127.0.0.1:8080")
	err := r.ReplaceSource("config", []Entry{{Source: "config", Hosts: []string{"api.local"}, Upstream: "http://127.0.0.1:9999"}})
	if err != nil {
		t.Fatalf("ReplaceSource: %v", err)
	}
	got, _ := r.Lookup("api.local")
	if got != "http://127.0.0.1:9999" {
		t.Fatalf("upstream = %q, want refreshed value", got)
	}
}

func TestReplaceSourceRejectsCrossSourceCollision(t *testing.T) {
	r := New()
	_ = r.Add("socket:1", []string{"taken.test"}, "http://127.0.0.1:9000")
	err := r.ReplaceSource("config", []Entry{{Source: "config", Hosts: []string{"taken.test"}, Upstream: "http://127.0.0.1:8080"}})
	if err == nil {
		t.Fatal("expected reload to be rejected")
	}
	got, _ := r.Lookup("taken.test")
	if got != "http://127.0.0.1:9000" {
		t.Fatalf("ephemeral entry was disturbed: %q", got)
	}
}

func TestReplaceSourceRejectsInternalDuplicate(t *testing.T) {
	r := New()
	err := r.ReplaceSource("config", []Entry{
		{Source: "config", Hosts: []string{"x.test"}, Upstream: "http://127.0.0.1:1"},
		{Source: "config", Hosts: []string{"x.test"}, Upstream: "http://127.0.0.1:2"},
	})
	if err == nil {
		t.Fatal("expected duplicate-host rejection")
	}
}

func TestReplaceSourceClearsRemovedEntries(t *testing.T) {
	r := New()
	_ = r.Add("config", []string{"old.test"}, "http://127.0.0.1:8080")
	if err := r.ReplaceSource("config", nil); err != nil {
		t.Fatalf("ReplaceSource(nil): %v", err)
	}
	if _, ok := r.Lookup("old.test"); ok {
		t.Fatal("old.test should be cleared by empty replace")
	}
}

func TestSnapshotReturnsAllEntriesSorted(t *testing.T) {
	r := New()
	_ = r.Add("config", []string{"b.test"}, "http://127.0.0.1:1")
	_ = r.Add("socket:1", []string{"a.test"}, "http://127.0.0.1:2")
	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d", len(snap))
	}
	if snap[0].Source != "config" || snap[1].Source != "socket:1" {
		t.Fatalf("snapshot not sorted by source: %+v", snap)
	}
}

func TestConcurrentAddLookup(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			host := "h.test"
			_ = r.Add("source", []string{host}, "http://127.0.0.1:1")
			_, _ = r.Lookup(host)
		}(i)
	}
	wg.Wait()
}
