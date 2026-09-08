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
// One-way [Client.Search] works end to end against Google's undocumented
// FlightsFrontendService RPC, driven by recorded fixtures in the tests.
// Round-trip search is best-effort and unverified: setting
// [SearchRequest.ReturnDate] adds a return segment to a single request, but the
// two-phase selected-flight flow and round-trip price semantics are not yet
// implemented or fixture-tested. Calendar graphs, booking options, and the full
// filter set are also still to come. The undocumented upstream can change shape
// without notice; a response whose rows no longer decode is reported as
// [ErrUpstreamChanged].
//
// Google fingerprints TLS clients: the default net/http transport may be met
// with [ErrBlocked]. Plug a browser-grade transport into [WithHTTPClient] if so.
//
// # Errors
//
// All failures are reported through the sentinels in errors.go — compare with
// [errors.Is] rather than matching on message text. Transport-level failures
// carry an [*HTTPError], reachable with [errors.As].
package gflight
