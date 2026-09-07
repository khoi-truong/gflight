package gflight

import (
	"log/slog"
	"net/http"
	"time"
)

// DefaultBaseURL is the Google Flights origin the client talks to.
const DefaultBaseURL = "https://www.google.com/travel/flights"

// DefaultUserAgent identifies this library to the upstream endpoint.
const DefaultUserAgent = "gflight/0.1 (+https://github.com/khoi-truong/gflight)"

// Client performs Google Flights searches. The zero value is not usable —
// construct one with [New]. A Client is safe for concurrent use.
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	logger     *slog.Logger
}

// New returns a Client configured with the given options.
func New(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    DefaultBaseURL,
		userAgent:  DefaultUserAgent,
		logger:     slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HTTPClient reports the [http.Client] used for upstream requests.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

// BaseURL reports the origin the client talks to.
func (c *Client) BaseURL() string { return c.baseURL }

// UserAgent reports the User-Agent header sent upstream.
func (c *Client) UserAgent() string { return c.userAgent }
