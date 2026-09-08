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

> **Status:** one-way `Client.Search` works end to end against Google's
> undocumented RPC, driven by recorded fixtures in the tests. Round-trip is
> best-effort and unverified until the two-phase selected-flight flow lands;
> price calendars, booking options, and the full filter set are still to come.
>
> Google fingerprints TLS clients — the stock `net/http` transport is often met
> with `ErrBlocked`. Plug a browser-grade transport into `WithHTTPClient` when
> that happens.

## Install

```sh
go get github.com/khoi-truong/gflight
```

## Quickstart

The snippet below is [`examples/search/main.go`](examples/search/main.go), so
CI proves it compiles.

```go
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
  price := fmt.Sprintf("%.0f %s", it.Price.Amount, it.Price.Currency)
  if it.Price.Unknown {
   price = "price on request"
  }
  fmt.Printf("%s · %d stop(s) · %s\n", price, it.Stops, it.Duration)
 }
 return nil
}
```

Run it with `mise run example`. Because Google often blocks non-browser TLS,
the example honours `GFLIGHT_BASE_URL` so it can be pointed at a local
recording; production callers supply a browser-grade transport via
`WithHTTPClient`.

Locale defaults to `USD` / `en` / `US`; override with `WithCurrency`,
`WithLanguage`, `WithCountry`, or per call with `SearchRequest.Currency`.
`SearchURL(req)` returns a shareable google.com/travel/flights deep link
without any network I/O.

## Errors

Compare with `errors.Is` against `ErrBadResponse`, `ErrNoResults`, `ErrBlocked`
(429 or a bot wall), and `ErrUpstreamChanged` (every response row failed to
decode — Google moved the wire format). A non-2xx reply carries an `*HTTPError`
reachable with `errors.As` that unwraps to `ErrBadResponse`. Never match on
message text.

## Acknowledgements

The reverse-engineered protocol knowledge this client depends on is not ours —
see [`docs/ACKNOWLEDGEMENTS.md`](docs/ACKNOWLEDGEMENTS.md).

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
