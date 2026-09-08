package gflight

import "time"

// CabinClass is the fare cabin requested for a search.
type CabinClass string

// Supported cabin classes.
const (
	CabinEconomy        CabinClass = "economy"
	CabinPremiumEconomy CabinClass = "premium_economy"
	CabinBusiness       CabinClass = "business"
	CabinFirst          CabinClass = "first"
)

// SearchRequest describes one Google Flights query. A zero ReturnDate means a
// one-way search.
type SearchRequest struct {
	Origin      string     // IATA code, e.g. "SGN".
	Destination string     // IATA code, e.g. "HAN".
	DepartDate  time.Time  // Local departure date; only the date part is used.
	ReturnDate  time.Time  // Zero for one-way.
	Adults      int        // Defaults to 1 when zero.
	Children    int        //
	Cabin       CabinClass // Defaults to [CabinEconomy] when empty.
	Currency    string     // ISO 4217 code, e.g. "VND". Overrides the client default.
	MaxStops    int        // 0 means no constraint.
}

// Airport is one endpoint of a [Segment] or [Layover].
type Airport struct {
	Code string // IATA code.
	Name string
	City string
}

// Price is a monetary amount in currency major units (e.g. dollars, not cents).
//
// Unknown is true when Google returned an itinerary but no shopping-list price
// for it — common for premium-cabin round trips. Amount is then zero and
// callers must not treat it as free; use [Itinerary.BookingToken] to resolve a
// real fare.
type Price struct {
	Amount   float64
	Currency string // ISO 4217 code.
	Unknown  bool
}

// Amenities are the onboard-amenity flags Google exposes per [Segment]. A nil
// pointer means "unknown", distinct from an explicit false.
type Amenities struct {
	WiFi          *bool
	Power         *bool
	OnDemandVideo *bool
	LegroomRating int // 0 = unknown; 2 ("normal") or 3 ("extra") observed.
}

// Segment is a single operated flight leg.
type Segment struct {
	Carrier          string // IATA airline code, e.g. "VN".
	OperatingCarrier string // IATA code of the operating carrier when it differs.
	FlightNumber     string
	Origin           Airport
	Destination      Airport
	DepartureTime    time.Time
	ArrivalTime      time.Time
	Duration         time.Duration
	Aircraft         string
	Legroom          string // Human-readable, e.g. "31 inches".
	Overnight        bool
	CO2Grams         int // Per-passenger CO2e for this leg; 0 = unknown.
	Amenities        Amenities
}

// Layover is a stop between two consecutive [Segment]s of an [Itinerary].
type Layover struct {
	Airport         Airport
	Duration        time.Duration
	Overnight       bool
	ChangeOfAirport bool
}

// Emissions holds the itinerary-level CO2e comparison from Google. Present is
// false when Google did not supply the block.
type Emissions struct {
	Present      bool
	Grams        int
	TypicalGrams int
	DeltaPercent int    // Signed; negative means below the typical route emission.
	Tag          string // "lower", "typical", "higher", or "".
}

// Itinerary is one priced journey made of one or more [Segment]s.
type Itinerary struct {
	Segments []Segment
	Price    Price
	Duration time.Duration
	Stops    int
	Layovers []Layover

	// BookingID is retained for backwards compatibility and mirrors
	// BookingToken.
	BookingID string
	// BookingToken is the opaque per-row token needed to resolve real fares
	// via a future booking-results call.
	BookingToken string

	Emissions      Emissions
	SelfTransfer   bool   // Itinerary requires collecting and re-checking bags.
	MixedCabin     bool   // Cabin class is not uniform across segments.
	PrimaryCarrier string // IATA code, or "multi" for a codeshare mix.
}
