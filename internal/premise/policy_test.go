package premise

import (
	"net"
	"testing"
)

func TestAllowedAddr(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"1.1.1.1", true},
		{"8.8.8.8", true},
		{"172.15.255.255", true},
		{"100.63.255.255", true},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"172.31.255.1", false},
		{"192.168.1.1", false},
		{"100.64.0.1", false},
		{"100.127.255.1", false},
		{"127.0.0.1", false},
		{"169.254.1.1", false},
		{"0.0.0.0", false},
		{"0.1.2.3", false},
		{"255.255.255.255", false},
		{"224.0.0.1", false},
		{"::1", false},
		{"fe80::1", false},
		{"fc00::1", false},
		{"fd12::1", false},
		{"2001:4860:4860::8888", true},
		{"::ffff:1.1.1.1", true},
		{"::ffff:10.0.0.1", false},
		{"::ffff:127.0.0.1", false},
	}
	for _, test := range cases {
		ip := net.ParseIP(test.ip)
		if ip == nil {
			t.Fatalf("parse %s", test.ip)
		}
		if got := allowedAddr(ip); got != test.want {
			t.Errorf("%s allowed = %v, want %v", test.ip, got, test.want)
		}
	}
}

func TestParseProbeURL(t *testing.T) {
	ok := []string{
		"https://example.com/platform/postgres.json",
		"https://example.com/a?b=1",
		"https://example.com:443/a",
	}
	for _, raw := range ok {
		if _, err := parseProbeURL(raw); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
	bad := []string{
		"http://example.com/a",
		"https://1.1.1.1/a",
		"https://[::1]/a",
		"https://user:pass@example.com/a",
		"https://example.com/a#frag",
		"https://example.com#",
		"https://example.com:8443/a",
	}
	for _, raw := range bad {
		if _, err := parseProbeURL(raw); err == nil {
			t.Errorf("%s was accepted", raw)
		}
	}
}
