package gflight

import (
	"errors"
	"fmt"
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
