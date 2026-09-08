package gflight

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors returned by this package. Compare with [errors.Is].
var (
	// ErrNotImplemented is returned by API surface that exists to lock the
	// signature but has no implementation yet.
	ErrNotImplemented = errors.New("gflight: not implemented")

	// ErrBadResponse means the upstream reply could not be understood —
	// usually a sign that Google changed the undocumented response shape.
	ErrBadResponse = errors.New("gflight: bad response")

	// ErrNoResults means the search completed but matched no itineraries.
	ErrNoResults = errors.New("gflight: no results")

	// ErrBlocked means the upstream refused the request for reasons unrelated
	// to its contents — a 429, a bot-detection interstitial, or a consent
	// wall. Retrying immediately will not help; a different network path or a
	// browser-grade TLS client via [WithHTTPClient] usually will.
	ErrBlocked = errors.New("gflight: request blocked upstream")

	// ErrUpstreamChanged means every row in an otherwise-valid response failed
	// to decode, which almost always means Google changed the undocumented
	// wire shape. It wraps [ErrBadResponse].
	ErrUpstreamChanged = fmt.Errorf("gflight: upstream wire format changed: %w", ErrBadResponse)
)

// HTTPError reports a non-2xx reply from an upstream endpoint. It unwraps to
// [ErrBadResponse] so callers can handle every malformed-upstream case with a
// single [errors.Is] check.
type HTTPError struct {
	StatusCode int
	Status     string
	URL        string
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("gflight: %s: unexpected status %s", e.URL, e.Status)
}

// Unwrap makes errors.Is(err, ErrBadResponse) true for any HTTPError.
func (e *HTTPError) Unwrap() error { return ErrBadResponse }

// BlockedError reports that the upstream refused the request for reasons
// unrelated to its contents — a 429, a 403, a bot-detection interstitial, or a
// consent wall. It unwraps to [ErrBlocked], so existing
// errors.Is(err, ErrBlocked) checks keep working.
//
// When RetryAfter is non-zero the upstream asked callers to wait that long
// before retrying. DeepLink, when set, is the [SearchURL] for the same query —
// a blocked caller can fall back to opening it in a browser.
type BlockedError struct {
	// StatusCode is the HTTP status that triggered the block, or 0 when the
	// block was inferred from a non-envelope response body.
	StatusCode int
	// RetryAfter is the delay the upstream requested via the Retry-After
	// header, or 0 when it sent none.
	RetryAfter time.Duration
	// DeepLink is the browser URL reproducing this search, or "" when it
	// could not be built.
	DeepLink string
}

func (e *BlockedError) Error() string {
	switch {
	case e.StatusCode != 0 && e.RetryAfter > 0:
		return fmt.Sprintf("gflight: request blocked upstream (status %d, retry after %s)", e.StatusCode, e.RetryAfter)
	case e.StatusCode != 0:
		return fmt.Sprintf("gflight: request blocked upstream (status %d)", e.StatusCode)
	default:
		return "gflight: request blocked upstream"
	}
}

// Unwrap makes errors.Is(err, ErrBlocked) true for any BlockedError.
func (e *BlockedError) Unwrap() error { return ErrBlocked }
