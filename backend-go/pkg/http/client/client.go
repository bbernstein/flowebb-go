package client

import (
	"context"
	"github.com/rs/zerolog/log"
	"net/http"
	"time"
)

type Options struct {
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

func New(opts Options) *Client {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}

	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}

	return &Client{
		baseURL: opts.BaseURL,
		httpClient: &http.Client{
			Timeout: opts.Timeout,
		},
		maxRetries: opts.MaxRetries,
	}
}

func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	var fullURL string
	if c.baseURL == "" {
		fullURL = path // If no base URL, treat path as full URL
	} else {
		fullURL = c.baseURL + path // Otherwise combine them
	}
	log.Debug().Str("url", fullURL).Msg("GET request")
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Do(req)
}
