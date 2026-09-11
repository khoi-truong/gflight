// Command search is a minimal consumer of the gflight client. It doubles as
// the README quickstart, so CI compiling it proves the snippet still builds.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/khoi-truong/gflight"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "search failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var opts []gflight.Option
	// The live endpoint can answer with ErrBlocked. Point GFLIGHT_BASE_URL at
	// a recorded httptest server (see search_test.go) to run this offline.
	if base := os.Getenv("GFLIGHT_BASE_URL"); base != "" {
		opts = append(opts, gflight.WithBaseURL(base))
	}
	client := gflight.New(opts...)

	itineraries, err := client.Search(ctx, gflight.SearchRequest{
		Origin:      "SGN",
		Destination: "HAN",
		DepartDate:  time.Now().AddDate(0, 1, 0),
		Adults:      1,
		Cabin:       gflight.CabinEconomy,
		Currency:    "VND",
	})
	if err != nil {
		return err
	}

	for _, it := range itineraries {
		price := fmt.Sprintf("%.0f %s", it.Price.Amount, it.Price.Currency)
		if it.Price.Unknown {
			price = "price on request"
		}
		fmt.Printf("%s · %d stop(s) · %s\n", price, it.Stops, it.Duration)
	}
	return nil
}
