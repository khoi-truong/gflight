package gflight

import (
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/time/rate"
)

// DefaultCurrency is the ISO 4217 code sent when neither [WithCurrency] nor
// [SearchRequest.Currency] is set.
const DefaultCurrency = "USD"

// DefaultLanguage and DefaultCountry are the locale hints Google uses to pick
// result language and market when no override is given.
const (
	DefaultLanguage = "en"
	DefaultCountry  = "US"
)

// DefaultMaxConcurrency bounds the phase-2 requests a round-trip search fans
// out when [SearchRequest.ReturnDate] is set. Override with [WithMaxConcurrency].
const DefaultMaxConcurrency = 3

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

// WithCurrency sets the default ISO 4217 currency for price results. The code
// is upper-cased before use — Google silently ignores lower-case codes. An
// empty string is ignored. [SearchRequest.Currency] overrides this per call.
func WithCurrency(code string) Option {
	return func(c *Client) {
		if code != "" {
			c.currency = strings.ToUpper(code)
		}
	}
}

// WithLanguage sets the BCP-47 language hint (the `hl` parameter), e.g. "en" or
// "en-GB". An empty string is ignored.
func WithLanguage(lang string) Option {
	return func(c *Client) {
		if lang != "" {
			c.language = lang
		}
	}
}

// WithCountry sets the ISO 3166-1 alpha-2 market hint (the `gl` parameter),
// e.g. "US" or "VN". The code is upper-cased. An empty string is ignored.
func WithCountry(code string) Option {
	return func(c *Client) {
		if code != "" {
			c.country = strings.ToUpper(code)
		}
	}
}

// WithMaxConcurrency caps the number of in-flight phase-2 requests a round-trip
// search issues (one per selected outbound). Values below 1 are ignored, so the
// client always makes progress. Defaults to [DefaultMaxConcurrency].
func WithMaxConcurrency(n int) Option {
	return func(c *Client) {
		if n >= 1 {
			c.maxConcurrency = n
		}
	}
}

// WithTransport sets the base [http.RoundTripper] for upstream requests — the
// single seam for plugging in a browser-grade TLS stack (e.g. uTLS) or a proxy
// without replacing the whole [http.Client]. It composes under [WithRetry]:
// retry wraps the transport given here. A nil transport is ignored.
//
// [WithHTTPClient] wins if both are set and that client already has a
// non-nil Transport.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *Client) {
		if rt != nil {
			c.baseTransport = rt
		}
	}
}

// WithRetry installs a retrying [http.RoundTripper] in front of the base
// transport. It retries connection errors and 408/425/429/500/502/503/504
// with full-jitter exponential backoff, honouring a Retry-After header when the
// upstream sends one. A policy with MaxAttempts below 2 is ignored (retrying
// stays off, the default).
func WithRetry(p RetryPolicy) Option {
	return func(c *Client) {
		if p.MaxAttempts >= 2 {
			policy := p.withDefaults()
			c.retry = &policy
		}
	}
}

// WithRateLimit paces every upstream request through a token bucket of rps
// requests per second with the given burst — retries included, since the
// limiter sits below the retrying transport. Wait time counts against the
// caller's context: a request that expires while queued — or that could not be
// sent before its deadline — fails with the matching context error and never
// reaches the network.
//
// Google has no published quota; comparable clients settle around 10 req/s.
// Rate limiting is off by default. An rps of zero or less is ignored; a burst
// below 1 is raised to 1 so the client always makes progress.
func WithRateLimit(rps float64, burst int) Option {
	return func(c *Client) {
		if rps <= 0 {
			return
		}
		if burst < 1 {
			burst = 1
		}
		c.limiter = rate.NewLimiter(rate.Limit(rps), burst)
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

// WithObserver routes retry, response, and parse events to o so a consumer can
// export metrics or traces without this library depending on any telemetry
// package. A nil observer is ignored; so is a nil callback within it.
//
// Callbacks run inline on the goroutine that produced the event and must not
// block — see [Observer].
func WithObserver(o *Observer) Option {
	return func(c *Client) {
		if o != nil {
			c.observer = o
		}
	}
}
