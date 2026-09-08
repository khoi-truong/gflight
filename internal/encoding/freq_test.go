package encoding

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"
)

func decodeFreq(t *testing.T, encoded string) []any {
	t.Helper()
	unescaped, err := url.QueryUnescape(encoded)
	if err != nil {
		t.Fatalf("unescape: %v", err)
	}
	var outer []any
	if err := json.Unmarshal([]byte(unescaped), &outer); err != nil {
		t.Fatalf("outer unmarshal: %v", err)
	}
	if len(outer) != 2 || outer[0] != nil {
		t.Fatalf("outer shape = %v", outer)
	}
	inner, ok := outer[1].(string)
	if !ok {
		t.Fatalf("outer[1] not a string")
	}
	var filters []any
	if err := json.Unmarshal([]byte(inner), &filters); err != nil {
		t.Fatalf("inner unmarshal: %v", err)
	}
	return filters
}

func TestEncodeFreqOneWay(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	enc, err := EncodeFreq(FreqRequest{
		Segments: []FreqSegment{{Origin: "JFK", Dest: "LAX", Date: date, MaxStops: 1}},
		Adults:   2,
		Children: 1,
		Cabin:    FreqCabinBusiness,
	})
	if err != nil {
		t.Fatalf("EncodeFreq: %v", err)
	}
	filters := decodeFreq(t, enc)

	main, ok := filters[1].([]any)
	if !ok {
		t.Fatalf("filters[1] not a list")
	}
	if len(main) != 29 {
		t.Errorf("main has %d slots, want 29", len(main))
	}
	if got := main[mainTripTypeIdx]; got != float64(tripOneWay) {
		t.Errorf("trip type = %v, want %d", got, tripOneWay)
	}
	if got := main[mainCabinIdx]; got != float64(cabinBusiness) {
		t.Errorf("cabin = %v, want %d", got, cabinBusiness)
	}
	pax, ok := main[mainPassengersIdx].([]any)
	if !ok || len(pax) != 4 || pax[0] != float64(2) || pax[1] != float64(1) {
		t.Errorf("passengers = %v", main[mainPassengersIdx])
	}
	if main[mainConstant17Idx] != float64(1) {
		t.Errorf("main[17] = %v, want 1", main[mainConstant17Idx])
	}

	segs, ok := main[mainSegmentsIdx].([]any)
	if !ok || len(segs) != 1 {
		t.Fatalf("segments = %v", main[mainSegmentsIdx])
	}
	seg := segs[0].([]any)
	if len(seg) != 15 {
		t.Errorf("segment has %d slots, want 15", len(seg))
	}
	// Departure airport is exactly three levels deep: [[["JFK",0]]].
	dep := seg[segDepartureIdx].([]any)[0].([]any)[0].([]any)
	if dep[0] != "JFK" {
		t.Errorf("departure code = %v", dep[0])
	}
	if seg[segDateIdx] != "2026-06-28" {
		t.Errorf("segment date = %v", seg[segDateIdx])
	}
	if seg[segMaxStopsIdx] != float64(1) {
		t.Errorf("max stops = %v", seg[segMaxStopsIdx])
	}
	if seg[segClassifierIdx] != float64(segOutbound) {
		t.Errorf("classifier = %v, want %d", seg[segClassifierIdx], segOutbound)
	}
}

func TestEncodeFreqRoundTripClassifier(t *testing.T) {
	t.Parallel()
	d1 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	enc, err := EncodeFreq(FreqRequest{
		Segments: []FreqSegment{
			{Origin: "SGN", Dest: "HAN", Date: d1},
			{Origin: "HAN", Dest: "SGN", Date: d2, IsReturn: true},
		},
	})
	if err != nil {
		t.Fatalf("EncodeFreq: %v", err)
	}
	filters := decodeFreq(t, enc)
	main := filters[1].([]any)
	if main[mainTripTypeIdx] != float64(tripRoundTrip) {
		t.Errorf("trip type = %v, want round trip", main[mainTripTypeIdx])
	}
	segs := main[mainSegmentsIdx].([]any)
	if len(segs) != 2 {
		t.Fatalf("want 2 segments, got %d", len(segs))
	}
	if segs[0].([]any)[segClassifierIdx] != float64(segOutbound) {
		t.Errorf("outbound classifier wrong")
	}
	if segs[1].([]any)[segClassifierIdx] != float64(segReturn) {
		t.Errorf("return classifier = %v, want %d", segs[1].([]any)[segClassifierIdx], segReturn)
	}
}

func TestEncodeFreqDefaultsAdultsToOne(t *testing.T) {
	t.Parallel()
	enc, err := EncodeFreq(FreqRequest{
		Segments: []FreqSegment{{Origin: "A", Dest: "B", Date: time.Now()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	main := decodeFreq(t, enc)[1].([]any)
	pax := main[mainPassengersIdx].([]any)
	if pax[0] != float64(1) {
		t.Errorf("adults defaulted to %v, want 1", pax[0])
	}
}

func TestEncodeFreqErrors(t *testing.T) {
	t.Parallel()
	if _, err := EncodeFreq(FreqRequest{}); err == nil {
		t.Error("want error for no segments")
	}
	if _, err := EncodeFreq(FreqRequest{Segments: []FreqSegment{{Origin: "A", Date: time.Now()}}}); err == nil {
		t.Error("want error for missing destination")
	}
	if _, err := EncodeFreq(FreqRequest{Segments: []FreqSegment{{Origin: "A", Dest: "B"}}}); err == nil {
		t.Error("want error for missing date")
	}
}

func TestEncodeFreqEscapingLeavesSlash(t *testing.T) {
	t.Parallel()
	enc, err := EncodeFreq(FreqRequest{
		Segments: []FreqSegment{{Origin: "JFK", Dest: "LAX", Date: time.Now()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A well-formed f.req value round-trips through QueryUnescape without loss.
	if _, err := url.QueryUnescape(enc); err != nil {
		t.Errorf("encoded value does not unescape: %v", err)
	}
}
