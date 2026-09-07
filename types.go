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
	Currency    string     // ISO 4217 code, e.g. "VND".
	MaxStops    int        // 0 means no constraint.
}

// Airport is one endpoint of a [Segment].
type Airport struct {
	Code string // IATA code.
	Name string
	City string
}

// Price is a monetary amount in minor-unit-agnostic decimal form.
type Price struct {
	Amount   float64
	Currency string // ISO 4217 code.
}

// Segment is a single operated flight leg.
type Segment struct {
	Carrier       string // IATA airline code, e.g. "VN".
	FlightNumber  string
	Origin        Airport
	Destination   Airport
	DepartureTime time.Time
	ArrivalTime   time.Time
	Duration      time.Duration
	Aircraft      string
}

// Itinerary is one priced journey made of one or more [Segment]s.
type Itinerary struct {
	Segments  []Segment
	Price     Price
	Duration  time.Duration
	Stops     int
	BookingID string // Opaque upstream identifier, if any.
}
