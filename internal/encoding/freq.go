package encoding

import (
	"encoding/json"
	"errors"
	"net/url"
	"time"
)

// The `f.req` body Google's FlightsFrontendService expects is a nested,
// double-encoded JSON array. Every position below was mapped by upstream
// reverse-engineering (docs/wire/shopping-results.md); the indices are named
// so search.go never reads a raw subscript, and each probed-but-inert slot is
// left explicitly nil with a note.

// Trip types (main[tripTypeIdx]).
const (
	tripRoundTrip = 1
	tripOneWay    = 2
	tripMultiCity = 3
)

// Cabin classes (main[cabinIdx]).
const (
	cabinEconomy        = 1
	cabinPremiumEconomy = 2
	cabinBusiness       = 3
	cabinFirst          = 4
)

// Segment classifier (segment[segClassifierIdx]): 3 = outbound / only leg,
// 1 = the return leg of a round trip. GetShoppingResults tolerates a uniform 3
// but GetBookingResults does not, so we set it correctly from the start.
const (
	segOutbound = 3
	segReturn   = 1
)

// Sort modes (outer[outerSortIdx]).
const (
	SortBest          = 1
	SortCheapest      = 2
	SortDepartureTime = 3
	SortArrivalTime   = 4
	SortDuration      = 5
)

// FreqCabin is the cabin selector for a [FreqRequest].
type FreqCabin int

// Cabin values accepted by [FreqRequest.Cabin].
const (
	FreqCabinEconomy FreqCabin = iota
	FreqCabinPremiumEconomy
	FreqCabinBusiness
	FreqCabinFirst
)

func (c FreqCabin) wire() int {
	switch c {
	case FreqCabinPremiumEconomy:
		return cabinPremiumEconomy
	case FreqCabinBusiness:
		return cabinBusiness
	case FreqCabinFirst:
		return cabinFirst
	default:
		return cabinEconomy
	}
}

// FreqSelectedLeg pins one leg of an already-chosen outbound itinerary into
// segment[8] for the round-trip second phase, so Google returns only the return
// options compatible with that outbound.
type FreqSelectedLeg struct {
	Origin       string
	Dest         string
	Date         time.Time
	Carrier      string // IATA airline code, e.g. "B6".
	FlightNumber string // Digits only, e.g. "123".
}

// FreqSegment is one leg of the requested journey.
type FreqSegment struct {
	Origin   string
	Dest     string
	Date     time.Time
	MaxStops int  // 0 = no constraint.
	IsReturn bool // Sets the segment classifier to 1.

	// Selected, when non-empty, fills segment[8] with the legs of an
	// already-chosen outbound (round-trip phase 2). Set only on the outbound
	// segment.
	Selected []FreqSelectedLeg
}

// FreqRequest is everything needed to build a one-way (or, later, round-trip)
// GetShoppingResults body.
type FreqRequest struct {
	Segments []FreqSegment
	Adults   int
	Children int
	Cabin    FreqCabin
	SortBy   int // One of the Sort* constants; 0 defaults to SortBest.
}

// EncodeFreq returns the percent-encoded value for the `f.req` form field.
func EncodeFreq(req FreqRequest) (string, error) {
	filters, err := buildFilters(req)
	if err != nil {
		return "", err
	}

	inner, err := json.Marshal(filters)
	if err != nil {
		return "", err
	}
	outer, err := json.Marshal([]any{nil, string(inner)})
	if err != nil {
		return "", err
	}
	// Google's UI uses url-encoding that leaves "/" untouched; QueryEscape is
	// a superset of that and the endpoint accepts it.
	return url.QueryEscape(string(outer)), nil
}

func buildFilters(req FreqRequest) ([]any, error) {
	if len(req.Segments) == 0 {
		return nil, errors.New("gflight/encoding: f.req needs at least one segment")
	}

	var trip int
	switch len(req.Segments) {
	case 1:
		trip = tripOneWay
	case 2:
		trip = tripRoundTrip
	default:
		trip = tripMultiCity
	}

	adults := max(req.Adults, 1)

	segments := make([]any, 0, len(req.Segments))
	for _, s := range req.Segments {
		enc, err := buildSegment(s)
		if err != nil {
			return nil, err
		}
		segments = append(segments, enc)
	}

	sortBy := req.SortBy
	if sortBy == 0 {
		sortBy = SortBest
	}

	// main — 29 slots (indices 0..28). Only the ones below carry meaning;
	// the rest were probed and had no observable effect, but Google rejects
	// a short array, so every slot is present.
	main := make([]any, 29)
	main[mainTripTypeIdx] = trip
	main[mainUnknown4Idx] = []any{} // rejects scalars; [] is the inert form.
	main[mainCabinIdx] = req.Cabin.wire()
	main[mainPassengersIdx] = []any{adults, req.Children, 0, 0} // [adults, children, infants_lap, infants_seat]
	main[mainSegmentsIdx] = segments
	main[mainConstant17Idx] = 1 // hard-coded to 1 by the UI.
	main[mainExcludeBasicEconomyIdx] = 0

	// outer — the top-level 6-slot wrapper.
	filters := []any{
		[]any{}, // outer[0]: always empty.
		main,    // outer[1]: the main settings block.
		sortBy,  // outer[2]: sort mode.
		1,       // outer[3]: 1 = all results (0 caps at ~30).
		0,       // outer[4]: probed, inert.
		1,       // outer[5]: probed, inert.
	}
	return filters, nil
}

// main[] index names.
const (
	mainTripTypeIdx            = 2
	mainUnknown4Idx            = 4
	mainCabinIdx               = 5
	mainPassengersIdx          = 6
	mainSegmentsIdx            = 13
	mainConstant17Idx          = 17
	mainExcludeBasicEconomyIdx = 28
)

// segment[] index names.
const (
	segDepartureIdx      = 0
	segArrivalIdx        = 1
	segTimeWindowIdx     = 2
	segMaxStopsIdx       = 3
	segDateIdx           = 6
	segSelectedFlightIdx = 8
	segClassifierIdx     = 14
)

// buildSelectedFlight encodes an already-chosen outbound for segment[8]. Each
// leg is [origin, "YYYY-MM-DD", dest, null, carrier, flight_number] and the
// list of legs is wrapped one level deep, mirroring how departure/arrival wrap
// their airport lists. The exact nesting is not verified against a live capture
// — see docs/plans/porting.md deviations.
func buildSelectedFlight(legs []FreqSelectedLeg) (any, error) {
	encoded := make([]any, 0, len(legs))
	for _, l := range legs {
		if l.Origin == "" || l.Dest == "" || l.Date.IsZero() {
			return nil, errors.New("gflight/encoding: selected-flight leg missing origin, destination or date")
		}
		encoded = append(encoded, []any{
			l.Origin,
			l.Date.Format(tfsDateLayout),
			l.Dest,
			nil,
			l.Carrier,
			l.FlightNumber,
		})
	}
	return []any{encoded}, nil
}

func buildSegment(s FreqSegment) ([]any, error) {
	if s.Origin == "" || s.Dest == "" {
		return nil, errors.New("gflight/encoding: segment missing origin or destination")
	}
	if s.Date.IsZero() {
		return nil, errors.New("gflight/encoding: segment missing date")
	}

	seg := make([]any, 15)
	// Departure / arrival are wrapped exactly three levels deep — a wrong
	// depth returns zero results with no error.
	seg[segDepartureIdx] = []any{[]any{[]any{s.Origin, 0}}}
	seg[segArrivalIdx] = []any{[]any{[]any{s.Dest, 0}}}
	seg[segTimeWindowIdx] = nil
	if s.MaxStops > 0 {
		seg[segMaxStopsIdx] = s.MaxStops
	} else {
		seg[segMaxStopsIdx] = 0
	}
	seg[segDateIdx] = s.Date.Format(tfsDateLayout)
	if len(s.Selected) > 0 {
		sel, err := buildSelectedFlight(s.Selected)
		if err != nil {
			return nil, err
		}
		seg[segSelectedFlightIdx] = sel
	}
	if s.IsReturn {
		seg[segClassifierIdx] = segReturn
	} else {
		seg[segClassifierIdx] = segOutbound
	}
	return seg, nil
}
