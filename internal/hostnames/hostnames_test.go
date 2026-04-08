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

func TestExpandRejectsBareNameWithoutDomain(t *testing.T) {
	if _, err := Expand([]string{"api"}, ""); err == nil {
		t.Fatal("expected error")
	}
}
