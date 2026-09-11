# gflight

[![Go Reference](https://pkg.go.dev/badge/github.com/khoi-truong/gflight.svg)](https://pkg.go.dev/github.com/khoi-truong/gflight)
[![Tests](https://github.com/khoi-truong/gflight/actions/workflows/test.yml/badge.svg)](https://github.com/khoi-truong/gflight/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/khoi-truong/gflight)](https://goreportcard.com/report/github.com/khoi-truong/gflight)

A small Go client for searching flights on Google Flights — context-first,
near-stdlib-only, and designed to be embedded in a backend rather than driven
from a terminal.

## Disclaimer

**This is an unofficial, scrape-based client.** It is not affiliated with,
endorsed by, or supported by Google. It talks to undocumented endpoints that
can change or disappear at any time, which will break this library without
notice. You are responsible for your own use of it, including compliance with
Google's Terms of Service and any applicable rate limits. Use at your own risk.

> **Status:** one-way `Client.Search` and two-phase round-trip
> `Client.RoundTripTopN` work end to end against Google's undocumented RPC,
> driven by recorded fixtures in the tests. The round-trip `segment[8]` shape is
> structurally derived, not browser-captured. `SearchRequest` exposes the full
> filter set — sort order, infants, airline/alliance include+exclude, max price,
> bags, max trip duration, layover airports and min/max layover, departure and
> arrival hour windows, less-emissions-only, exclude-basic-economy — each inert
> at its zero value and each with a structurally derived wire shape.
>
> Search is the only reachable surface. Google gates every other
> `FlightsFrontendService` method — vendor fares and price calendars among them —
> behind a BotGuard token minted by in-page JavaScript, which no Go client can
> produce. The one method that answers an anonymous caller is
> `GetShoppingResultsPrefetch`, and this library rides on it alone: if Google
> ever gates that rpc id too, nothing here works. See
> [`docs/wire/botguard.md`](docs/wire/botguard.md).
>
> `WithRetry` handles transient 429/5xx; `WithTransport` slots in a custom
> `http.RoundTripper`. Neither defeats the BotGuard gate — a browser-grade TLS
> fingerprint was tested and makes no difference.

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

For round trips, set `ReturnDate` and call `RoundTripTopN(ctx, req, n)`: it
searches outbound options, then re-queries Google once per top-`n` outbound to
get returns priced against it (bounded by `WithMaxConcurrency`). Each
`RoundTrip` holds the `Outbound` itinerary and its `Return` list; return prices
are trip totals.

Some itineraries come back with `Price.Unknown` set — Google returned the
journey but no shopping-list price, which is common for premium-cabin round
trips. Treat those as unpriced, not free: Google prices them only in its own UI,
behind a method this library cannot call.

Locale defaults to `USD` / `en` / `US`; override with `WithCurrency`,
`WithLanguage`, `WithCountry`, or per call with `SearchRequest.Currency`.
`SearchURL(req)` returns a shareable google.com/travel/flights deep link
without any network I/O.

## Errors

Compare with `errors.Is` against `ErrBadResponse`, `ErrNoResults`, `ErrBlocked`
(429 or a bot wall), and `ErrUpstreamChanged` (every response row failed to
decode — Google moved the wire format). A non-2xx reply carries an `*HTTPError`
reachable with `errors.As` that unwraps to `ErrBadResponse`. A block carries a
`*BlockedError` (unwraps to `ErrBlocked`) with the `RetryAfter` delay and a
`DeepLink` browser-fallback URL. Never match on message text.

## Resilience

`WithRetry(RetryPolicy{MaxAttempts: 4})` installs a retrying transport —
connection errors and 408/425/429/500/502/503/504 are retried with full-jitter
exponential backoff, and a `Retry-After` header overrides the computed wait.
`WithTransport` sets the base `http.RoundTripper` for slotting in a uTLS or
proxy stack without replacing the whole `http.Client` — recipe in
[`examples/utls`](examples/utls/README.md).
`WithRateLimit(rps, burst)` paces every request through a token bucket —
retries included, since the limiter sits below retry. A request that cannot be
sent before its context deadline fails with the context error instead of
queueing. Google publishes no quota; comparable clients settle around 10 req/s.

All three are off by default. Rate limiting is the library's one dependency,
`golang.org/x/time/rate`; nothing else is imported outside the stdlib.

## Observability

`WithObserver(&gflight.Observer{...})` hands retries, per-attempt responses, and
per-payload decode counts to callbacks you supply, so Prometheus or OTel wire up
without this library importing either. Every callback is optional and runs
inline on the goroutine that produced the event, so keep them non-blocking.

```go
client := gflight.New(gflight.WithObserver(&gflight.Observer{
    OnResponse: func(e gflight.ResponseEvent) {
        requests.WithLabelValues(strconv.Itoa(e.StatusCode)).Observe(e.Duration.Seconds())
    },
    OnParse: func(e gflight.ParseEvent) {
        // Rising failures mean Google moved the wire format.
        rowFailures.Add(float64(e.Failures))
    },
}))
```

`ParseEvent.Failures` is the drift canary — it climbs before decoding breaks
outright and the client starts returning `ErrUpstreamChanged`. The same signal
runs daily in CI as a live smoke test (`mise run test:live`, behind the `live`
build tag and excluded from `mise run ci`).

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
