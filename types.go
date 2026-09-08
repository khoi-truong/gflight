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

// SortOrder is the order Google returns itineraries in.
type SortOrder int

// Supported sort orders. The zero value, [SortBest], is Google's default
// relevance ranking.
const (
	SortBest SortOrder = iota
	SortCheapest
	SortDepartureTime
	SortArrivalTime
	SortDuration
)

// TimeWindow constrains a departure or arrival to an hour-of-day range. Hours
// are 0–23 in local airport time.
//
// The zero value is no constraint. A zero LatestHour means "no upper bound" —
// there is no way to ask for arrivals before 01:00, which matches Google's own
// slider.
type TimeWindow struct {
	EarliestHour int
	LatestHour   int
}

// SearchRequest describes one Google Flights query. A zero ReturnDate means a
// one-way search. Every filter field is inert at its zero value.
type SearchRequest struct {
	Origin      string     // IATA code, e.g. "SGN".
	Destination string     // IATA code, e.g. "HAN".
	DepartDate  time.Time  // Local departure date; only the date part is used.
	ReturnDate  time.Time  // Zero for one-way. Set it and use [Client.RoundTripTopN] for return options priced against a chosen outbound.
	Adults      int        // Defaults to 1 when zero.
	Children    int        //
	Cabin       CabinClass // Defaults to [CabinEconomy] when empty.
	Currency    string     // ISO 4217 code, e.g. "VND". Overrides the client default.
	MaxStops    int        // 0 means no constraint.

	// InfantsOnLap and InfantsInSeat are counted separately from Children;
	// Google prices a lap infant differently from one holding a seat.
	InfantsOnLap  int
	InfantsInSeat int

	// SortBy orders the returned itineraries. Defaults to [SortBest].
	SortBy SortOrder

	// IncludeAirlines restricts results to these IATA airline codes or
	// alliance names ("STAR_ALLIANCE", "SKYTEAM", "ONEWORLD"). Empty means no
	// constraint. Applied to every segment of the journey.
	IncludeAirlines []string
	// ExcludeAirlines drops these IATA airline codes or alliance names.
	ExcludeAirlines []string

	// MaxPrice caps the itinerary price, expressed in the currency the search
	// asks for. 0 means no cap.
	MaxPrice int

	// CheckedBags and CarryOnBags ask Google to price the fare with that many
	// bags per passenger included.
	CheckedBags int
	CarryOnBags int

	// MaxDuration caps the total travel time of each segment. 0 means no cap;
	// sub-minute precision is discarded.
	MaxDuration time.Duration

	// LayoverAirports restricts connections to these IATA airport codes.
	LayoverAirports []string
	// MinLayover and MaxLayover bound the wait at each connection. 0 means no
	// bound; sub-minute precision is discarded.
	MinLayover time.Duration
	MaxLayover time.Duration

	// DepartureWindow and ArrivalWindow constrain the hour of day each segment
	// may leave and land.
	DepartureWindow TimeWindow
	ArrivalWindow   TimeWindow

	// LessEmissionsOnly keeps only itineraries Google flags as lower-emission.
	LessEmissionsOnly bool

	// ExcludeBasicEconomy drops basic-economy fares.
	ExcludeBasicEconomy bool
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
	WiFi *bool
	// Power is the folded power flag: true when either an in-seat AC outlet or
	// a USB outlet is offered. Use ACPower / USBPower for the finer split.
	Power *bool
	// OnDemandVideo is the folded video flag: true when either on-demand video
	// or a seat-back screen is offered. Use StreamingVideo / InSeatVideo for
	// the finer split.
	OnDemandVideo *bool
	LegroomRating int // 0 = unknown; 2 ("normal") or 3 ("extra") observed.

	// ACPower is the in-seat AC power outlet on its own.
	ACPower *bool
	// USBPower is the in-seat USB outlet on its own.
	USBPower *bool
	// StreamingVideo is on-demand video streamed to the passenger's own
	// device, on its own.
	StreamingVideo *bool
	// InSeatVideo is a seat-back entertainment screen, on its own.
	InSeatVideo *bool
}

// Segment is a single operated flight leg.
type Segment struct {
	Carrier          string // IATA airline code, e.g. "VN".
	OperatingCarrier string // IATA code of the operating carrier when it differs.
	FlightNumber     string
	// OperatingFlightNumber is the flight number under which the operating
	// carrier runs the leg, when it differs from FlightNumber. Empty when
	// Google did not supply one.
	OperatingFlightNumber string
	Origin                Airport
	Destination           Airport
	DepartureTime         time.Time
	ArrivalTime           time.Time
	Duration              time.Duration
	Aircraft              string
	Legroom               string // Human-readable, e.g. "31 inches".
	Overnight             bool
	CO2Grams              int // Per-passenger CO2e for this leg; 0 = unknown.
	Amenities             Amenities
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

// SearchResult is the outcome of one search, including the shopping-session id
// a later booking-results call needs.
type SearchResult struct {
	Itineraries []Itinerary
	SessionID   string // inner[0][4]; "" when upstream omitted it.
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
