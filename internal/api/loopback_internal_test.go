package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/brent-hoover/sutra/internal/config"
)

// TestRunStartupGuard verifies Run refuses a tokenless non-loopback bind but
// allows tokenless loopback and token-authenticated non-loopback binds.
func TestRunStartupGuard(t *testing.T) {
	cfgFor := func(addr, token string) config.Config {
		d := t.TempDir()
		return config.Config{
			ListenAddr:  addr,
			Token:       token,
			DBPath:      filepath.Join(d, "t.db"),
			ProjectsDir: filepath.Join(d, "projects"),
		}
	}

	// Tokenless non-loopback: rejected before serving (background ctx, never binds).
	if err := Run(context.Background(), cfgFor("0.0.0.0:0", "")); err == nil {
		t.Error("tokenless 0.0.0.0: expected startup error, got nil")
	}
	if err := Run(context.Background(), cfgFor(":0", "")); err == nil {
		t.Error("tokenless wildcard :0: expected startup error, got nil")
	}

	// Allowed configs: an already-cancelled ctx makes Run return promptly with
	// no guard error.
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	if err := Run(cancelled(), cfgFor("127.0.0.1:0", "")); err != nil {
		t.Errorf("tokenless loopback: unexpected error: %v", err)
	}
	if err := Run(cancelled(), cfgFor("0.0.0.0:0", "tok")); err != nil {
		t.Errorf("token-authenticated non-loopback: unexpected error: %v", err)
	}
}

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
		{"LOCALHOST:8422", true},
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
