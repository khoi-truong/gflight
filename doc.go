// Package gflight is an unofficial Go client for Google Flights.
//
// It talks to the same undocumented endpoints the Google Flights web UI uses.
// It is not affiliated with, endorsed by, or supported by Google, and the
// upstream surface can change or break at any time without notice. Use it at
// your own risk and respect Google's Terms of Service.
//
// # Usage
//
//	client := gflight.New()
//
//	itineraries, err := client.Search(ctx, gflight.SearchRequest{
//		Origin:      "SGN",
//		Destination: "HAN",
//		DepartDate:  time.Now().AddDate(0, 1, 0),
//		Adults:      1,
//		Currency:    "VND",
//	})
//	if err != nil {
//		return err
//	}
//	for _, it := range itineraries {
//		fmt.Println(it.Price.Amount, it.Price.Currency)
//	}
//
// # Status
//
// One-way [Client.Search] and two-phase round-trip [Client.RoundTripTopN] work
// end to end against Google's undocumented FlightsFrontendService RPC, driven by
// recorded fixtures in the tests. Round-trip runs a phase-1 outbound search then
// re-queries per selected outbound (segment[8]); its return prices are trip
// totals. The exact segment[8] shape is structurally derived, not captured from
// a live browser session — see docs/plans/porting.md.
//
// [SearchRequest] carries the full filter set: sort order, cabin, passengers
// (including infants), airline or alliance include/exclude, max price, bag
// counts, max trip duration, layover airports and min/max layover, departure
// and arrival hour windows, less-emissions-only and exclude-basic-economy.
// Every filter is inert at its zero value. Their wire shapes are structurally
// derived rather than browser-captured — see docs/wire/shopping-results.md.
//
// Only the search surface is reachable. Google gates its other
// FlightsFrontendService methods — vendor fares and the price calendar among
// them — behind a BotGuard token that no Go client can mint. See
// docs/wire/botguard.md.
//
// The undocumented upstream can change shape without notice; a response whose
// rows no longer decode is reported as [ErrUpstreamChanged].
//
// A rate-limited or walled reply is a [*BlockedError] (which unwraps to
// [ErrBlocked] and carries the Retry-After delay and a [SearchURL] deep-link
// fallback). Enable [WithRetry] for transient 429/5xx with full-jitter backoff
// that honours Retry-After, and pace requests with [WithRateLimit] to stay
// under the (unpublished) upstream quota. [WithTransport] swaps the base
// RoundTripper for a proxy or custom TLS stack (recipe in examples/utls); note
// that a browser-grade TLS fingerprint was tested and does not affect the
// BotGuard gate.
//
// # Observability
//
// [WithObserver] reports retries, per-attempt responses, and per-payload decode
// counts to callbacks a consumer supplies, so metrics or tracing wire up
// without this library importing any telemetry package. The [ParseEvent]
// failure count is the drift canary: it rises before decoding fails outright.
//
// # Errors
//
// All failures are reported through the sentinels in errors.go — compare with
// [errors.Is] rather than matching on message text. Transport-level failures
// carry an [*HTTPError], reachable with [errors.As].
package gflight
