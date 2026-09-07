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

	client := gflight.New()

	//nolint:staticcheck // SA4023: Search is a stub that always fails today; the check below is real once it is implemented.
	itineraries, err := client.Search(ctx, gflight.SearchRequest{
		Origin:      "SGN",
		Destination: "HAN",
		DepartDate:  time.Now().AddDate(0, 1, 0),
		Adults:      1,
		Cabin:       gflight.CabinEconomy,
		Currency:    "VND",
	})
	//nolint:staticcheck // SA4023: Search is a stub that always fails today; the check is real once it is implemented.
	if err != nil {
		return err
	}

	for _, it := range itineraries {
		fmt.Printf("%.0f %s · %d stop(s) · %s\n", it.Price.Amount, it.Price.Currency, it.Stops, it.Duration)
	}
	return nil
}
