package client

import (
	"context"
	"io"
	"net/http"
	"time"
)

type Response struct {
	StatusCode int
	Body       []byte
}

type Interface interface {
	Get(ctx context.Context, path string) (*Response, error)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
	GetFunc    func(ctx context.Context, path string) (*Response, error)
}

type Options struct {
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
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

func (c *Client) Get(ctx context.Context, path string) (*Response, error) {
	if c.GetFunc != nil {
		return c.GetFunc(ctx, path)
	}

	var fullURL string
	if c.baseURL == "" {
		fullURL = path // If no base URL, treat path as full URL
	} else {
		fullURL = c.baseURL + path // Otherwise combine them
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       body,
	}, nil
}
