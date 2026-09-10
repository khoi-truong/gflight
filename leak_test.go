package gflight_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/khoi-truong/gflight"
)

// TestNoGoroutineLeak asserts that the fan-out, retry, and rate-limit
// machinery leaves nothing running once a search returns — including the
// cancelled and failing paths, where a bug would strand a phase-2 goroutine
// blocked on a semaphore.
//
// It deliberately does not call t.Parallel: the goroutine count is
// process-global, so it only reads cleanly while no other test is running.
func TestNoGoroutineLeak(t *testing.T) {
	body := fixtureBytes(t, "shopping_results_oneway_jfk_lax.txt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	req := sampleRequest()
	req.ReturnDate = req.DepartDate.AddDate(0, 0, 7)

	newClient := func() *gflight.Client {
		return gflight.New(
			gflight.WithBaseURL(srv.URL),
			gflight.WithHTTPClient(srv.Client()),
			gflight.WithRetry(gflight.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}),
			gflight.WithRateLimit(1000, 10),
			gflight.WithMaxConcurrency(4),
		)
	}

	// Warm up first: the first exchange starts the connection-pool goroutines
	// that every later request reuses, and those are not a leak.
	warm := newClient()
	if _, err := warm.RoundTripTopN(context.Background(), req, 3); err != nil {
		t.Fatalf("warm-up RoundTripTopN: %v", err)
	}
	srv.Client().CloseIdleConnections()
	baseline := stableGoroutines(t)

	for range 5 {
		c := newClient()
		if _, err := c.RoundTripTopN(context.Background(), req, 3); err != nil {
			t.Fatalf("RoundTripTopN: %v", err)
		}

		// A context cancelled mid-flight must not strand the fan-out either.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := c.RoundTripTopN(ctx, req, 3); err == nil {
			t.Fatal("RoundTripTopN with a cancelled context: want error, got nil")
		}
	}
	srv.Client().CloseIdleConnections()

	if got := stableGoroutines(t); got > baseline {
		buf := make([]byte, 1<<16)
		buf = buf[:runtime.Stack(buf, true)]
		t.Fatalf("goroutines = %d, baseline %d; leaked:\n%s", got, baseline, buf)
	}
}

// stableGoroutines waits for the goroutine count to settle and returns it.
// Connection teardown is asynchronous, so a single reading right after a
// request is noise.
func stableGoroutines(t *testing.T) int {
	t.Helper()
	var last int
	for i := range 100 {
		runtime.GC()
		n := runtime.NumGoroutine()
		if i > 0 && n <= last {
			return n
		}
		last = n
		time.Sleep(10 * time.Millisecond)
	}
	return last
}
