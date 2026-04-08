package hostnames

import (
	"reflect"
	"testing"
)

func TestExpandExpandsBareNames(t *testing.T) {
	got, err := Expand([]string{"api", "admin.other.test"}, "example.com.")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := []string{"api.example.com", "admin.other.test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandLeavesBareNameUnchangedWithoutDomain(t *testing.T) {
	got, err := Expand([]string{"api"}, "")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := []string{"api"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandLeavesLocalhostAndLiteralHostsUnchanged(t *testing.T) {
	got, err := Expand([]string{"localhost", "127.0.0.1", "[::1]"}, "example.com")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := []string{"localhost", "127.0.0.1", "[::1]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandReturnsEmptySliceForEmptyInput(t *testing.T) {
	got, err := Expand([]string{}, "example.com")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty slice", got)
	}
}

func TestExpandLeavesAllFullHostsUnchanged(t *testing.T) {
	got, err := Expand([]string{"full.test", "admin.other.test"}, "example.com")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := []string{"full.test", "admin.other.test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
