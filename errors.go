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
