package gflight_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
		if got := r.URL.Query().Get("rpcids"); got != "LqxFAb" {
			t.Errorf("rpcids = %q, want the shopping rpc id", got)
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

func TestSearchResults(t *testing.T) {
	t.Parallel()
	c := serveFixture(t, "shopping_results_oneway_jfk_lax.txt", http.StatusOK)

	res, err := c.SearchResults(t.Context(), sampleRequest())
	if err != nil {
		t.Fatalf("SearchResults: %v", err)
	}
	if res.SessionID == "" {
		t.Error("SearchResult carried no session id")
	}
	if len(res.Itineraries) == 0 {
		t.Fatal("no itineraries")
	}

	its, err := c.Search(t.Context(), sampleRequest())
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(its) != len(res.Itineraries) {
		t.Errorf("Search returned %d itineraries, SearchResults %d", len(its), len(res.Itineraries))
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

// f.req positions asserted end to end. They mirror the named constants in
// internal/encoding/freq.go and docs/wire/shopping-results.md.
const (
	outerSortIdx = 2

	mainPassengersIdx          = 6
	mainMaxPriceIdx            = 7
	mainBagsIdx                = 10
	mainSegmentsIdx            = 13
	mainExcludeBasicEconomyIdx = 28

	segTimeWindowIdx      = 2
	segAirlineIncludeIdx  = 4
	segAirlineExcludeIdx  = 5
	segMaxDurationIdx     = 7
	segLayoverAirportsIdx = 9
	segMinLayoverIdx      = 11
	segMaxLayoverIdx      = 12
	segLessEmissionsIdx   = 13
)

// TestSearchFiltersReachTheWire is the end-to-end proof that a filtered
// SearchRequest lands in the POSTed f.req body, on both legs of a round trip.
func TestSearchFiltersReachTheWire(t *testing.T) {
	t.Parallel()
	c, bodies := roundTripServer(t, "shopping_results_oneway_jfk_lax.txt")

	req := sampleRequest()
	req.ReturnDate = time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	req.Adults = 2
	req.Children = 1
	req.InfantsOnLap = 1
	req.InfantsInSeat = 1
	req.SortBy = gflight.SortCheapest
	req.MaxPrice = 700
	req.CheckedBags = 2
	req.CarryOnBags = 1
	req.MaxStops = 1
	req.IncludeAirlines = []string{"B6", "AA"}
	req.ExcludeAirlines = []string{"NK"}
	req.MaxDuration = 8 * time.Hour
	req.LayoverAirports = []string{"DFW"}
	req.MinLayover = 90 * time.Minute
	req.MaxLayover = 5 * time.Hour
	req.DepartureWindow = gflight.TimeWindow{EarliestHour: 6, LatestHour: 12}
	req.ArrivalWindow = gflight.TimeWindow{LatestHour: 22}
	req.LessEmissionsOnly = true
	req.ExcludeBasicEconomy = true

	if _, err := c.SearchResults(t.Context(), req); err != nil {
		t.Fatalf("SearchResults: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("got %d requests, want 1", len(*bodies))
	}

	filters := decodeFreqBody(t, (*bodies)[0])
	main, ok := filters[1].([]any)
	if !ok {
		t.Fatalf("filters[1] not a list")
	}

	mainWant := map[int]any{
		mainPassengersIdx:          []any{float64(2), float64(1), float64(1), float64(1)},
		mainMaxPriceIdx:            []any{nil, float64(700)},
		mainBagsIdx:                []any{float64(2), float64(1)},
		mainExcludeBasicEconomyIdx: float64(1),
	}
	for idx, want := range mainWant {
		if got := main[idx]; !reflect.DeepEqual(got, want) {
			t.Errorf("main[%d] = %#v, want %#v", idx, got, want)
		}
	}
	if got := filters[outerSortIdx]; got != float64(2) {
		t.Errorf("sort mode = %#v, want 2 (cheapest)", got)
	}

	segs, ok := main[mainSegmentsIdx].([]any)
	if !ok || len(segs) != 2 {
		t.Fatalf("want 2 segments, got %#v", main[mainSegmentsIdx])
	}
	segWant := map[int]any{
		segTimeWindowIdx:      []any{float64(6), float64(12), nil, float64(22)},
		segAirlineIncludeIdx:  []any{"B6", "AA"},
		segAirlineExcludeIdx:  []any{"NK"},
		segMaxDurationIdx:     []any{float64(480)},
		segLayoverAirportsIdx: []any{"DFW"},
		segMinLayoverIdx:      float64(90),
		segMaxLayoverIdx:      float64(300),
		segLessEmissionsIdx:   []any{float64(1)},
	}
	// Per-segment filters apply to the outbound and the return alike.
	for i, s := range segs {
		seg, ok := s.([]any)
		if !ok {
			t.Fatalf("segment %d not a list", i)
		}
		for idx, want := range segWant {
			if got := seg[idx]; !reflect.DeepEqual(got, want) {
				t.Errorf("segment %d slot %d = %#v, want %#v", i, idx, got, want)
			}
		}
	}
}

// TestSearchUnfilteredStaysInert guards the additive contract: a request with no
// filters set must still encode the pre-M3 body.
func TestSearchUnfilteredStaysInert(t *testing.T) {
	t.Parallel()
	c, bodies := roundTripServer(t, "shopping_results_oneway_jfk_lax.txt")

	if _, err := c.Search(t.Context(), sampleRequest()); err != nil {
		t.Fatalf("Search: %v", err)
	}
	filters := decodeFreqBody(t, (*bodies)[0])
	main := filters[1].([]any)
	seg := main[mainSegmentsIdx].([]any)[0].([]any)

	inert := map[string]any{
		"main[7]":     main[mainMaxPriceIdx],
		"main[10]":    main[mainBagsIdx],
		"main[28]":    main[mainExcludeBasicEconomyIdx],
		"segment[2]":  seg[segTimeWindowIdx],
		"segment[4]":  seg[segAirlineIncludeIdx],
		"segment[5]":  seg[segAirlineExcludeIdx],
		"segment[7]":  seg[segMaxDurationIdx],
		"segment[9]":  seg[segLayoverAirportsIdx],
		"segment[11]": seg[segMinLayoverIdx],
		"segment[12]": seg[segMaxLayoverIdx],
		"segment[13]": seg[segLessEmissionsIdx],
	}
	for name, got := range inert {
		want := any(nil)
		if name == "main[28]" {
			want = float64(0)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("unfiltered %s = %#v, want %#v", name, got, want)
		}
	}
	if pax := main[mainPassengersIdx].([]any); !reflect.DeepEqual(pax, []any{float64(1), float64(0), float64(0), float64(0)}) {
		t.Errorf("unfiltered passengers = %#v", pax)
	}
	if got := filters[outerSortIdx]; got != float64(1) {
		t.Errorf("unfiltered sort mode = %#v, want 1 (best)", got)
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
