package gflight

import (
	"net/http"
	"time"
)

// Observer receives operational events so consumers can wire metrics or
// tracing without this library importing Prometheus, OpenTelemetry, or
// anything else. Every field is optional; a nil func is skipped.
//
// Callbacks run on the goroutine that produced the event — including the
// concurrent phase-2 goroutines of [Client.RoundTripTopN] — so an
// implementation must be safe for concurrent use and must not block. Do the
// work of a counter increment, not of an HTTP call.
//
// The struct is additive: new event hooks arrive as new fields, so an
// Observer built with keyed fields keeps compiling.
type Observer struct {
	// OnRetry fires once per retry decision, just before the backoff wait,
	// when [WithRetry] is enabled.
	OnRetry func(RetryEvent)
	// OnResponse fires once per network attempt — a retried request produces
	// one event per try.
	OnResponse func(ResponseEvent)
	// OnParse fires once per decoded response payload. Its Failures count is
	// the upstream-drift canary: it rises before decoding breaks outright.
	OnParse func(ParseEvent)
}

// RetryEvent describes one retry decision made by the transport [WithRetry]
// installs.
type RetryEvent struct {
	// Attempt is the 1-based number of the attempt that just failed.
	Attempt int
	// Wait is the backoff before the next attempt, either the parsed
	// Retry-After or the full-jitter delay.
	Wait time.Duration
	// StatusCode is the retryable status that triggered the retry, or 0 when
	// the attempt failed before a response arrived.
	StatusCode int
	// RetryAfter reports whether Wait came from a Retry-After header.
	RetryAfter bool
	// Err is the transport error that triggered the retry, or nil when the
	// attempt produced a retryable response instead.
	Err error
}

// ResponseEvent describes one completed network attempt.
type ResponseEvent struct {
	// Method and URL identify the request. URL carries the query string, which
	// holds only locale parameters — never credentials.
	Method string
	URL    string
	// StatusCode is the response status, or 0 when the attempt failed before
	// a response arrived.
	StatusCode int
	// Duration is the round-trip latency, excluding any rate-limiter wait.
	Duration time.Duration
	// Err is the transport error, or nil on a completed exchange — including
	// one that returned a non-2xx status.
	Err error
}

// ParseEvent describes the decode of one response payload.
type ParseEvent struct {
	// Rows is the number of candidate itinerary rows the payload held.
	Rows int
	// Failures is how many of those rows failed to decode. A non-zero value
	// against a healthy Rows count means the wire shape drifted.
	Failures int
}

func (o *Observer) retry(e RetryEvent) {
	if o != nil && o.OnRetry != nil {
		o.OnRetry(e)
	}
}

func (o *Observer) response(e ResponseEvent) {
	if o != nil && o.OnResponse != nil {
		o.OnResponse(e)
	}
}

func (o *Observer) parse(e ParseEvent) {
	if o != nil && o.OnParse != nil {
		o.OnParse(e)
	}
}

// observeTransport reports every network attempt to an [Observer]. It sits at
// the bottom of the stack, directly above the base transport, so it sees each
// retried attempt individually and its Duration excludes rate-limiter queueing.
type observeTransport struct {
	next     http.RoundTripper
	observer *Observer
}

func (t *observeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	t.observer.response(ResponseEvent{
		Method:     req.Method,
		URL:        req.URL.String(),
		StatusCode: statusOf(resp),
		Duration:   time.Since(start),
		Err:        err,
	})
	return resp, err
}
