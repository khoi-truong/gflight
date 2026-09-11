package encoding

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func decodeFreq(t *testing.T, payload string) []any {
	t.Helper()
	var filters []any
	if err := json.Unmarshal([]byte(payload), &filters); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
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

// filterCase asserts one filter's effect on a single f.req slot. want is
// compared against the JSON-decoded value, so numbers are float64.
type filterCase struct {
	name string
	// mutate applies the filter to an otherwise-plain one-way request.
	mutate func(*FreqRequest)
	// pick reads the slot under test out of the decoded filter array.
	pick func(t *testing.T, filters []any) any
	// want is the value with the filter set; inert is the value without it.
	want  any
	inert any
}

func mainSlot(i int) func(*testing.T, []any) any {
	return func(t *testing.T, filters []any) any {
		t.Helper()
		main, ok := filters[1].([]any)
		if !ok {
			t.Fatalf("filters[1] not a list")
		}
		return main[i]
	}
}

func segSlot(i int) func(*testing.T, []any) any {
	return func(t *testing.T, filters []any) any {
		t.Helper()
		main, ok := filters[1].([]any)
		if !ok {
			t.Fatalf("filters[1] not a list")
		}
		segs, ok := main[mainSegmentsIdx].([]any)
		if !ok || len(segs) == 0 {
			t.Fatalf("no segments")
		}
		return segs[0].([]any)[i]
	}
}

func plainRequest() FreqRequest {
	return FreqRequest{
		Segments: []FreqSegment{{
			Origin: "JFK", Dest: "LAX",
			Date: time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
		}},
	}
}

// TestEncodeFreqFilters walks every M3 filter: with the filter set the slot
// carries the documented shape, and with it unset the slot keeps the inert form
// M1 shipped.
func TestEncodeFreqFilters(t *testing.T) {
	t.Parallel()
	cases := []filterCase{
		{
			name:   "max price",
			mutate: func(r *FreqRequest) { r.MaxPrice = 700 },
			pick:   mainSlot(mainMaxPriceIdx),
			want:   []any{nil, float64(700)},
		},
		{
			name:   "bags",
			mutate: func(r *FreqRequest) { r.CheckedBags, r.CarryOnBags = 2, 1 },
			pick:   mainSlot(mainBagsIdx),
			want:   []any{float64(2), float64(1)},
		},
		{
			name:   "exclude basic economy",
			mutate: func(r *FreqRequest) { r.ExcludeBasicEconomy = true },
			pick:   mainSlot(mainExcludeBasicEconomyIdx),
			want:   float64(1),
			inert:  float64(0),
		},
		{
			name:   "infants",
			mutate: func(r *FreqRequest) { r.Adults, r.InfantsOnLap, r.InfantsInSeat = 2, 1, 3 },
			pick:   mainSlot(mainPassengersIdx),
			want:   []any{float64(2), float64(0), float64(1), float64(3)},
			inert:  []any{float64(1), float64(0), float64(0), float64(0)},
		},
		{
			name:   "sort mode",
			mutate: func(r *FreqRequest) { r.SortBy = SortCheapest },
			pick:   func(_ *testing.T, filters []any) any { return filters[2] },
			want:   float64(SortCheapest),
			inert:  float64(SortBest),
		},
		{
			name:   "airline include",
			mutate: func(r *FreqRequest) { r.Segments[0].IncludeAirlines = []string{"B6", "AA"} },
			pick:   segSlot(segAirlineIncludeIdx),
			want:   []any{"B6", "AA"},
		},
		{
			name:   "airline exclude",
			mutate: func(r *FreqRequest) { r.Segments[0].ExcludeAirlines = []string{"NK"} },
			pick:   segSlot(segAirlineExcludeIdx),
			want:   []any{"NK"},
		},
		{
			name:   "max duration",
			mutate: func(r *FreqRequest) { r.Segments[0].MaxDurationMins = 480 },
			pick:   segSlot(segMaxDurationIdx),
			want:   []any{float64(480)},
		},
		{
			name:   "layover airports",
			mutate: func(r *FreqRequest) { r.Segments[0].LayoverAirports = []string{"DFW"} },
			pick:   segSlot(segLayoverAirportsIdx),
			want:   []any{"DFW"},
		},
		{
			name:   "min layover",
			mutate: func(r *FreqRequest) { r.Segments[0].MinLayoverMins = 90 },
			pick:   segSlot(segMinLayoverIdx),
			want:   float64(90),
		},
		{
			name:   "max layover",
			mutate: func(r *FreqRequest) { r.Segments[0].MaxLayoverMins = 300 },
			pick:   segSlot(segMaxLayoverIdx),
			want:   float64(300),
		},
		{
			name:   "less emissions",
			mutate: func(r *FreqRequest) { r.Segments[0].LessEmissionsOnly = true },
			pick:   segSlot(segLessEmissionsIdx),
			want:   []any{float64(1)},
		},
		{
			name: "hour windows",
			mutate: func(r *FreqRequest) {
				s := &r.Segments[0]
				s.DepEarliestHour, s.DepLatestHour = 6, 12
				s.ArrEarliestHour, s.ArrLatestHour = 0, 22
			},
			pick: segSlot(segTimeWindowIdx),
			want: []any{float64(6), float64(12), nil, float64(22)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := plainRequest()
			tc.mutate(&req)
			enc, err := EncodeFreq(req)
			if err != nil {
				t.Fatalf("EncodeFreq: %v", err)
			}
			if got := tc.pick(t, decodeFreq(t, enc)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("filtered slot = %#v, want %#v", got, tc.want)
			}

			base, err := EncodeFreq(plainRequest())
			if err != nil {
				t.Fatalf("EncodeFreq (unfiltered): %v", err)
			}
			if got := tc.pick(t, decodeFreq(t, base)); !reflect.DeepEqual(got, tc.inert) {
				t.Errorf("unfiltered slot = %#v, want %#v", got, tc.inert)
			}
		})
	}
}

// freqRequestAllFilters is a one-way JFK->LAX search with every M3 filter set,
// so the golden below moves whenever any index or shape does.
func freqRequestAllFilters() FreqRequest {
	return FreqRequest{
		Segments: []FreqSegment{{
			Origin: "JFK", Dest: "LAX",
			Date:              time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
			MaxStops:          1,
			IncludeAirlines:   []string{"B6", "AA"},
			ExcludeAirlines:   []string{"NK"},
			MaxDurationMins:   480,
			LayoverAirports:   []string{"DFW"},
			MinLayoverMins:    90,
			MaxLayoverMins:    300,
			DepEarliestHour:   6,
			DepLatestHour:     12,
			ArrLatestHour:     22,
			LessEmissionsOnly: true,
		}},
		Adults:              2,
		Children:            1,
		InfantsOnLap:        1,
		InfantsInSeat:       1,
		Cabin:               FreqCabinPremiumEconomy,
		SortBy:              SortCheapest,
		MaxPrice:            700,
		CheckedBags:         2,
		CarryOnBags:         1,
		ExcludeBasicEconomy: true,
	}
}

// goldenAllFiltersOneWay freezes the fully-filtered body. Every slot's shape is
// structurally derived from docs/wire/shopping-results.md, not from a live
// capture — see docs/plans/porting.md deviations.
const goldenAllFiltersOneWay = "[[],[null,null,2,null,[],2,[2,1,1,1],[null,700],null,null,[2,1],null,null,[[[[[\"JFK\",0]]],[[[\"LAX\",0]]],[6,12,null,22],1,[\"B6\",\"AA\"],[\"NK\"],\"2026-06-28\",[480],null,[\"DFW\"],null,90,300,[1],3]],null,null,null,1,null,null,null,null,null,null,null,null,null,null,1],2,1,0,1]"

func TestEncodeFreqAllFiltersGolden(t *testing.T) {
	t.Parallel()
	got, err := EncodeFreq(freqRequestAllFilters())
	if err != nil {
		t.Fatalf("EncodeFreq: %v", err)
	}
	if got != goldenAllFiltersOneWay {
		t.Errorf("f.req drift:\n got %q\nwant %q", got, goldenAllFiltersOneWay)
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

// goldenSelectedFlightRT freezes the phase-2 round-trip body: SGN->HAN on
// 2026-10-01 with the outbound pinned to VN 245, HAN->SGN return open on
// 2026-10-08. Structurally verified by TestEncodeFreqSelectedFlight; not a live
// capture — see docs/plans/porting.md deviations.
const goldenSelectedFlightRT = "[[],[null,null,1,null,[],1,[1,0,0,0],null,null,null,null,null,null,[[[[[\"SGN\",0]]],[[[\"HAN\",0]]],null,0,null,null,\"2026-10-01\",null,[[[\"SGN\",\"2026-10-01\",\"HAN\",null,\"VN\",\"245\"]]],null,null,null,null,null,3],[[[[\"HAN\",0]]],[[[\"SGN\",0]]],null,0,null,null,\"2026-10-08\",null,null,null,null,null,null,null,1]],null,null,null,1,null,null,null,null,null,null,null,null,null,null,0],1,1,0,1]"

func TestEncodeFreqSelectedFlightGolden(t *testing.T) {
	t.Parallel()
	got, err := EncodeFreq(freqRequestSelectedFlight(t))
	if err != nil {
		t.Fatalf("EncodeFreq: %v", err)
	}
	if got != goldenSelectedFlightRT {
		t.Errorf("f.req drift:\n got %q\nwant %q", got, goldenSelectedFlightRT)
	}
}

func freqRequestSelectedFlight(t *testing.T) FreqRequest {
	t.Helper()
	d1 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	return FreqRequest{
		Segments: []FreqSegment{
			{
				Origin: "SGN", Dest: "HAN", Date: d1,
				Selected: []FreqSelectedLeg{
					{Origin: "SGN", Dest: "HAN", Date: d1, Carrier: "VN", FlightNumber: "245"},
				},
			},
			{Origin: "HAN", Dest: "SGN", Date: d2, IsReturn: true},
		},
	}
}

func TestEncodeFreqSelectedFlight(t *testing.T) {
	t.Parallel()
	enc, err := EncodeFreq(freqRequestSelectedFlight(t))
	if err != nil {
		t.Fatalf("EncodeFreq: %v", err)
	}
	segs := decodeFreq(t, enc)[1].([]any)[mainSegmentsIdx].([]any)
	outbound := segs[0].([]any)

	sel, ok := outbound[segSelectedFlightIdx].([]any)
	if !ok || len(sel) != 1 {
		t.Fatalf("segment[8] = %v", outbound[segSelectedFlightIdx])
	}
	legs := sel[0].([]any)
	if len(legs) != 1 {
		t.Fatalf("want 1 selected leg, got %d", len(legs))
	}
	leg := legs[0].([]any)
	want := []any{"SGN", "2026-10-01", "HAN", nil, "VN", "245"}
	for i := range want {
		if leg[i] != want[i] {
			t.Errorf("selected leg[%d] = %v, want %v", i, leg[i], want[i])
		}
	}
	if segs[1].([]any)[segSelectedFlightIdx] != nil {
		t.Error("return segment[8] should be nil")
	}
}

func TestEncodeFreqSelectedFlightErrors(t *testing.T) {
	t.Parallel()
	d := time.Now()
	_, err := EncodeFreq(FreqRequest{
		Segments: []FreqSegment{
			{Origin: "SGN", Dest: "HAN", Date: d, Selected: []FreqSelectedLeg{{Origin: "SGN", Carrier: "VN"}}},
			{Origin: "HAN", Dest: "SGN", Date: d, IsReturn: true},
		},
	})
	if err == nil {
		t.Error("want error for selected leg missing dest/date")
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
