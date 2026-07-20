package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
)

// The client must send `since` with sub-second precision so the window boundary
// is not rounded back by up to a second.
func TestActivitySendsNanosecondSince(t *testing.T) {
	var gotSince string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSince = r.URL.Query().Get("since")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"since":"2026-07-17T10:00:00Z","events":[]}`))
	}))
	defer srv.Close()

	c := client.New(config.Config{Host: srv.URL})
	since := time.Date(2026, 7, 17, 10, 0, 0, 123456789, time.UTC)
	if _, err := c.Activity(context.Background(), since); err != nil {
		t.Fatalf("Activity: %v", err)
	}
	got, err := time.Parse(time.RFC3339Nano, gotSince)
	if err != nil {
		t.Fatalf("parse since %q: %v", gotSince, err)
	}
	if !got.Equal(since) {
		t.Errorf("since = %v, want %v (sub-second precision dropped)", got, since)
	}
}
