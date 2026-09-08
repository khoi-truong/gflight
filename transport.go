package gflight

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy configures the retrying [http.RoundTripper] that [WithRetry]
// installs. The zero value disables retrying; pass it to [WithRetry] with
// MaxAttempts >= 2 to turn it on.
//
// Backoff is full jitter: the wait before attempt n is a uniform random value
// in [0, min(MaxDelay, BaseDelay*2^(n-1))). A Retry-After header on the
// response overrides that computed wait.
type RetryPolicy struct {
	// MaxAttempts is the total number of tries, including the first. Values
	// below 2 disable retrying.
	MaxAttempts int
	// BaseDelay is the backoff before the first retry. Defaults to 500ms.
	BaseDelay time.Duration
	// MaxDelay caps any single backoff wait. Defaults to 30s.
	MaxDelay time.Duration
}

func (p RetryPolicy) withDefaults() RetryPolicy {
	if p.BaseDelay <= 0 {
		p.BaseDelay = 500 * time.Millisecond
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = 30 * time.Second
	}
	return p
}

// retryableStatus reports whether an HTTP status warrants another attempt.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, // 408
		http.StatusTooEarly,            // 425
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}

// retryTransport retries idempotent-by-construction POSTs to the RPC endpoint.
// Every request this library issues carries a fully-buffered body (a short
// form-encoded string) and is safe to replay.
type retryTransport struct {
	next   http.RoundTripper
	policy RetryPolicy
	logger *slog.Logger
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	var (
		resp    *http.Response
		err     error
		attempt int
	)
	for attempt = 1; ; attempt++ {
		if attempt > 1 {
			body, rewErr := rewindBody(req)
			if rewErr != nil {
				return resp, err // cannot replay; surface the prior outcome
			}
			req.Body = body
		}

		resp, err = t.next.RoundTrip(req)

		done := attempt >= t.policy.MaxAttempts
		retryable := err != nil || (resp != nil && retryableStatus(resp.StatusCode))
		if !retryable || done {
			return resp, err
		}

		wait := t.backoff(attempt, resp)
		if t.logger != nil {
			t.logger.Debug("gflight: retrying upstream request",
				"attempt", attempt, "next_in", wait, "err", err, "status", statusOf(resp))
		}
		if resp != nil {
			drain(resp)
		}
		if sleepErr := sleep(ctx, wait); sleepErr != nil {
			return nil, sleepErr
		}
	}
}

// backoff picks the wait before the next attempt: the Retry-After header when
// the server sent one, otherwise full-jitter exponential backoff.
func (t *retryTransport) backoff(attempt int, resp *http.Response) time.Duration {
	if resp != nil {
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			if d > t.policy.MaxDelay {
				return t.policy.MaxDelay
			}
			return d
		}
	}
	ceil := float64(t.policy.BaseDelay) * math.Ldexp(1, attempt-1)
	if capped := float64(t.policy.MaxDelay); ceil > capped {
		ceil = capped
	}
	if ceil <= 0 {
		return 0
	}
	return time.Duration(rand.Float64() * ceil)
}

// rewindBody returns a fresh body ReadCloser for a replayed request.
func rewindBody(req *http.Request) (io.ReadCloser, error) {
	if req.Body == nil {
		return nil, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("gflight: request body cannot be replayed")
	}
	return req.GetBody()
}

func drain(resp *http.Response) {
	if resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}

func statusOf(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

// sleep waits for d or until ctx is done, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// parseRetryAfter reads a Retry-After header value, which is either
// delta-seconds or an HTTP-date. It returns a non-negative duration and true
// on success.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if when, err := http.ParseTime(v); err == nil {
		d := when.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}
