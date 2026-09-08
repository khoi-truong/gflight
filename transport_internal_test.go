package gflight

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newResp(code int, hdr http.Header) *http.Response {
	if hdr == nil {
		hdr = http.Header{}
	}
	return &http.Response{
		StatusCode: code,
		Header:     hdr,
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

func reqWithBody(t *testing.T) *http.Request {
	t.Helper()
	r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://x/rpc", strings.NewReader("f.req=abc"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRetryTransportRetriesThenSucceeds(t *testing.T) {
	t.Parallel()
	var calls int
	rt := &retryTransport{
		policy: RetryPolicy{MaxAttempts: 4}.withDefaults(),
		next: rtFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			b, _ := io.ReadAll(r.Body)
			if string(b) != "f.req=abc" {
				t.Errorf("attempt %d body = %q, want replayed body", calls, b)
			}
			if calls < 3 {
				return newResp(http.StatusTooManyRequests, nil), nil
			}
			return newResp(http.StatusOK, nil), nil
		}),
	}

	synctest.Test(t, func(t *testing.T) {
		resp, err := rt.RoundTrip(reqWithBody(t))
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if calls != 3 {
			t.Fatalf("calls = %d, want 3", calls)
		}
	})
}

func TestRetryTransportExhaustsAttempts(t *testing.T) {
	t.Parallel()
	var calls int
	rt := &retryTransport{
		policy: RetryPolicy{MaxAttempts: 3}.withDefaults(),
		next: rtFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return newResp(http.StatusServiceUnavailable, nil), nil
		}),
	}
	synctest.Test(t, func(t *testing.T) {
		resp, err := rt.RoundTrip(reqWithBody(t))
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", resp.StatusCode)
		}
		if calls != 3 {
			t.Fatalf("calls = %d, want 3 (MaxAttempts)", calls)
		}
	})
}

func TestRetryTransportHonoursRetryAfter(t *testing.T) {
	t.Parallel()
	var calls int
	rt := &retryTransport{
		policy: RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Hour}.withDefaults(),
		next: rtFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return newResp(http.StatusTooManyRequests, http.Header{"Retry-After": {"5"}}), nil
			}
			return newResp(http.StatusOK, nil), nil
		}),
	}
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if _, err := rt.RoundTrip(reqWithBody(t)); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		if waited := time.Since(start); waited != 5*time.Second {
			t.Fatalf("waited %s, want exactly 5s from Retry-After", waited)
		}
	})
}

func TestRetryTransportStopsOnContextCancel(t *testing.T) {
	t.Parallel()
	rt := &retryTransport{
		policy: RetryPolicy{MaxAttempts: 5, BaseDelay: time.Hour, MaxDelay: time.Hour}.withDefaults(),
		next: rtFunc(func(r *http.Request) (*http.Response, error) {
			return newResp(http.StatusServiceUnavailable, http.Header{"Retry-After": {"3600"}}), nil
		}),
	}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		r := reqWithBody(t).WithContext(ctx)
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()
		_, err := rt.RoundTrip(r)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})
}

func TestRetryTransportDoesNotRetryNonRetryable(t *testing.T) {
	t.Parallel()
	var calls int
	rt := &retryTransport{
		policy: RetryPolicy{MaxAttempts: 4}.withDefaults(),
		next: rtFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return newResp(http.StatusBadRequest, nil), nil
		}),
	}
	if _, err := rt.RoundTrip(reqWithBody(t)); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (400 is not retryable)", calls)
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"", 0, false},
		{"0", 0, true},
		{"120", 120 * time.Second, true},
		{"-5", 0, false},
		{"garbage", 0, false},
		{now.Add(30 * time.Second).UTC().Format(http.TimeFormat), 30 * time.Second, true},
		{now.Add(-time.Minute).UTC().Format(http.TimeFormat), 0, true},
	}
	for _, tt := range tests {
		got, ok := parseRetryAfter(tt.in, now)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseRetryAfter(%q) = (%s, %t), want (%s, %t)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
