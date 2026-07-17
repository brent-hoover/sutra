package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/service"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc, err := service.New(config.Config{DBPath: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	srv := httptest.NewServer(api.Handler(svc))
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateIssueBodyDecoding(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid", `{"subject":"a","body":"b"}`, http.StatusCreated},
		{"trailing whitespace ok", "{\"subject\":\"a\",\"body\":\"b\"}\n  \t", http.StatusCreated},
		{"concatenated json rejected", `{"subject":"a","body":"b"}{"subject":"c","body":"d"}`, http.StatusBadRequest},
		{"malformed trailing rejected", `{"subject":"a","body":"b"} garbage`, http.StatusBadRequest},
		{"invalid json rejected", `{`, http.StatusBadRequest},
		{"missing body rejected", `{"subject":"a"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/issues", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
