package encoding

import (
	"encoding/json"
	"errors"
	"net/url"
)

// bookingMainLen is how much of the main settings block GetBookingResults
// takes: the first 18 slots (0..17) of the 29 GetShoppingResults expects. The
// filter slots past main[17] describe the shopping list, which is already
// settled by the time a booking token exists.
const bookingMainLen = 18

// EncodeBooking returns the percent-encoded `f.req` value for a
// GetBookingResults call: the per-row booking token, plus the same main
// settings block the shopping search used, trimmed to its first 18 slots.
//
// The token is the opaque row[8] value from the shopping response; it is
// required, because the itinerary cannot be identified without it.
func EncodeBooking(req FreqRequest, token string) (string, error) {
	if token == "" {
		return "", errors.New("gflight/encoding: booking request needs a booking token")
	}
	filters, err := buildFilters(req)
	if err != nil {
		return "", err
	}
	main, ok := filters[outerMainIdx].([]any)
	if !ok || len(main) < bookingMainLen {
		return "", errors.New("gflight/encoding: main settings block too short for a booking request")
	}

	body := []any{
		[]any{nil, token},
		main[:bookingMainLen],
	}
	inner, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	outer, err := json.Marshal([]any{nil, string(inner)})
	if err != nil {
		return "", err
	}
	return url.QueryEscape(string(outer)), nil
}
