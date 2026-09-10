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
// cheapest-relevant first (Google's "best" order). It is [Client.SearchResults]
// without the session id — see there for the round-trip and error semantics.
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]Itinerary, error) {
	res, err := c.SearchResults(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.Itineraries, nil
}

// SearchResults executes a flight search and returns the itineraries alongside
// the shopping-session id a later booking-results call needs.
//
// With [SearchRequest.ReturnDate] set it returns the outbound options of a
// round trip in a single request. To get return itineraries priced against a
// chosen outbound, use [Client.RoundTripTopN], which runs the two-phase
// selected-flight flow.
//
// Errors: a cancelled ctx is returned as-is; a non-2xx reply as *[HTTPError]
// (which unwraps to [ErrBadResponse]); a bot wall or rate limit as [ErrBlocked];
// a response whose every row failed to decode as [ErrUpstreamChanged]; and a
// search that genuinely matched nothing as [ErrNoResults].
func (c *Client) SearchResults(ctx context.Context, req SearchRequest) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if req.Origin == "" || req.Destination == "" {
		return SearchResult{}, fmt.Errorf("gflight: search needs origin and destination: %w", ErrBadResponse)
	}
	if req.DepartDate.IsZero() {
		return SearchResult{}, fmt.Errorf("gflight: search needs a departure date: %w", ErrBadResponse)
	}

	return c.executeFreq(ctx, req, c.buildFreq(req))
}

// DefaultRoundTripTopN is how many outbound itineraries [Client.RoundTripTopN]
// expands into phase-2 return searches when the caller passes n <= 0.
const DefaultRoundTripTopN = 3

// RoundTrip pairs one chosen outbound itinerary with the return itineraries
// Google offers once that outbound is selected. Return prices are trip totals,
// not per-leg — do not add them to the outbound price.
type RoundTrip struct {
	Outbound Itinerary
	Return   []Itinerary
}

// RoundTripTopN runs the real two-phase round-trip search. Phase 1 fetches
// outbound options for req (which must set [SearchRequest.ReturnDate]); phase 2
// re-queries Google once per outbound, with that outbound pinned into
// segment[8], to collect its compatible return itineraries.
//
// n bounds how many outbounds are expanded and is clamped to the number
// returned; n <= 0 uses [DefaultRoundTripTopN]. Phase-2 requests run
// concurrently, capped by [WithMaxConcurrency]. The caller's ctx deadline spans
// both phases — phase 2 does not get a fresh timeout.
//
// An outbound whose phase-2 call fails is dropped; the error is only returned
// when every phase-2 call fails. Errors otherwise match [Client.SearchResults].
func (c *Client) RoundTripTopN(ctx context.Context, req SearchRequest, n int) ([]RoundTrip, error) {
	if req.ReturnDate.IsZero() {
		return nil, fmt.Errorf("gflight: round-trip search needs a return date: %w", ErrBadResponse)
	}

	phase1, err := c.SearchResults(ctx, req)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		n = DefaultRoundTripTopN
	}
	n = min(n, len(phase1.Itineraries))
	outbounds := phase1.Itineraries[:n]

	trips, errs := mapConcurrent(ctx, c.maxConcurrency, outbounds,
		func(ctx context.Context, ob Itinerary) (RoundTrip, error) {
			ret, err := c.returnOptions(ctx, req, ob)
			if err != nil {
				return RoundTrip{}, err
			}
			return RoundTrip{Outbound: ob, Return: ret}, nil
		})

	out := make([]RoundTrip, 0, len(trips))
	var firstErr error
	for i, tr := range trips {
		if errs[i] != nil {
			if firstErr == nil {
				firstErr = errs[i]
			}
			continue
		}
		out = append(out, tr)
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// returnOptions runs one phase-2 request: the same round-trip body as phase 1
// with the chosen outbound pinned into the outbound segment's segment[8]. A
// genuinely empty return set is reported as no error and a nil slice.
func (c *Client) returnOptions(ctx context.Context, req SearchRequest, outbound Itinerary) ([]Itinerary, error) {
	sel := make([]encoding.FreqSelectedLeg, 0, len(outbound.Segments))
	for _, s := range outbound.Segments {
		sel = append(sel, encoding.FreqSelectedLeg{
			Origin:       s.Origin.Code,
			Dest:         s.Destination.Code,
			Date:         s.DepartureTime,
			Carrier:      s.Carrier,
			FlightNumber: s.FlightNumber,
		})
	}

	freqReq := c.buildFreq(req)
	freqReq.Segments[0].Selected = sel

	res, err := c.executeFreq(ctx, req, freqReq)
	if errors.Is(err, ErrNoResults) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return res.Itineraries, nil
}

// executeFreq encodes freqReq, POSTs it, and decodes the shopping response —
// the shared body of every GetShoppingResults call, one-way or round-trip.
func (c *Client) executeFreq(ctx context.Context, req SearchRequest, freqReq encoding.FreqRequest) (SearchResult, error) {
	body, err := encoding.EncodeFreq(freqReq)
	if err != nil {
		return SearchResult{}, fmt.Errorf("gflight: encode request: %w", err)
	}

	currency := c.currency
	if req.Currency != "" {
		currency = strings.ToUpper(req.Currency)
	}

	endpoint, err := c.rpcEndpoint(currency)
	if err != nil {
		return SearchResult{}, err
	}

	raw, err := c.post(ctx, endpoint, "f.req="+body)
	if err != nil {
		var be *BlockedError
		if errors.As(err, &be) && be.DeepLink == "" {
			be.DeepLink = deepLink(req)
		}
		return SearchResult{}, err
	}

	payloads, err := wire.Payloads(raw)
	if err != nil {
		var se *wire.StatusError
		if errors.As(err, &se) {
			return SearchResult{}, &BlockedError{StatusCode: se.Code, DeepLink: deepLink(req)}
		}
		if errors.Is(err, wire.ErrNoEnvelope) {
			return SearchResult{}, &BlockedError{DeepLink: deepLink(req)}
		}
		return SearchResult{}, fmt.Errorf("gflight: read response: %w", errors.Join(err, ErrBadResponse))
	}

	var (
		flights   []decode.Flight
		sessionID string
	)
	for _, p := range payloads {
		var inner any
		if err := json.Unmarshal(p, &inner); err != nil {
			return SearchResult{}, fmt.Errorf("gflight: decode payload: %w", errors.Join(err, ErrBadResponse))
		}
		if sid := decode.SessionID(inner); sid != "" {
			sessionID = sid
		}
		fs, stats, err := decode.Flights(inner)
		c.observer.parse(ParseEvent{Rows: stats.Rows, Failures: stats.Failures})
		if err != nil {
			var allFailed *decode.AllRowsFailedError
			if errors.As(err, &allFailed) {
				return SearchResult{}, fmt.Errorf("gflight: %s: %w", allFailed.Error(), ErrUpstreamChanged)
			}
			if errors.Is(err, decode.ErrShapeChanged) {
				return SearchResult{}, fmt.Errorf("gflight: %w", ErrUpstreamChanged)
			}
			return SearchResult{}, fmt.Errorf("gflight: decode flights: %w", errors.Join(err, ErrBadResponse))
		}
		flights = append(flights, fs...)
	}

	if sessionID != "" {
		c.mu.Lock()
		c.lastSessionID = sessionID
		c.mu.Unlock()
	}

	if len(flights) == 0 {
		return SearchResult{}, ErrNoResults
	}

	out := make([]Itinerary, 0, len(flights))
	for _, f := range flights {
		out = append(out, toItinerary(f, currency))
	}
	return SearchResult{Itineraries: out, SessionID: sessionID}, nil
}

func (c *Client) buildFreq(req SearchRequest) encoding.FreqRequest {
	seg := segmentFilters(req)
	seg.Origin = req.Origin
	seg.Dest = req.Destination
	seg.Date = req.DepartDate

	fr := encoding.FreqRequest{
		Segments:            []encoding.FreqSegment{seg},
		Adults:              req.Adults,
		Children:            req.Children,
		InfantsOnLap:        req.InfantsOnLap,
		InfantsInSeat:       req.InfantsInSeat,
		Cabin:               cabinToFreq(req.Cabin),
		SortBy:              sortToFreq(req.SortBy),
		MaxPrice:            req.MaxPrice,
		CheckedBags:         req.CheckedBags,
		CarryOnBags:         req.CarryOnBags,
		ExcludeBasicEconomy: req.ExcludeBasicEconomy,
	}
	if !req.ReturnDate.IsZero() {
		ret := segmentFilters(req)
		ret.Origin = req.Destination
		ret.Dest = req.Origin
		ret.Date = req.ReturnDate
		ret.IsReturn = true
		fr.Segments = append(fr.Segments, ret)
	}
	return fr
}

// segmentFilters projects the per-segment filters of a request; they apply
// identically to the outbound and the return leg.
func segmentFilters(req SearchRequest) encoding.FreqSegment {
	return encoding.FreqSegment{
		MaxStops:          req.MaxStops,
		IncludeAirlines:   req.IncludeAirlines,
		ExcludeAirlines:   req.ExcludeAirlines,
		MaxDurationMins:   int(req.MaxDuration.Minutes()),
		LayoverAirports:   req.LayoverAirports,
		MinLayoverMins:    int(req.MinLayover.Minutes()),
		MaxLayoverMins:    int(req.MaxLayover.Minutes()),
		DepEarliestHour:   req.DepartureWindow.EarliestHour,
		DepLatestHour:     req.DepartureWindow.LatestHour,
		ArrEarliestHour:   req.ArrivalWindow.EarliestHour,
		ArrLatestHour:     req.ArrivalWindow.LatestHour,
		LessEmissionsOnly: req.LessEmissionsOnly,
	}
}

func sortToFreq(s SortOrder) int {
	switch s {
	case SortCheapest:
		return encoding.SortCheapest
	case SortDepartureTime:
		return encoding.SortDepartureTime
	case SortArrivalTime:
		return encoding.SortArrivalTime
	case SortDuration:
		return encoding.SortDuration
	default:
		return encoding.SortBest
	}
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
		be := &BlockedError{StatusCode: resp.StatusCode}
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			be.RetryAfter = d
		}
		return nil, be
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
			Carrier:               leg.Carrier,
			OperatingCarrier:      leg.OperatingCarrier,
			FlightNumber:          leg.FlightNumber,
			OperatingFlightNumber: leg.OperatingFlightNumber,
			Origin:                airport(leg.Origin),
			Destination:           airport(leg.Dest),
			DepartureTime:         leg.Departure,
			ArrivalTime:           leg.Arrival,
			Duration:              time.Duration(leg.DurationMin) * time.Minute,
			Aircraft:              leg.Aircraft,
			Legroom:               leg.Legroom,
			Overnight:             leg.Overnight,
			CO2Grams:              leg.CO2Grams,
			Amenities: Amenities{
				WiFi:           leg.Amenities.Wifi,
				Power:          leg.Amenities.Power,
				OnDemandVideo:  leg.Amenities.OnDemandVideo,
				LegroomRating:  leg.Amenities.LegroomRating,
				ACPower:        leg.Amenities.ACPower,
				USBPower:       leg.Amenities.USBPower,
				StreamingVideo: leg.Amenities.StreamingVideo,
				InSeatVideo:    leg.Amenities.InSeatVideo,
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

// deepLink returns the browser URL reproducing req, or "" if one can't be
// built — used to enrich a [BlockedError] so a blocked caller can fall back to
// opening the search in a browser.
func deepLink(req SearchRequest) string {
	u, err := SearchURL(req)
	if err != nil {
		return ""
	}
	return u
}

func airport(a decode.Airport) Airport {
	return Airport{Code: a.Code, Name: a.Name, City: a.City}
}
