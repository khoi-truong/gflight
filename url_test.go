package gflight_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/khoi-truong/gflight"
)

func mustParseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u.Query()
}

func TestSearchURLOneWay(t *testing.T) {
	t.Parallel()
	got, err := gflight.SearchURL(gflight.SearchRequest{
		Origin:      "SGN",
		Destination: "HAN",
		DepartDate:  time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Currency:    "vnd",
	})
	if err != nil {
		t.Fatalf("SearchURL: %v", err)
	}
	if !strings.HasPrefix(got, gflight.DefaultBaseURL+"?") {
		t.Errorf("URL %q does not start with the flights base", got)
	}
	q := mustParseQuery(t, got)
	if q.Get("tfs") == "" {
		t.Error("tfs parameter missing")
	}
	if q.Get("curr") != "VND" {
		t.Errorf("curr = %q, want VND (upper-cased)", q.Get("curr"))
	}
}

func TestSearchURLDeterministic(t *testing.T) {
	t.Parallel()
	req := gflight.SearchRequest{
		Origin: "JFK", Destination: "LAX",
		DepartDate: time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
	}
	a, _ := gflight.SearchURL(req)
	b, _ := gflight.SearchURL(req)
	if a != b {
		t.Errorf("SearchURL not deterministic:\n%s\n%s", a, b)
	}
}

func TestSearchURLRoundTripDiffersFromOneWay(t *testing.T) {
	t.Parallel()
	base := gflight.SearchRequest{
		Origin: "JFK", Destination: "LAX",
		DepartDate: time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
	}
	oneWay, _ := gflight.SearchURL(base)

	rt := base
	rt.ReturnDate = time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	roundTrip, _ := gflight.SearchURL(rt)

	if oneWay == roundTrip {
		t.Error("round-trip URL matches one-way URL")
	}
}

func TestSearchURLErrors(t *testing.T) {
	t.Parallel()
	cases := []gflight.SearchRequest{
		{Destination: "HAN", DepartDate: time.Now()},
		{Origin: "SGN", DepartDate: time.Now()},
		{Origin: "SGN", Destination: "HAN"},
	}
	for i, c := range cases {
		if _, err := gflight.SearchURL(c); !errors.Is(err, gflight.ErrBadResponse) {
			t.Errorf("case %d: err = %v, want ErrBadResponse", i, err)
		}
	}
}
