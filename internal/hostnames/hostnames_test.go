package hostnames

import (
	"reflect"
	"testing"
)

func TestExpandExpandsBareNames(t *testing.T) {
	got := Expand([]string{"api", "admin.other.test"}, "example.com.")
	want := []string{"api.example.com", "admin.other.test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandLeavesBareNameUnchangedWithoutDomain(t *testing.T) {
	got := Expand([]string{"api"}, "")
	want := []string{"api"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandLeavesLocalhostAndLiteralHostsUnchanged(t *testing.T) {
	got := Expand([]string{"localhost", "127.0.0.1", "[::1]"}, "example.com")
	want := []string{"localhost", "127.0.0.1", "[::1]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandReturnsEmptySliceForEmptyInput(t *testing.T) {
	got := Expand([]string{}, "example.com")
	if len(got) != 0 {
		t.Fatalf("got %v, want empty slice", got)
	}
}

func TestExpandLeavesAllFullHostsUnchanged(t *testing.T) {
	got := Expand([]string{"full.test", "admin.other.test"}, "example.com")
	want := []string{"full.test", "admin.other.test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
