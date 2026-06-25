package core_refactor

import (
	"testing"
)

func TestParseProxyURL(t *testing.T) {
	cases := []struct {
		input    string
		wantHost string
		wantUser string
		wantPass string
	}{
		{"", "", "", ""},
		{"http://proxy.example.com:8080", "proxy.example.com:8080", "", ""},
		{"http://user:pass@proxy.example.com:8080", "proxy.example.com:8080", "user", "pass"},
	}

	for _, tc := range cases {
		p, err := ParseProxyURL(tc.input)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.input, err)
		}
		if p.Host != tc.wantHost {
			t.Fatalf("host = %q, want %q", p.Host, tc.wantHost)
		}
		if p.Username != tc.wantUser {
			t.Fatalf("username = %q, want %q", p.Username, tc.wantUser)
		}
		if p.Password != tc.wantPass {
			t.Fatalf("password = %q, want %q", p.Password, tc.wantPass)
		}
	}
}

func TestProxyBasicAuth(t *testing.T) {
	p := Proxy{Username: "user", Password: "pass"}
	auth := p.BasicAuth()
	if auth != "Basic dXNlcjpwYXNz" {
		t.Fatalf("auth = %q", auth)
	}
}

func TestDirectProxy(t *testing.T) {
	p := DirectProxy(nil)
	if !p.IsDirect() {
		t.Fatal("expected direct proxy")
	}
}
