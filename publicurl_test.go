package main

import "testing"

func TestFormatPublicURL(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		scheme string
		port   int
		want   string
	}{
		{"https default port", "app.localhost", "https", 443, "https://app.localhost"},
		{"https zero port", "app.localhost", "https", 0, "https://app.localhost"},
		{"https custom port", "app.localhost", "https", 8443, "https://app.localhost:8443"},
		{"http default port", "app.localhost", "http", 80, "http://app.localhost"},
		{"http zero port", "app.localhost", "http", 0, "http://app.localhost"},
		{"http custom port", "app.localhost", "http", 9000, "http://app.localhost:9000"},
		{"https non-localhost", "api.example.com", "https", 443, "https://api.example.com"},
		{"https non-localhost custom port", "api.example.com", "https", 8443, "https://api.example.com:8443"},
	}
	for _, tc := range cases {
		got := formatPublicURL(tc.host, tc.scheme, tc.port)
		if got != tc.want {
			t.Errorf("%s: formatPublicURL(%q, %q, %d) = %q, want %q", tc.name, tc.host, tc.scheme, tc.port, got, tc.want)
		}
	}
}
