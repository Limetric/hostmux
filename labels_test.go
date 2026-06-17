package main

import "testing"

func TestParseLabels(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		m, err := parseLabels(nil)
		if err != nil || m != nil {
			t.Fatalf("parseLabels(nil) = %v, %v", m, err)
		}
	})

	t.Run("key=value pairs", func(t *testing.T) {
		m, err := parseLabels([]string{"team=web", "kind=api"})
		if err != nil {
			t.Fatal(err)
		}
		if m["team"] != "web" || m["kind"] != "api" {
			t.Fatalf("m = %v", m)
		}
	})

	t.Run("value may contain equals", func(t *testing.T) {
		m, err := parseLabels([]string{"expr=a=b"})
		if err != nil {
			t.Fatal(err)
		}
		if m["expr"] != "a=b" {
			t.Fatalf("m = %v", m)
		}
	})

	t.Run("last value wins", func(t *testing.T) {
		m, _ := parseLabels([]string{"k=1", "k=2"})
		if m["k"] != "2" {
			t.Fatalf("m = %v", m)
		}
	})

	for _, bad := range []string{"noequals", "=novalue"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			if _, err := parseLabels([]string{bad}); err == nil {
				t.Fatalf("expected error for %q", bad)
			}
		})
	}
}

func TestFormatLabels(t *testing.T) {
	if got := formatLabels(nil); got != "" {
		t.Fatalf("empty = %q", got)
	}
	// Stable, sorted output regardless of map iteration order.
	got := formatLabels(map[string]string{"z": "1", "a": "2"})
	if got != "a=2,z=1" {
		t.Fatalf("got %q", got)
	}
}
