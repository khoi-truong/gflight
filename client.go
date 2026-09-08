package gflight

import (
	"log/slog"
	"net/http"
	"sync"
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
	currency   string
	language   string
	country    string
	logger     *slog.Logger

	// baseTransport is the caller-supplied RoundTripper from [WithTransport];
	// nil means "use the http.Client's own transport".
	baseTransport http.RoundTripper
	// retry, when non-nil, enables the retrying RoundTripper from [WithRetry].
	retry *RetryPolicy

	// maxConcurrency bounds phase-2 round-trip fan-out. Always >= 1.
	maxConcurrency int

	mu sync.Mutex
	// lastSessionID is inner[0][4] from the most recent successful search —
	// the shopping-session id a future booking-results call needs.
	lastSessionID string
}

// New returns a Client configured with the given options.
func New(opts ...Option) *Client {
	c := &Client{
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		baseURL:        DefaultBaseURL,
		userAgent:      DefaultUserAgent,
		currency:       DefaultCurrency,
		language:       DefaultLanguage,
		country:        DefaultCountry,
		logger:         slog.New(slog.DiscardHandler),
		maxConcurrency: DefaultMaxConcurrency,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.assembleTransport()
	return c
}

// assembleTransport layers the RoundTripper stack onto a private copy of the
// http.Client, so a caller's [WithHTTPClient] value is never mutated:
//
//	retry (WithRetry) -> base (WithTransport, else the client's own transport)
func (c *Client) assembleTransport() {
	if c.baseTransport == nil && c.retry == nil {
		return
	}
	hc := *c.httpClient
	base := hc.Transport
	if c.baseTransport != nil {
		base = c.baseTransport
	}
	if base == nil {
		base = http.DefaultTransport
	}
	if c.retry != nil {
		base = &retryTransport{next: base, policy: *c.retry, logger: c.logger}
	}
	hc.Transport = base
	c.httpClient = &hc
}

// HTTPClient reports the [http.Client] used for upstream requests.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

// BaseURL reports the origin the client talks to.
func (c *Client) BaseURL() string { return c.baseURL }

// UserAgent reports the User-Agent header sent upstream.
func (c *Client) UserAgent() string { return c.userAgent }

// Currency reports the default ISO 4217 currency code.
func (c *Client) Currency() string { return c.currency }

// SessionID reports the shopping-session id captured from the most recent
// successful [Client.Search], or "" if none. It authenticates a follow-up
// booking-results call.
//
// Deprecated: use [SearchResult.SessionID] from [Client.SearchResults]. This
// accessor is per-Client mutable state and races across concurrent searches.
func (c *Client) SessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSessionID
}
