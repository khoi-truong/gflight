package decode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/khoi-truong/gflight/internal/wire"
)

func innerFromFixture(t *testing.T, name string) []any {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	payloads, err := wire.Payloads(body)
	if err != nil {
		t.Fatalf("wire.Payloads: %v", err)
	}
	if len(payloads) == 0 {
		t.Fatalf("no payloads in %s", name)
	}
	out := make([]any, 0, len(payloads))
	for _, p := range payloads {
		var v any
		if err := json.Unmarshal(p, &v); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		out = append(out, v)
	}
	return out
}

func allFlights(t *testing.T, name string) []Flight {
	t.Helper()
	var out []Flight
	for _, inner := range innerFromFixture(t, name) {
		fs, err := Flights(inner)
		if err != nil {
			t.Fatalf("Flights(%s): %v", name, err)
		}
		out = append(out, fs...)
	}
	return out
}

func TestFlightsOneWay(t *testing.T) {
	t.Parallel()
	flights := allFlights(t, "shopping_results_oneway_jfk_lax.txt")
	if len(flights) == 0 {
		t.Fatal("no flights decoded")
	}
	f := flights[0]
	if len(f.Legs) == 0 {
		t.Fatal("first flight has no legs")
	}
	leg := f.Legs[0]
	if leg.Origin.Code == "" || leg.Dest.Code == "" {
		t.Errorf("leg airports empty: %+v", leg)
	}
	if leg.Departure.IsZero() || leg.Arrival.IsZero() {
		t.Errorf("leg times zero: dep=%v arr=%v", leg.Departure, leg.Arrival)
	}
	if f.Price == nil || *f.Price <= 0 {
		t.Errorf("price = %v, want positive", f.Price)
	}
	if f.Currency != "USD" {
		t.Errorf("currency = %q, want USD", f.Currency)
	}
}

func TestFlightsSessionID(t *testing.T) {
	t.Parallel()
	inner := innerFromFixture(t, "shopping_results_oneway_jfk_lax.txt")[0]
	if sid := SessionID(inner); sid != "REDACTED_SESSION_ID" {
		t.Errorf("SessionID = %q, want REDACTED_SESSION_ID", sid)
	}
}

func TestFlightsNoPrice(t *testing.T) {
	t.Parallel()
	flights := allFlights(t, "shopping_results_no_price.txt")
	if len(flights) == 0 {
		t.Fatal("no flights decoded")
	}
	var sawUnknown bool
	for _, f := range flights {
		if f.Price == nil {
			sawUnknown = true
		}
	}
	if !sawUnknown {
		t.Error("expected at least one row with an unknown (nil) price")
	}
}

func TestFlightsNoResults(t *testing.T) {
	t.Parallel()
	flights := allFlights(t, "shopping_results_no_results.txt")
	if len(flights) != 0 {
		t.Errorf("got %d flights, want 0", len(flights))
	}
}

func TestFlightsLayovers(t *testing.T) {
	t.Parallel()
	flights := allFlights(t, "shopping_results_layover_buf_ath.txt")
	var sawLayover bool
	for _, f := range flights {
		if len(f.Legs) > 1 && len(f.Layovers) == len(f.Legs)-1 {
			sawLayover = true
			for _, lo := range f.Layovers {
				if lo.Airport.Code == "" {
					t.Errorf("layover missing airport code: %+v", lo)
				}
			}
		}
	}
	if !sawLayover {
		t.Error("expected a multi-leg itinerary with layovers")
	}
}

func TestFlightsMultichunk(t *testing.T) {
	t.Parallel()
	flights := allFlights(t, "shopping_results_multichunk.txt")
	if len(flights) == 0 {
		t.Fatal("no flights decoded from multichunk fixture")
	}
}

func TestParseTimeTupleShapes(t *testing.T) {
	t.Parallel()
	date := []any{float64(2026), float64(6), float64(28)}
	cases := []struct {
		name         string
		timeArr      any
		wantH, wantM int
	}{
		{"h and m", []any{float64(9), float64(45)}, 9, 45},
		{"h only", []any{float64(9)}, 9, 0},
		{"null h, m", []any{nil, float64(45)}, 0, 45},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTimeTuple(date, tc.timeArr)
			if err != nil {
				t.Fatalf("parseTimeTuple: %v", err)
			}
			if got.Hour() != tc.wantH || got.Minute() != tc.wantM {
				t.Errorf("got %02d:%02d, want %02d:%02d", got.Hour(), got.Minute(), tc.wantH, tc.wantM)
			}
			if got.Year() != 2026 || got.Month() != 6 || got.Day() != 28 {
				t.Errorf("date part = %v", got)
			}
		})
	}
}

func TestParseTimeTupleEmpty(t *testing.T) {
	t.Parallel()
	if _, err := parseTimeTuple([]any{nil}, []any{nil}); err == nil {
		t.Error("expected error for all-nil date and time arrays")
	}
}

func TestFlightsAllRowsFail(t *testing.T) {
	t.Parallel()
	// inner[2][0] is a list of rows, each of which is unparseable garbage.
	inner := []any{nil, nil, []any{[]any{"garbage", float64(42), true}}, nil}
	_, err := Flights(inner)
	var allFailed *AllRowsFailedError
	if !errors.As(err, &allFailed) {
		t.Fatalf("err = %v, want *AllRowsFailedError", err)
	}
	if allFailed.Total != 3 || len(allFailed.Samples) == 0 {
		t.Errorf("unexpected AllRowsFailedError: %+v", allFailed)
	}
}

func TestFlightsShapeChanged(t *testing.T) {
	t.Parallel()
	inner := []any{nil, nil, "not-a-list", 7}
	_, err := Flights(inner)
	if !errors.Is(err, ErrShapeChanged) {
		t.Fatalf("err = %v, want ErrShapeChanged", err)
	}
}
