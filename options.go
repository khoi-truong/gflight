package gflight

import (
	"log/slog"
	"net/http"
	"strings"
)

// Option customises a [Client]. Options are applied in order by [New].
type Option func(*Client)

// WithHTTPClient sets the [http.Client] used for upstream requests. Passing an
// httptest.Server's client is how tests keep the network out of `go test`.
// A nil client is ignored.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithUserAgent sets the User-Agent header sent upstream. An empty string is
// ignored.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithBaseURL overrides the upstream origin — chiefly to point the client at a
// test server. Trailing slashes are trimmed; an empty string is ignored.
func WithBaseURL(rawURL string) Option {
	return func(c *Client) {
		if rawURL != "" {
			c.baseURL = strings.TrimRight(rawURL, "/")
		}
	}
}

// WithLogger attaches a logger. Library code logs nothing by default; without
// this option the client discards every record.  A nil logger is ignored.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.logger = l
		}
	}
}
