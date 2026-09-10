package gflight_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/khoi-truong/gflight"
	"github.com/khoi-truong/gflight/internal/wire"
)

// recorder collects observer events under a mutex: callbacks run on whatever
// goroutine produced the event, including concurrent phase-2 goroutines.
type recorder struct {
	mu        sync.Mutex
	retries   []gflight.RetryEvent
	responses []gflight.ResponseEvent
	parses    []gflight.ParseEvent
}

func (r *recorder) observer() *gflight.Observer {
	return &gflight.Observer{
		OnRetry: func(e gflight.RetryEvent) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.retries = append(r.retries, e)
		},
		OnResponse: func(e gflight.ResponseEvent) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.responses = append(r.responses, e)
		},
		OnParse: func(e gflight.ParseEvent) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.parses = append(r.parses, e)
		},
	}
}

func (r *recorder) snapshot() ([]gflight.RetryEvent, []gflight.ResponseEvent, []gflight.ParseEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]gflight.RetryEvent(nil), r.retries...),
		append([]gflight.ResponseEvent(nil), r.responses...),
		append([]gflight.ParseEvent(nil), r.parses...)
}

func TestObserverReportsResponseAndParse(t *testing.T) {
	t.Parallel()
	body := fixtureBytes(t, "shopping_results_oneway_jfk_lax.txt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	var rec recorder
	c := gflight.New(
		gflight.WithBaseURL(srv.URL),
		gflight.WithHTTPClient(srv.Client()),
		gflight.WithObserver(rec.observer()),
	)
	its, err := c.Search(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	retries, responses, parses := rec.snapshot()
	if len(retries) != 0 {
		t.Errorf("retry events = %d, want 0", len(retries))
	}
	if len(responses) != 1 {
		t.Fatalf("response events = %d, want 1", len(responses))
	}
	got := responses[0]
	if got.StatusCode != http.StatusOK || got.Err != nil || got.Method != http.MethodPost {
		t.Errorf("response event = %+v, want a 200 POST with no error", got)
	}
	if got.Duration <= 0 {
		t.Errorf("response Duration = %v, want > 0", got.Duration)
	}
	if len(parses) == 0 {
		t.Fatal("no parse events")
	}
	var rows, failures int
	for _, p := range parses {
		rows += p.Rows
		failures += p.Failures
	}
	if failures != 0 {
		t.Errorf("parse failures = %d, want 0", failures)
	}
	if rows != len(its) {
		t.Errorf("parse rows = %d, want %d decoded itineraries", rows, len(its))
	}
}

func TestObserverReportsRetryPerAttempt(t *testing.T) {
	t.Parallel()
	body := fixtureBytes(t, "shopping_results_oneway_jfk_lax.txt")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	var rec recorder
	c := gflight.New(
		gflight.WithBaseURL(srv.URL),
		gflight.WithHTTPClient(srv.Client()),
		gflight.WithRetry(gflight.RetryPolicy{MaxAttempts: 4, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}),
		gflight.WithObserver(rec.observer()),
	)
	if _, err := c.Search(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Search: %v", err)
	}

	retries, responses, _ := rec.snapshot()
	if len(retries) != 2 {
		t.Fatalf("retry events = %d, want 2", len(retries))
	}
	for i, e := range retries {
		if e.Attempt != i+1 {
			t.Errorf("retry[%d].Attempt = %d, want %d", i, e.Attempt, i+1)
		}
		if e.StatusCode != http.StatusTooManyRequests {
			t.Errorf("retry[%d].StatusCode = %d, want 429", i, e.StatusCode)
		}
		if !e.RetryAfter {
			t.Errorf("retry[%d].RetryAfter = false, want true (server sent the header)", i)
		}
		if e.Err != nil {
			t.Errorf("retry[%d].Err = %v, want nil", i, e.Err)
		}
	}
	// One response event per network attempt, retries included.
	if len(responses) != 3 {
		t.Fatalf("response events = %d, want 3", len(responses))
	}
}

func TestObserverReportsTransportError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening: the round trip fails before a response.

	var rec recorder
	c := gflight.New(
		gflight.WithBaseURL(url),
		gflight.WithObserver(rec.observer()),
	)
	if _, err := c.Search(context.Background(), sampleRequest()); err == nil {
		t.Fatal("Search against a closed server: want error, got nil")
	}

	_, responses, parses := rec.snapshot()
	if len(responses) != 1 {
		t.Fatalf("response events = %d, want 1", len(responses))
	}
	if responses[0].Err == nil || responses[0].StatusCode != 0 {
		t.Errorf("response event = %+v, want a zero status with an error", responses[0])
	}
	if len(parses) != 0 {
		t.Errorf("parse events = %d, want 0", len(parses))
	}
}

// TestObserverCountsParseFailures feeds a payload whose rows are half garbage:
// the failure count is the drift canary, so it must survive a partial decode.
func TestObserverCountsParseFailures(t *testing.T) {
	t.Parallel()
	body := corruptOneRow(t, fixtureBytes(t, "shopping_results_oneway_jfk_lax.txt"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	var rec recorder
	c := gflight.New(
		gflight.WithBaseURL(srv.URL),
		gflight.WithHTTPClient(srv.Client()),
		gflight.WithObserver(rec.observer()),
	)
	if _, err := c.Search(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Search: %v", err)
	}

	_, _, parses := rec.snapshot()
	var failures int
	for _, p := range parses {
		failures += p.Failures
	}
	if failures == 0 {
		t.Error("parse failures = 0, want the corrupted row counted")
	}
}

func TestWithObserverNilIsIgnored(t *testing.T) {
	t.Parallel()
	body := fixtureBytes(t, "shopping_results_oneway_jfk_lax.txt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := gflight.New(
		gflight.WithBaseURL(srv.URL),
		gflight.WithHTTPClient(srv.Client()),
		gflight.WithObserver(nil),
		// An observer with every callback nil must be equally harmless.
		gflight.WithObserver(&gflight.Observer{}),
	)
	if _, err := c.Search(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Search: %v", err)
	}
}

// corruptOneRow rewrites a recorded response so exactly one itinerary row is
// unparseable garbage, leaving the rest decodable. Re-emitted as a bare frame,
// which the wire reader accepts without a length prefix.
func corruptOneRow(t *testing.T, body []byte) []byte {
	t.Helper()
	payloads, err := wire.Payloads(body)
	if err != nil || len(payloads) == 0 {
		t.Fatalf("wire.Payloads: %v (%d payloads)", err, len(payloads))
	}

	var inner []any
	if err := json.Unmarshal(payloads[0], &inner); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if !replaceFirstRow(inner, "garbage") {
		t.Fatal("fixture has no itinerary rows to corrupt")
	}

	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	frame, err := json.Marshal([]any{[]any{"wrb.fr", nil, string(innerJSON)}})
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return append([]byte(")]}'\n\n"), frame...)
}

// replaceFirstRow swaps inner[2|3][0][0] — the first itinerary row — for v,
// reporting whether it found one.
func replaceFirstRow(inner []any, v any) bool {
	for _, i := range []int{2, 3} {
		if i >= len(inner) {
			continue
		}
		block, ok := inner[i].([]any)
		if !ok || len(block) == 0 {
			continue
		}
		rows, ok := block[0].([]any)
		if !ok || len(rows) < 2 {
			continue
		}
		rows[0] = v
		return true
	}
	return false
}
