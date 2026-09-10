package gflight_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/khoi-truong/gflight"
)

func TestWithRetryRecoversFrom429(t *testing.T) {
	t.Parallel()
	body := fixtureBytes(t, "shopping_results_oneway_jfk_lax.txt")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := gflight.New(
		gflight.WithBaseURL(srv.URL),
		gflight.WithHTTPClient(srv.Client()),
		gflight.WithRetry(gflight.RetryPolicy{MaxAttempts: 4, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}),
	)
	its, err := c.Search(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(its) == 0 {
		t.Fatal("no itineraries")
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("server hits = %d, want 3", got)
	}
}

func TestWithRetryGivesUpAsBlockedError(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	c := gflight.New(
		gflight.WithBaseURL(srv.URL),
		gflight.WithHTTPClient(srv.Client()),
		gflight.WithRetry(gflight.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}),
	)
	_, err := c.Search(context.Background(), sampleRequest())
	if !errors.Is(err, gflight.ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
	var be *gflight.BlockedError
	if !errors.As(err, &be) {
		t.Fatalf("err = %v, want *BlockedError", err)
	}
	if be.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want 429", be.StatusCode)
	}
	if be.RetryAfter != 7*time.Second {
		t.Errorf("RetryAfter = %s, want 7s", be.RetryAfter)
	}
	if be.DeepLink == "" {
		t.Error("DeepLink is empty, want a fallback browser URL")
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("server hits = %d, want 3 (MaxAttempts)", got)
	}
}

func TestBlockedErrorFromNonEnvelopeBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!doctype html><title>Before you continue</title>"))
	}))
	t.Cleanup(srv.Close)

	c := gflight.New(gflight.WithBaseURL(srv.URL), gflight.WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), sampleRequest())
	var be *gflight.BlockedError
	if !errors.As(err, &be) {
		t.Fatalf("err = %v, want *BlockedError", err)
	}
	if be.DeepLink == "" {
		t.Error("DeepLink is empty, want a fallback browser URL")
	}
}

func TestWithTransportIsUsed(t *testing.T) {
	t.Parallel()
	var seen atomic.Bool
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen.Store(true)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Header:     http.Header{},
		}, nil
	})
	c := gflight.New(
		gflight.WithBaseURL("http://flights.test"),
		gflight.WithTransport(base),
	)
	_, _ = c.Search(context.Background(), sampleRequest())
	if !seen.Load() {
		t.Fatal("custom transport was not invoked")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWithRateLimitSpacesRequests(t *testing.T) {
	t.Parallel()
	body := fixtureBytes(t, "shopping_results_oneway_jfk_lax.txt")
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	const interval = 20 * time.Millisecond
	c := gflight.New(
		gflight.WithBaseURL(srv.URL),
		gflight.WithHTTPClient(srv.Client()),
		gflight.WithRateLimit(float64(time.Second/interval), 1),
	)
	for range 3 {
		if _, err := c.Search(context.Background(), sampleRequest()); err != nil {
			t.Fatalf("Search: %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(times) != 3 {
		t.Fatalf("server hits = %d, want 3", len(times))
	}
	// The burst covers the first request; each later one waits out the interval.
	// Exact pacing is asserted under synctest in transport_internal_test.go —
	// here a wall clock only has to show the limiter is in the stack at all.
	if gap := times[2].Sub(times[0]); gap < interval {
		t.Fatalf("first-to-last gap = %v, want >= %v", gap, interval)
	}
}

func TestWithRateLimitIgnoresNonPositiveRate(t *testing.T) {
	t.Parallel()
	hc := &http.Client{Transport: http.DefaultTransport}
	c := gflight.New(gflight.WithHTTPClient(hc), gflight.WithRateLimit(0, 5))
	if got := c.HTTPClient().Transport; got != http.DefaultTransport {
		t.Fatalf("transport = %T, want the unwrapped base transport", got)
	}
}
