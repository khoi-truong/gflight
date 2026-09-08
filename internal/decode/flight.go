package decode

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Row index map (row = one itinerary entry from inner[2][0] / inner[3][0]).
const (
	rowDetailIdx       = 0
	rowPriceIdx        = 1
	rowBookingTokenIdx = 8
	rowMixedCabinIdx   = 10
)

// detail[] index map (detail = row[0]).
const (
	detailPrimaryCarrierIdx = 0
	detailNamesIdx          = 1
	detailLegsIdx           = 2
	detailDurationIdx       = 9
	detailSelfTransferIdx   = 12
	detailLayoverNamesIdx   = 13
	detailEmissionsIdx      = 22
)

// leg[] index map (leg = detail[2][i]).
const (
	legDepAirportIdx   = 3
	legArrAirportIdx   = 6
	legDepNameIdx      = 4
	legArrNameIdx      = 5
	legDepTimeIdx      = 8
	legArrTimeIdx      = 10
	legDurationIdx     = 11
	legAmenitiesIdx    = 12
	legLegroomShortIdx = 14
	legAircraftIdx     = 17
	legOvernightIdx    = 19
	legDepDateIdx      = 20
	legArrDateIdx      = 21
	legCarrierInfoIdx  = 22
	legLegroomLongIdx  = 30
	legCO2Idx          = 31
)

// emissions[] index map (block = detail[22]).
const (
	emThisGramsIdx    = 7
	emTypicalGramsIdx = 8
	emDeltaPctIdx     = 3
	emTagIdx          = 11
)

// amenities[] slot map (slots = leg[12]).
const (
	amWifiIdx          = 1
	amPowerIdx         = 5
	amOnDemandVideoIdx = 9
	amLegroomRatingIdx = 11
)

// Airport is one endpoint of a [Leg] or [Layover].
type Airport struct {
	Code string
	Name string
}

// Amenities is the subset of onboard-amenity flags Google exposes per leg. A
// nil pointer means "unknown", distinct from an explicit false.
type Amenities struct {
	Wifi          *bool
	Power         *bool
	OnDemandVideo *bool
	LegroomRating int // 0 = unknown; 2 or 3 observed.
}

// Emissions holds the CO2 figures from detail[22]. HasData is false when the
// block was absent or unparseable.
type Emissions struct {
	HasData      bool
	Grams        int
	TypicalGrams int
	DeltaPct     int
	Tag          string // "lower", "typical", "higher", or "".
}

// Layover is a stop between two consecutive legs.
type Layover struct {
	Airport         Airport
	DurationMin     int
	Overnight       bool
	ChangeOfAirport bool
	City            string
}

// Leg is one operated flight within a [Flight].
type Leg struct {
	Carrier          string
	FlightNumber     string
	OperatingCarrier string
	Origin           Airport
	Dest             Airport
	Departure        time.Time
	Arrival          time.Time
	DurationMin      int
	Aircraft         string
	Legroom          string
	Overnight        bool
	CO2Grams         int
	Amenities        Amenities
}

// Flight is one priced itinerary.
type Flight struct {
	Price              *float64 // nil when Google declined to price the row.
	Currency           string
	DurationMin        int
	Stops              int
	Legs               []Leg
	Layovers           []Layover
	Emissions          Emissions
	SelfTransfer       bool
	MixedCabin         bool
	PrimaryCarrier     string
	PrimaryCarrierName string
	BookingToken       string
}

// ErrShapeChanged means a response was well-formed JSON but had no flight array
// where one was expected.
var ErrShapeChanged = errors.New("gflight/decode: response shape changed")

// AllRowsFailedError means every candidate row failed to decode — almost always
// an upstream wire-format change. Samples holds up to three distinct reasons.
type AllRowsFailedError struct {
	Total   int
	Samples []string
}

func (e *AllRowsFailedError) Error() string {
	return fmt.Sprintf("gflight/decode: parsed 0/%d rows (sample reasons: %s)",
		e.Total, strings.Join(e.Samples, "; "))
}

// SessionID reads the shopping-session id at inner[0][4], used to authenticate
// a follow-up GetBookingResults call. Returns "" when absent.
func SessionID(inner any) string {
	return asStr(path(inner, 0, 4))
}

// Flights decodes every itinerary row in an already-JSON-decoded inner payload.
// A zero-length result with err == nil means the search genuinely matched
// nothing.
func Flights(inner any) ([]Flight, error) {
	var rows []any
	for _, i := range []int{2, 3} {
		if block := at(inner, i); isSlice(block) {
			rows = append(rows, sliceOf(at(block, 0))...)
		}
	}
	if rows == nil {
		if !isSlice(at(inner, 2)) && !isSlice(at(inner, 3)) {
			return nil, ErrShapeChanged
		}
		return nil, nil
	}

	out := make([]Flight, 0, len(rows))
	var samples []string
	var failed bool
	for _, row := range rows {
		f, err := parseRow(row)
		if err != nil {
			failed = true
			reason := err.Error()
			if len(samples) < 3 && !slices.Contains(samples, reason) {
				samples = append(samples, reason)
			}
			continue
		}
		out = append(out, f)
	}

	if len(out) == 0 && failed {
		return nil, &AllRowsFailedError{Total: len(rows), Samples: samples}
	}
	return out, nil
}

func parseRow(row any) (Flight, error) {
	detail := at(row, rowDetailIdx)
	if !isSlice(detail) {
		return Flight{}, errors.New("row[0] not a list")
	}

	price, unknown, err := parsePrice(at(row, rowPriceIdx))
	if err != nil {
		return Flight{}, err
	}

	rawLegs := sliceOf(at(detail, detailLegsIdx))
	if len(rawLegs) == 0 {
		return Flight{}, errors.New("detail[2] has no legs")
	}
	legs := make([]Leg, 0, len(rawLegs))
	for _, rl := range rawLegs {
		leg, err := parseLeg(rl)
		if err != nil {
			return Flight{}, err
		}
		legs = append(legs, leg)
	}

	duration, _ := asNonNegInt(at(detail, detailDurationIdx))

	f := Flight{
		Currency:           "",
		DurationMin:        duration,
		Stops:              max(len(legs)-1, 0),
		Legs:               legs,
		Emissions:          parseEmissions(at(detail, detailEmissionsIdx)),
		PrimaryCarrier:     asStr(at(detail, detailPrimaryCarrierIdx)),
		PrimaryCarrierName: asStr(path(detail, detailNamesIdx, 0)),
		BookingToken:       asStr(at(row, rowBookingTokenIdx)),
	}
	if b := asBool(at(detail, detailSelfTransferIdx)); b != nil {
		f.SelfTransfer = *b
	}
	if b := asBool(at(row, rowMixedCabinIdx)); b != nil {
		f.MixedCabin = *b
	}
	if !unknown {
		f.Price = &price
	}
	if len(legs) > 1 {
		f.Layovers = deriveLayovers(legs, sliceOf(at(detail, detailLayoverNamesIdx)))
	}

	if tok := asStr(path(at(row, rowPriceIdx), 1)); tok != "" {
		f.Currency = currencyFromToken(tok)
	}
	return f, nil
}

// parsePrice returns (amount, unknown, err). unknown is true for the
// [[], "<token>"] form — Google declined to surface a shopping-list price and
// the caller must resolve a real fare via a booking-results call.
func parsePrice(block any) (amount float64, unknown bool, err error) {
	s := sliceOf(block)
	if s == nil {
		return 0, false, errors.New("price block missing")
	}
	head, ok := s[0].([]any)
	if !ok {
		return 0, false, errors.New("price head not a list")
	}
	if len(head) == 0 {
		return 0, true, nil
	}
	raw := head[len(head)-1]
	v, ok := asFloat(raw)
	if !ok {
		return 0, false, fmt.Errorf("price not numeric: %v", raw)
	}
	return v, false, nil
}

func parseLeg(fl any) (Leg, error) {
	if !isSlice(fl) {
		return Leg{}, errors.New("leg not a list")
	}
	dep, err := parseTimeTuple(at(fl, legDepDateIdx), at(fl, legDepTimeIdx))
	if err != nil {
		return Leg{}, fmt.Errorf("departure: %w", err)
	}
	arr, err := parseTimeTuple(at(fl, legArrDateIdx), at(fl, legArrTimeIdx))
	if err != nil {
		return Leg{}, fmt.Errorf("arrival: %w", err)
	}

	info := sliceOf(at(fl, legCarrierInfoIdx))
	leg := Leg{
		Carrier:          asStr(at(info, 0)),
		FlightNumber:     asStr(at(info, 1)),
		OperatingCarrier: asStr(at(info, 2)),
		Origin:           Airport{Code: asStr(at(fl, legDepAirportIdx)), Name: asStr(at(fl, legDepNameIdx))},
		Dest:             Airport{Code: asStr(at(fl, legArrAirportIdx)), Name: asStr(at(fl, legArrNameIdx))},
		Departure:        dep,
		Arrival:          arr,
		Aircraft:         asStr(at(fl, legAircraftIdx)),
	}
	leg.DurationMin, _ = asNonNegInt(at(fl, legDurationIdx))
	if lr := asStr(at(fl, legLegroomLongIdx)); lr != "" {
		leg.Legroom = lr
	} else {
		leg.Legroom = asStr(at(fl, legLegroomShortIdx))
	}
	if b := asBool(at(fl, legOvernightIdx)); b != nil {
		leg.Overnight = *b
	}
	leg.CO2Grams, _ = asNonNegInt(at(fl, legCO2Idx))
	leg.Amenities = parseAmenities(at(fl, legAmenitiesIdx))
	return leg, nil
}

func parseAmenities(slots any) Amenities {
	s := sliceOf(slots)
	if s == nil {
		return Amenities{}
	}
	a := Amenities{
		Wifi:          asBool(at(s, amWifiIdx)),
		Power:         asBool(at(s, amPowerIdx)),
		OnDemandVideo: asBool(at(s, amOnDemandVideoIdx)),
	}
	a.LegroomRating, _ = asNonNegInt(at(s, amLegroomRatingIdx))
	return a
}

func parseEmissions(block any) Emissions {
	s := sliceOf(block)
	if s == nil {
		return Emissions{}
	}
	e := Emissions{HasData: true}
	e.Grams, _ = asNonNegInt(at(s, emThisGramsIdx))
	e.TypicalGrams, _ = asNonNegInt(at(s, emTypicalGramsIdx))
	if d, ok := asInt(at(s, emDeltaPctIdx)); ok {
		e.DeltaPct = d
	}
	if tag, ok := asInt(at(s, emTagIdx)); ok {
		switch tag {
		case 1:
			e.Tag = "lower"
		case 2:
			e.Tag = "typical"
		case 3:
			e.Tag = "higher"
		}
	}
	return e
}

func deriveLayovers(legs []Leg, names []any) []Layover {
	out := make([]Layover, 0, len(legs)-1)
	for i := range len(legs) - 1 {
		prev, next := legs[i], legs[i+1]
		wait := next.Departure.Sub(prev.Arrival)
		lo := Layover{
			Airport:         prev.Dest,
			DurationMin:     max(int(wait.Minutes()), 0),
			Overnight:       !sameDate(prev.Arrival, next.Departure),
			ChangeOfAirport: prev.Dest.Code != next.Origin.Code,
		}
		if i < len(names) {
			if entry := sliceOf(names[i]); entry != nil {
				if name := asStr(at(entry, 4)); name != "" {
					lo.Airport.Name = name
				}
				lo.City = asStr(at(entry, 5))
			}
		}
		out = append(out, lo)
	}
	return out
}

// parseTimeTuple builds a time from a [y,m,d] date array and an [h,m] time
// array. Missing entries default to zero; each array must have at least one
// non-null value or the leg is unusable. The time is naive (no zone) — Google
// reports local airport time.
func parseTimeTuple(dateArr, timeArr any) (time.Time, error) {
	d := sliceOf(dateArr)
	t := sliceOf(timeArr)
	if !anyNonNil(d) || !anyNonNil(t) {
		return time.Time{}, errors.New("empty date or time array")
	}
	year := tupleAt(d, 0, 0)
	month := tupleAt(d, 1, 1)
	day := tupleAt(d, 2, 1)
	hour := tupleAt(t, 0, 0)
	minVal := tupleAt(t, 1, 0)
	if month < 1 || month > 12 {
		month = 1
	}
	if day < 1 || day > 31 {
		day = 1
	}
	return time.Date(year, time.Month(month), day, hour, minVal, 0, 0, time.UTC), nil
}

func tupleAt(s []any, i, fallback int) int {
	if i >= len(s) || s[i] == nil {
		return fallback
	}
	if n, ok := asInt(s[i]); ok {
		return n
	}
	return fallback
}

func anyNonNil(s []any) bool {
	for _, v := range s {
		if v != nil {
			return true
		}
	}
	return false
}

func sameDate(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
