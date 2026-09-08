package gflight_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khoi-truong/gflight"
)

// decodeFreqBody unwraps a captured `f.req=<...>` body into the filter array.
func decodeFreqBody(t *testing.T, body string) []any {
	t.Helper()
	body = strings.TrimPrefix(body, "f.req=")
	unescaped, err := url.QueryUnescape(body)
	if err != nil {
		t.Fatalf("unescape body: %v", err)
	}
	var outer []any
	if err := json.Unmarshal([]byte(unescaped), &outer); err != nil {
		t.Fatalf("outer unmarshal: %v", err)
	}
	inner, ok := outer[1].(string)
	if !ok {
		t.Fatalf("outer[1] not a string: %v", outer)
	}
	var filters []any
	if err := json.Unmarshal([]byte(inner), &filters); err != nil {
		t.Fatalf("inner unmarshal: %v", err)
	}
	return filters
}

// selectedFlightOf returns the outbound segment's segment[8], or nil.
func selectedFlightOf(t *testing.T, filters []any) []any {
	t.Helper()
	main, ok := filters[1].([]any)
	if !ok {
		t.Fatalf("filters[1] not a list")
	}
	segs, ok := main[13].([]any)
	if !ok || len(segs) == 0 {
		t.Fatalf("no segments in body")
	}
	sel, _ := segs[0].([]any)[8].([]any)
	return sel
}

// roundTripServer serves the given shopping fixture for every request and
// records each request body.
func roundTripServer(t *testing.T, fixture string) (*gflight.Client, *[]string) {
	t.Helper()
	body := fixtureBytes(t, fixture)
	var (
		mu     sync.Mutex
		bodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	c := gflight.New(gflight.WithBaseURL(srv.URL), gflight.WithHTTPClient(srv.Client()))
	return c, &bodies
}

func roundTripRequest() gflight.SearchRequest {
	r := sampleRequest()
	r.ReturnDate = time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	return r
}

func TestRoundTripTopN(t *testing.T) {
	t.Parallel()
	c, bodies := roundTripServer(t, "shopping_results_oneway_jfk_lax.txt")

	trips, err := c.RoundTripTopN(t.Context(), roundTripRequest(), 2)
	if err != nil {
		t.Fatalf("RoundTripTopN: %v", err)
	}
	if len(trips) != 2 {
		t.Fatalf("got %d trips, want 2", len(trips))
	}
	for i, tr := range trips {
		if len(tr.Outbound.Segments) == 0 {
			t.Errorf("trip %d: outbound has no segments", i)
		}
		if len(tr.Return) == 0 {
			t.Errorf("trip %d: no return itineraries", i)
		}
	}

	// One phase-1 request plus one phase-2 request per expanded outbound.
	if len(*bodies) != 3 {
		t.Fatalf("got %d upstream requests, want 3", len(*bodies))
	}

	if sel := selectedFlightOf(t, decodeFreqBody(t, (*bodies)[0])); sel != nil {
		t.Errorf("phase-1 body carried a selected flight: %v", sel)
	}

	var sawPinned bool
	for _, b := range (*bodies)[1:] {
		sel := selectedFlightOf(t, decodeFreqBody(t, b))
		if sel == nil {
			t.Error("phase-2 body missing segment[8]")
			continue
		}
		leg := sel[0].([]any)[0].([]any)
		if leg[0] == "JFK" && leg[2] == "LAX" && leg[4] == "B6" && leg[5] == "123" {
			sawPinned = true
		}
	}
	if !sawPinned {
		t.Error("no phase-2 request pinned the JFK-LAX B6 123 outbound")
	}
}

func TestRoundTripTopNClampsN(t *testing.T) {
	t.Parallel()
	c, _ := roundTripServer(t, "shopping_results_oneway_jfk_lax.txt")

	phase1, err := c.SearchResults(t.Context(), roundTripRequest())
	if err != nil {
		t.Fatalf("SearchResults: %v", err)
	}
	trips, err := c.RoundTripTopN(t.Context(), roundTripRequest(), 999)
	if err != nil {
		t.Fatalf("RoundTripTopN: %v", err)
	}
	if len(trips) != len(phase1.Itineraries) {
		t.Fatalf("got %d trips, want %d (clamped to outbound count)", len(trips), len(phase1.Itineraries))
	}
}

func TestRoundTripTopNNeedsReturnDate(t *testing.T) {
	t.Parallel()
	c := gflight.New()
	if _, err := c.RoundTripTopN(t.Context(), sampleRequest(), 3); err == nil {
		t.Fatal("want error when ReturnDate is unset")
	}
}
