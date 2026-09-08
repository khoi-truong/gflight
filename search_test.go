package gflight_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khoi-truong/gflight"
)

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("internal", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// serveFixture starts a server that answers every request with the named
// fixture body and returns a client pointed at it.
func serveFixture(t *testing.T, name string, status int) *gflight.Client {
	t.Helper()
	body := fixtureBytes(t, name)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "GetShoppingResults") {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("curr"); got != strings.ToUpper(got) {
			t.Errorf("curr not upper-cased: %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return gflight.New(gflight.WithBaseURL(srv.URL), gflight.WithHTTPClient(srv.Client()))
}

func sampleRequest() gflight.SearchRequest {
	return gflight.SearchRequest{
		Origin:      "JFK",
		Destination: "LAX",
		DepartDate:  time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
		Adults:      1,
		Cabin:       gflight.CabinEconomy,
		Currency:    "USD",
	}
}

func TestSearchOneWay(t *testing.T) {
	t.Parallel()
	c := serveFixture(t, "shopping_results_oneway_jfk_lax.txt", http.StatusOK)

	its, err := c.Search(t.Context(), sampleRequest())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(its) == 0 {
		t.Fatal("no itineraries")
	}
	for _, it := range its {
		if len(it.Segments) == 0 {
			t.Error("itinerary has no segments")
		}
		if it.Price.Currency != "USD" {
			t.Errorf("currency = %q, want USD", it.Price.Currency)
		}
		if !it.Price.Unknown && it.Price.Amount <= 0 {
			t.Errorf("priced itinerary has non-positive amount: %+v", it.Price)
		}
	}
	if c.SessionID() == "" {
		t.Error("client did not capture a session id")
	}
}

func TestSearchNoPrice(t *testing.T) {
	t.Parallel()
	c := serveFixture(t, "shopping_results_no_price.txt", http.StatusOK)

	its, err := c.Search(t.Context(), sampleRequest())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var sawUnknown bool
	for _, it := range its {
		if it.Price.Unknown {
			sawUnknown = true
			if it.Price.Amount != 0 {
				t.Errorf("unknown price has non-zero amount %v", it.Price.Amount)
			}
		}
	}
	if !sawUnknown {
		t.Error("expected an itinerary with an unknown price")
	}
}

func TestSearchNoResults(t *testing.T) {
	t.Parallel()
	c := serveFixture(t, "shopping_results_no_results.txt", http.StatusOK)

	_, err := c.Search(t.Context(), sampleRequest())
	if !errors.Is(err, gflight.ErrNoResults) {
		t.Fatalf("err = %v, want ErrNoResults", err)
	}
}

func TestSearchBlocked429Body(t *testing.T) {
	t.Parallel()
	c := serveFixture(t, "shopping_results_error_429.txt", http.StatusOK)

	_, err := c.Search(t.Context(), sampleRequest())
	if !errors.Is(err, gflight.ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestSearchBlockedHTTP429(t *testing.T) {
	t.Parallel()
	c := serveFixture(t, "shopping_results_error_429.txt", http.StatusTooManyRequests)

	_, err := c.Search(t.Context(), sampleRequest())
	if !errors.Is(err, gflight.ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestSearchTruncatedChunk(t *testing.T) {
	t.Parallel()
	c := serveFixture(t, "shopping_results_truncated_chunk.txt", http.StatusOK)

	_, err := c.Search(t.Context(), sampleRequest())
	if !errors.Is(err, gflight.ErrBadResponse) {
		t.Fatalf("err = %v, want ErrBadResponse", err)
	}
}

func TestSearchMultichunk(t *testing.T) {
	t.Parallel()
	c := serveFixture(t, "shopping_results_multichunk.txt", http.StatusOK)

	its, err := c.Search(t.Context(), sampleRequest())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(its) == 0 {
		t.Fatal("no itineraries from multichunk fixture")
	}
}

func TestSearchNonEnvelopeResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>are you a robot?</html>")
	}))
	t.Cleanup(srv.Close)
	c := gflight.New(gflight.WithBaseURL(srv.URL), gflight.WithHTTPClient(srv.Client()))

	_, err := c.Search(t.Context(), sampleRequest())
	if !errors.Is(err, gflight.ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestSearchValidation(t *testing.T) {
	t.Parallel()
	c := gflight.New()
	cases := []gflight.SearchRequest{
		{Destination: "LAX", DepartDate: time.Now()},
		{Origin: "JFK", DepartDate: time.Now()},
		{Origin: "JFK", Destination: "LAX"},
	}
	for i, req := range cases {
		if _, err := c.Search(t.Context(), req); !errors.Is(err, gflight.ErrBadResponse) {
			t.Errorf("case %d: err = %v, want ErrBadResponse", i, err)
		}
	}
}
