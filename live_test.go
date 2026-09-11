//go:build live

// This file is the only test in the repository that touches the network. It is
// excluded from `mise run test` (and therefore from `mise run ci`) by the
// `live` build tag; run it with `mise run test:live`.
//
// It is a canary, not a unit test: it answers "does the recorded wire shape
// still match what Google serves today?", which no fixture can. Expect it to
// fail for reasons outside this repository — a bot wall, a network path Google
// dislikes, a genuinely empty route — so read a failure as "look at upstream",
// not "the last commit broke something".
//
// There are two very different upstream failures and the messages below are
// written to tell them apart:
//
//   - A *BlockedError, or an HTTP 500 whose wrb.fr status is 13, means the
//     rpc id itself was refused. Every other FlightsFrontendService method is
//     already gated that way; GetShoppingResultsPrefetch (LqxFAb) is the only
//     one left open, and this library has no fallback if it closes. See
//     docs/wire/botguard.md.
//   - Rows that decode to zero or to empty fields mean the payload shape
//     moved. That is a normal, fixable drift: re-capture and re-map.

package gflight_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khoi-truong/gflight"
)

// TestLiveSearchSmoke runs one canonical search against the real endpoint and
// asserts the response decodes into itineraries whose required fields are
// populated. A drifted wire shape shows up here as zero rows or empty fields.
func TestLiveSearchSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := gflight.New(
		gflight.WithRetry(gflight.RetryPolicy{MaxAttempts: 3}),
		gflight.WithRateLimit(2, 1),
	)

	// A dense, always-served route, far enough out to be reliably bookable.
	req := gflight.SearchRequest{
		Origin:      "JFK",
		Destination: "LAX",
		DepartDate:  time.Now().AddDate(0, 0, 30),
		Adults:      1,
		Cabin:       gflight.CabinEconomy,
		Currency:    "USD",
	}

	its, err := c.Search(ctx, req)
	if err != nil {
		failLive(t, "live Search", err)
	}
	if len(its) == 0 {
		t.Fatal("live Search returned 0 itineraries for JFK->LAX: " +
			"the rpc id still answers, so the payload shape likely changed")
	}

	for i, it := range its {
		if len(it.Segments) == 0 {
			t.Errorf("itinerary %d has no segments", i)
			continue
		}
		if it.Duration <= 0 {
			t.Errorf("itinerary %d: Duration = %v, want > 0", i, it.Duration)
		}
		if it.Price.Currency == "" {
			t.Errorf("itinerary %d: empty Price.Currency", i)
		}
		for j, s := range it.Segments {
			if s.Origin.Code == "" || s.Destination.Code == "" {
				t.Errorf("itinerary %d segment %d: missing airport code (%q -> %q)",
					i, j, s.Origin.Code, s.Destination.Code)
			}
			if s.Carrier == "" || s.FlightNumber == "" {
				t.Errorf("itinerary %d segment %d: missing flight identity (%q %q)",
					i, j, s.Carrier, s.FlightNumber)
			}
			if s.DepartureTime.IsZero() || s.ArrivalTime.IsZero() {
				t.Errorf("itinerary %d segment %d: zero departure or arrival time", i, j)
			}
		}
	}

	// Prices are optional per row, but a response where none has one means the
	// price token moved.
	var priced int
	for _, it := range its {
		if !it.Price.Unknown {
			priced++
		}
	}
	if priced == 0 {
		t.Errorf("none of %d itineraries carried a price: the price shape likely changed", len(its))
	}

	// row[8] is the only place a booking token appears; an empty one across the
	// whole response means the row layout moved.
	var tokened int
	for _, it := range its {
		if it.BookingToken != "" {
			tokened++
		}
	}
	if tokened == 0 {
		t.Errorf("none of %d itineraries carried a booking token: row[8] likely moved", len(its))
	}
}

// failLive reports err as a fatal test failure, distinguishing an upstream
// refusal from a genuine wire-format break. The two need different responses
// from whoever reads the failure, so they get different messages.
func failLive(t *testing.T, what string, err error) {
	t.Helper()
	var blocked *gflight.BlockedError
	if errors.As(err, &blocked) {
		t.Fatalf("upstream refused this runner (status %d, retry after %s) — "+
			"not a wire-format failure. If this is transient, it is a bot wall or "+
			"rate limit and the runner's network path is the thing to change. If "+
			"it persists, Google has gated rpcid=LqxFAb "+
			"(GetShoppingResultsPrefetch), the last method open to an anonymous "+
			"caller, and this library has no fallback — see "+
			"docs/wire/botguard.md: %v",
			blocked.StatusCode, blocked.RetryAfter, err)
	}
	t.Fatalf("%s: %v", what, err)
}
