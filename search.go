package gflight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/khoi-truong/gflight/internal/decode"
	"github.com/khoi-truong/gflight/internal/encoding"
	"github.com/khoi-truong/gflight/internal/wire"
)

// rpcPath is the FlightsFrontendService method the web UI calls for shopping
// results. It is undocumented and unversioned.
const rpcPath = "/_/FlightsFrontendUi/data/travel.frontend.flights.FlightsFrontendService/GetShoppingResults"

// Search executes a flight search and returns the itineraries Google offers,
// cheapest-relevant first (Google's "best" order).
//
// One-way search is fixture-verified. Setting [SearchRequest.ReturnDate] adds a
// return segment to a single request, but this round-trip path is best-effort
// and unverified: the two-phase selected-flight flow and round-trip price
// semantics are not yet implemented.
//
// Errors: a cancelled ctx is returned as-is; a non-2xx reply as *[HTTPError]
// (which unwraps to [ErrBadResponse]); a bot wall or rate limit as [ErrBlocked];
// a response whose every row failed to decode as [ErrUpstreamChanged]; and a
// search that genuinely matched nothing as [ErrNoResults].
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]Itinerary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Origin == "" || req.Destination == "" {
		return nil, fmt.Errorf("gflight: search needs origin and destination: %w", ErrBadResponse)
	}
	if req.DepartDate.IsZero() {
		return nil, fmt.Errorf("gflight: search needs a departure date: %w", ErrBadResponse)
	}

	freqReq := c.buildFreq(req)
	body, err := encoding.EncodeFreq(freqReq)
	if err != nil {
		return nil, fmt.Errorf("gflight: encode request: %w", err)
	}

	currency := c.currency
	if req.Currency != "" {
		currency = strings.ToUpper(req.Currency)
	}

	endpoint, err := c.rpcEndpoint(currency)
	if err != nil {
		return nil, err
	}

	raw, err := c.post(ctx, endpoint, "f.req="+body)
	if err != nil {
		return nil, err
	}

	payloads, err := wire.Payloads(raw)
	if err != nil {
		var se *wire.StatusError
		if errors.As(err, &se) {
			return nil, fmt.Errorf("gflight: upstream status %d: %w", se.Code, ErrBlocked)
		}
		if errors.Is(err, wire.ErrNoEnvelope) {
			return nil, fmt.Errorf("gflight: response was not a batchexecute envelope: %w", ErrBlocked)
		}
		return nil, fmt.Errorf("gflight: read response: %w", errors.Join(err, ErrBadResponse))
	}

	var flights []decode.Flight
	for _, p := range payloads {
		var inner any
		if err := json.Unmarshal(p, &inner); err != nil {
			return nil, fmt.Errorf("gflight: decode payload: %w", errors.Join(err, ErrBadResponse))
		}
		if sid := decode.SessionID(inner); sid != "" {
			c.mu.Lock()
			c.lastSessionID = sid
			c.mu.Unlock()
		}
		fs, err := decode.Flights(inner)
		if err != nil {
			var allFailed *decode.AllRowsFailedError
			if errors.As(err, &allFailed) {
				return nil, fmt.Errorf("gflight: %s: %w", allFailed.Error(), ErrUpstreamChanged)
			}
			if errors.Is(err, decode.ErrShapeChanged) {
				return nil, fmt.Errorf("gflight: %w", ErrUpstreamChanged)
			}
			return nil, fmt.Errorf("gflight: decode flights: %w", errors.Join(err, ErrBadResponse))
		}
		flights = append(flights, fs...)
	}

	if len(flights) == 0 {
		return nil, ErrNoResults
	}

	out := make([]Itinerary, 0, len(flights))
	for _, f := range flights {
		out = append(out, toItinerary(f, currency))
	}
	return out, nil
}

func (c *Client) buildFreq(req SearchRequest) encoding.FreqRequest {
	seg := encoding.FreqSegment{
		Origin:   req.Origin,
		Dest:     req.Destination,
		Date:     req.DepartDate,
		MaxStops: req.MaxStops,
	}
	fr := encoding.FreqRequest{
		Segments: []encoding.FreqSegment{seg},
		Adults:   req.Adults,
		Children: req.Children,
		Cabin:    cabinToFreq(req.Cabin),
	}
	if !req.ReturnDate.IsZero() {
		fr.Segments = append(fr.Segments, encoding.FreqSegment{
			Origin:   req.Destination,
			Dest:     req.Origin,
			Date:     req.ReturnDate,
			MaxStops: req.MaxStops,
			IsReturn: true,
		})
	}
	return fr
}

func cabinToFreq(c CabinClass) encoding.FreqCabin {
	switch c {
	case CabinPremiumEconomy:
		return encoding.FreqCabinPremiumEconomy
	case CabinBusiness:
		return encoding.FreqCabinBusiness
	case CabinFirst:
		return encoding.FreqCabinFirst
	default:
		return encoding.FreqCabinEconomy
	}
}

// rpcEndpoint builds the GetShoppingResults URL from the client's base origin,
// carrying the locale query parameters the UI sends.
func (c *Client) rpcEndpoint(currency string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("gflight: bad base URL %q: %w", c.baseURL, err)
	}
	// The RPC route lives at a fixed path on the same origin; the deep-link
	// path segment in the base URL is not part of it.
	base.Path = rpcPath
	base.RawPath = ""
	q := url.Values{}
	q.Set("curr", currency)
	q.Set("hl", c.language)
	q.Set("gl", c.country)
	base.RawQuery = q.Encode()
	return base.String(), nil
}

func (c *Client) post(ctx context.Context, endpoint, body string) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gflight: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	httpReq.Header.Set("User-Agent", c.userAgent)
	httpReq.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gflight: request failed: %w", err)
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("gflight: upstream returned %s: %w", resp.Status, ErrBlocked)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			URL:        endpoint,
			Body:       data,
		}
	}
	if readErr != nil {
		return nil, fmt.Errorf("gflight: read body: %w", errors.Join(readErr, ErrBadResponse))
	}
	return data, nil
}

func toItinerary(f decode.Flight, fallbackCurrency string) Itinerary {
	it := Itinerary{
		Duration:       time.Duration(f.DurationMin) * time.Minute,
		Stops:          f.Stops,
		BookingID:      f.BookingToken,
		BookingToken:   f.BookingToken,
		SelfTransfer:   f.SelfTransfer,
		MixedCabin:     f.MixedCabin,
		PrimaryCarrier: f.PrimaryCarrier,
	}

	currency := f.Currency
	if currency == "" {
		currency = fallbackCurrency
	}
	it.Price = Price{Currency: currency}
	if f.Price != nil {
		it.Price.Amount = *f.Price
	} else {
		it.Price.Unknown = true
	}

	for _, leg := range f.Legs {
		it.Segments = append(it.Segments, Segment{
			Carrier:          leg.Carrier,
			OperatingCarrier: leg.OperatingCarrier,
			FlightNumber:     leg.FlightNumber,
			Origin:           airport(leg.Origin),
			Destination:      airport(leg.Dest),
			DepartureTime:    leg.Departure,
			ArrivalTime:      leg.Arrival,
			Duration:         time.Duration(leg.DurationMin) * time.Minute,
			Aircraft:         leg.Aircraft,
			Legroom:          leg.Legroom,
			Overnight:        leg.Overnight,
			CO2Grams:         leg.CO2Grams,
			Amenities: Amenities{
				WiFi:          leg.Amenities.Wifi,
				Power:         leg.Amenities.Power,
				OnDemandVideo: leg.Amenities.OnDemandVideo,
				LegroomRating: leg.Amenities.LegroomRating,
			},
		})
	}

	for _, lo := range f.Layovers {
		it.Layovers = append(it.Layovers, Layover{
			Airport:         Airport{Code: lo.Airport.Code, Name: lo.Airport.Name, City: lo.City},
			Duration:        time.Duration(lo.DurationMin) * time.Minute,
			Overnight:       lo.Overnight,
			ChangeOfAirport: lo.ChangeOfAirport,
		})
	}

	if f.Emissions.HasData {
		it.Emissions = Emissions{
			Present:      true,
			Grams:        f.Emissions.Grams,
			TypicalGrams: f.Emissions.TypicalGrams,
			DeltaPercent: f.Emissions.DeltaPct,
			Tag:          f.Emissions.Tag,
		}
	}
	return it
}

func airport(a decode.Airport) Airport {
	return Airport{Code: a.Code, Name: a.Name}
}
