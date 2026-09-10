package gflight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/khoi-truong/gflight/internal/decode"
	"github.com/khoi-truong/gflight/internal/encoding"
	"github.com/khoi-truong/gflight/internal/wire"
)

// BookingOption is one bookable fare behind an itinerary: who sells it, what it
// costs, and where to buy it.
type BookingOption struct {
	// Vendor is the seller's display name, e.g. "American" or "Expedia".
	Vendor string
	// VendorCode is the machine-readable identifier, usually an IATA airline
	// code, e.g. "AA".
	VendorCode string
	Price      Price
	// URL is the booking deep link. It is a Google click-out that redirects to
	// the vendor, not the vendor's own URL.
	URL string
	// FareName is the human-readable fare, e.g. "Basic Economy".
	FareName string
	// FareCode is the upstream fare code, e.g. "BASIC ECONOMY".
	FareCode string
}

// BookingOptions resolves the real, bookable fares behind one itinerary,
// cheapest first as Google orders them. It is the way to price an itinerary
// whose [Price.Unknown] is true.
//
// req must be the same [SearchRequest] the itinerary came from — Google prices
// the booking against the original passenger count and cabin. it must carry a
// non-empty [Itinerary.BookingToken]; a zero-value itinerary is rejected with
// [ErrBadResponse].
//
// Errors otherwise match [Client.SearchResults]: [ErrBlocked] for a bot wall,
// [ErrUpstreamChanged] when every option row failed to decode, and
// [ErrNoResults] when Google offered no bookable fare at all.
func (c *Client) BookingOptions(ctx context.Context, req SearchRequest, it Itinerary) ([]BookingOption, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if it.BookingToken == "" {
		return nil, fmt.Errorf("gflight: booking options need an itinerary with a booking token: %w", ErrBadResponse)
	}

	body, err := encoding.EncodeBooking(c.buildFreq(req), it.BookingToken)
	if err != nil {
		return nil, fmt.Errorf("gflight: encode request: %w", err)
	}

	currency := c.currency
	if req.Currency != "" {
		currency = strings.ToUpper(req.Currency)
	}

	endpoint, err := c.rpcEndpoint(rpcBookingResults, currency)
	if err != nil {
		return nil, err
	}

	raw, err := c.post(ctx, endpoint, "f.req="+body)
	if err != nil {
		var be *BlockedError
		if errors.As(err, &be) && be.DeepLink == "" {
			be.DeepLink = deepLink(req)
		}
		return nil, err
	}

	payloads, err := wire.Payloads(raw)
	if err != nil {
		var se *wire.StatusError
		if errors.As(err, &se) {
			return nil, &BlockedError{StatusCode: se.Code, DeepLink: deepLink(req)}
		}
		if errors.Is(err, wire.ErrNoEnvelope) {
			return nil, &BlockedError{DeepLink: deepLink(req)}
		}
		return nil, fmt.Errorf("gflight: read response: %w", errors.Join(err, ErrBadResponse))
	}

	var out []BookingOption
	for _, p := range payloads {
		var inner any
		if err := json.Unmarshal(p, &inner); err != nil {
			return nil, fmt.Errorf("gflight: decode payload: %w", errors.Join(err, ErrBadResponse))
		}
		opts, stats, err := decode.BookingOptions(inner)
		c.observer.parse(ParseEvent{Rows: stats.Rows, Failures: stats.Failures})
		if err != nil {
			var allFailed *decode.AllRowsFailedError
			if errors.As(err, &allFailed) {
				return nil, fmt.Errorf("gflight: %s: %w", allFailed.Error(), ErrUpstreamChanged)
			}
			return nil, fmt.Errorf("gflight: decode booking options: %w", errors.Join(err, ErrBadResponse))
		}
		for _, o := range opts {
			out = append(out, toBookingOption(o, currency))
		}
	}

	if len(out) == 0 {
		return nil, ErrNoResults
	}
	return out, nil
}

func toBookingOption(o decode.BookingOption, fallbackCurrency string) BookingOption {
	cur := o.Currency
	if cur == "" {
		cur = fallbackCurrency
	}
	return BookingOption{
		Vendor:     o.VendorName,
		VendorCode: o.VendorCode,
		Price:      Price{Amount: o.Amount, Currency: cur},
		URL:        o.URL,
		FareName:   o.FareName,
		FareCode:   o.FareCode,
	}
}
