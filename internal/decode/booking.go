package decode

import (
	"errors"
	"net/url"
	"slices"
)

// Booking option layout inside a GetBookingResults payload. Cross-referenced to
// docs/wire/booking-results.md; never read a raw subscript elsewhere.
const (
	// bookingInnerOptionsIdx is inner[1]; the option rows are its element 0.
	// The first chunk of a booking response carries no options at all, so an
	// absent list is normal, not an error.
	bookingInnerOptionsIdx = 1

	bookingVendorIdx   = 1  // [[code, name, null, bool]]
	bookingLinkIdx     = 5  // [display_host, null, [base_url, [[key, value]]]]
	bookingPriceIdx    = 7  // [[..., amount], "<currency token>"]
	bookingFareInfoIdx = 21 // [[carrier, FARE_CODE], ..., "Fare name"]
)

// BookingOption is one bookable fare behind an itinerary: who sells it, what it
// costs, and where to buy it.
type BookingOption struct {
	VendorCode string // IATA airline code or OTA identifier, e.g. "AA".
	VendorName string // Display name, e.g. "American".
	Amount     float64
	Currency   string // ISO 4217, or "" when the token did not carry one.
	URL        string // Booking deep link, already query-encoded.
	FareCode   string // Upstream fare code, e.g. "BASIC ECONOMY".
	FareName   string // Human-readable name, e.g. "Basic Economy".
}

// BookingOptions decodes the vendor offers in an already-JSON-decoded
// GetBookingResults payload. A payload that carries no option list yields
// (nil, nil); a payload whose every row is unparseable yields
// [*AllRowsFailedError].
func BookingOptions(inner any) ([]BookingOption, Stats, error) {
	block := at(inner, bookingInnerOptionsIdx)
	if !isSlice(block) {
		return nil, Stats{}, nil
	}
	rows := sliceOf(at(block, 0))
	if rows == nil {
		return nil, Stats{}, nil
	}

	out := make([]BookingOption, 0, len(rows))
	var samples []string
	stats := Stats{Rows: len(rows)}
	for _, row := range rows {
		opt, err := parseBookingRow(row)
		if err != nil {
			stats.Failures++
			reason := err.Error()
			if len(samples) < 3 && !slices.Contains(samples, reason) {
				samples = append(samples, reason)
			}
			continue
		}
		out = append(out, opt)
	}

	if len(out) == 0 && stats.Failures > 0 {
		return nil, stats, &AllRowsFailedError{Total: len(rows), Samples: samples}
	}
	return out, stats, nil
}

func parseBookingRow(row any) (BookingOption, error) {
	amount, unknown, err := parsePrice(at(row, bookingPriceIdx))
	if err != nil {
		return BookingOption{}, err
	}
	if unknown {
		return BookingOption{}, errors.New("booking option has no price")
	}

	vendor := path(row, bookingVendorIdx, 0)
	opt := BookingOption{
		VendorCode: asStr(at(vendor, 0)),
		VendorName: asStr(at(vendor, 1)),
		Amount:     amount,
		Currency:   currencyFromToken(asStr(path(row, bookingPriceIdx, 1))),
		URL:        bookingURL(at(row, bookingLinkIdx)),
		FareCode:   asStr(path(row, bookingFareInfoIdx, 0, 1)),
		FareName:   asStr(path(row, bookingFareInfoIdx, 3)),
	}
	if opt.VendorCode == "" && opt.VendorName == "" {
		return BookingOption{}, errors.New("booking option has no vendor")
	}
	return opt, nil
}

// bookingURL rebuilds the click-out link from row[5]: a base URL plus a list of
// [key, value] query pairs. Returns "" when either half is missing.
func bookingURL(link any) string {
	target := at(link, 2)
	base := asStr(at(target, 0))
	if base == "" {
		return ""
	}
	q := url.Values{}
	for _, pair := range sliceOf(at(target, 1)) {
		if k := asStr(at(pair, 0)); k != "" {
			q.Set(k, asStr(at(pair, 1)))
		}
	}
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}
