package api

import "testing"

func TestIsLoopbackAddr(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8422", true},
		{"127.0.0.1", true},
		{"[::1]:8422", true},
		{"localhost:8422", true},
		{"localhost", true},
		{"0.0.0.0:8422", false},
		{":8422", false}, // empty host = all interfaces
		{"192.168.1.5:8422", false},
		{"10.0.0.1:8422", false},
		{"[2001:db8::1]:8422", false},
	}
	for _, tc := range cases {
		if got := isLoopbackAddr(tc.addr); got != tc.want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}
