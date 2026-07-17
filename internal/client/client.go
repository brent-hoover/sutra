package client

import (
	"net/http"
	"time"

	"github.com/brent-hoover/sutra/internal/config"
)

// Client talks to the Sutra daemon over HTTP.
type Client struct {
	baseURL string
	token   string
	hc      *http.Client
}

// New builds a Client targeting the configured host.
func New(cfg config.Config) *Client {
	return &Client{
		baseURL: cfg.Host,
		token:   cfg.Token,
		hc:      &http.Client{Timeout: 30 * time.Second},
	}
}
