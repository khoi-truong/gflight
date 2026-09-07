# gflight

[![Go Reference](https://pkg.go.dev/badge/github.com/khoi-truong/gflight.svg)](https://pkg.go.dev/github.com/khoi-truong/gflight)
[![Tests](https://github.com/khoi-truong/gflight/actions/workflows/test.yml/badge.svg)](https://github.com/khoi-truong/gflight/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/khoi-truong/gflight)](https://goreportcard.com/report/github.com/khoi-truong/gflight)

A small Go client for searching flights on Google Flights — context-first,
stdlib-only, and designed to be embedded in a backend rather than driven from a
terminal.

## Disclaimer

**This is an unofficial, scrape-based client.** It is not affiliated with,
endorsed by, or supported by Google. It talks to undocumented endpoints that
can change or disappear at any time, which will break this library without
notice. You are responsible for your own use of it, including compliance with
Google's Terms of Service and any applicable rate limits. Use at your own risk.

> **Status:** scaffold. `Client.Search` currently returns `ErrNotImplemented` —
> the API shape is settled, the transport is not.

## Install

```sh
go get github.com/khoi-truong/gflight
```

## Quickstart

The snippet below is [`examples/search/main.go`](examples/search/main.go), so
CI proves it compiles.

```go
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
		fmt.Printf("%.0f %s · %d stop(s) · %s
", it.Price.Amount, it.Price.Currency, it.Stops, it.Duration)
	}
	return nil
}
```

Run it with `mise run example`.

## Errors

Compare with `errors.Is` against `ErrNotImplemented`, `ErrBadResponse`, and
`ErrNoResults`; a non-2xx reply carries an `*HTTPError` reachable with
`errors.As` that unwraps to `ErrBadResponse`. Never match on message text.

## Stability

Pre-1.0: minor version bumps may break the API. Pin an exact version.
Changes are additive-first — new fields and new `Option`s, never repurposed
ones.

## Development

```sh
mise install     # provision the toolchain (or: curl https://mise.run | sh)
mise run setup   # tools + git hooks
mise run ci      # tidy → vet → lint → lint:actions → test → vuln
```

`mise` is the single source of truth for the toolchain and tasks; `mise tasks`
lists them all. See [`AGENTS.md`](AGENTS.md) for conventions.

## License

MIT — see [LICENSE](LICENSE).
