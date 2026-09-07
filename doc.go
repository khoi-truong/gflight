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
// The client is a scaffold: [Client.Search] currently returns
// [ErrNotImplemented]. The exported types and the context-first signatures are
// stable enough to build against; the transport internals are not.
//
// # Errors
//
// All failures are reported through the sentinels in errors.go — compare with
// [errors.Is] rather than matching on message text. Transport-level failures
// carry an [*HTTPError], reachable with [errors.As].
package gflight
