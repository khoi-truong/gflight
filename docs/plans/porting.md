# Plan: port a real Google Flights client into `gflight`

**Status:** in progress (M1–M3 done, M5a done; M4 and M5b–M5c pending) · **Module:** `github.com/khoi-truong/gflight` · Go 1.26 · MIT
**Supersedes nothing.** Follows `docs/plans/scaffold.md` (commit `3e2f6ae`).
**Amended 2026-09-08** — see [Amendments](#amendments-2026-09-08-deep-analysis-review)
after a deep-analysis review against `fli`, `krisukox`, `fast-flights`, and
current Go + resilience best practices.

---

## Context

`gflight` today is a scaffold. `Client.Search` returns `ErrNotImplemented`,
`internal/encoding` is a `doc.go` with no code, and `internal/testdata/` holds a
`.gitkeep`. The public signatures are frozen and `mise run ci` is green — the
spine exists, the body does not.

This plan fills it in by porting the _approach_ (not the code) of the best
existing reverse-engineered clients, so we inherit years of upstream
archaeology instead of rediscovering it. The intended outcome: a real
`Search(ctx, SearchRequest) ([]Itinerary, error)` backed by Google's
`FlightsFrontendService` RPC, driven entirely by recorded fixtures in tests, with
`go.sum` still empty.

### Research: what we learn from whom

| Lib                                                                                                     | Lang/licence | Approach                                                                        | What we take                                                                                                                                                                      |
| ------------------------------------------------------------------------------------------------------- | ------------ | ------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [punitarani/fli](https://github.com/punitarani/fli) — 3.1k★, MIT, active                                | Python       | RPC `GetShoppingResults` / `GetCalendarGraph` / `GetBookingResults` via `f.req` | **The architecture.** Wire framing, `f.req` index map, row decoder layout, hand-rolled protobuf (no runtime dep), `.bin` snapshot fixtures, round-trip expansion via `segment[8]` |
| [krisukox/google-flights-api](https://github.com/krisukox/google-flights-api) — 69★, MIT, stale 2025-02 | Go           | `tfs` protobuf URL + HTML scrape                                                | Go ergonomics prior art; the `tfs` proto field numbers; a warning — HTML scraping and the `GOOGLE_ABUSE_EXEMPTION` cookie dependency are why we don't copy it                     |
| [AWeirdDev/flights](https://github.com/AWeirdDev/flights) — 2.0k★, MIT                                  | Python       | `tfs` + HTML parse                                                              | The cleanest `flights.proto` for the `tfs` deep-link parameter                                                                                                                    |
| [MomoDeve tfs gist](https://gist.github.com/MomoDeve/a18053dea84dd28e320b8b2c489540eb)                  | —            | spec                                                                            | Authoritative `tfs` field-number table incl. fields krisukox omits                                                                                                                |
| [ayushsaraswat.com writeup](https://ayushsaraswat.com/writing/reverse-engineering-google-flights/)      | —            | prose                                                                           | Response index gotchas; anti-bot context                                                                                                                                          |

All three libs are MIT. We port design, not source — see **Attribution** below.

### Decisions (answered by the user, 2026-09-08)

1. **RPC only.** No HTML parsing anywhere. Plus a pure, network-free `tfs`
   deep-link URL builder as a public helper.
2. **Stdlib only.** Hand-rolled protobuf encoder in `internal/encoding`
   (~120 lines: varint, length-delimited, nested). `go.sum` stays empty. No
   `google.golang.org/protobuf`, no uTLS — consumers needing a Chrome TLS
   fingerprint plug their own transport into the existing `WithHTTPClient`.
3. **Rich public types**, ported from fli's model — layovers, CO₂, amenities,
   aircraft, legroom, operating carrier, self-transfer, mixed-cabin, booking
   token. Additive-first, existing field names preserved.
4. **Milestone 1 = one-way search end-to-end.** Round-trip, calendar graph,
   booking options, and rich filters follow in M2–M4.

---

## The upstream surface (established by research, not assumption)

**Request** — `POST` to

```text
https://www.google.com/_/FlightsFrontendUi/data/travel.frontend.flights.FlightsFrontendService/GetShoppingResults
```

`Content-Type: application/x-www-form-urlencoded;charset=UTF-8`, body
`f.req=<percent-encoded JSON>`. Locale rides the query string: `curr=`, `hl=`,
`gl=` (currency **must** be uppercase — Google silently ignores lowercase).

The `f.req` value is a double-encoded nested array: build the filter array,
`json.Marshal` it to a _string_, wrap that string as `[null, "<that string>"]`,
marshal again, percent-encode.

```text
outer[0] = []
outer[1] = main settings          outer[2] = sort mode
outer[3] = 1 (all results)        outer[4] = 0        outer[5] = 1

main[2]  trip type (1=RT, 2=OW, 3=MC)      main[5]  cabin (1..4)
main[6]  [adults, children, inf_lap, inf_seat]
main[7]  [null, max_price]                 main[10] [checked_bags, carry_on]
main[13] segments                          main[17] 1 (constant)
main[28] exclude basic economy (0|1)       — all other indices null

segment[0]  [[[IATA, 0]]] departure   (exactly 3 levels — wrong depth = 0 results, no error)
segment[1]  [[[IATA, 0]]] arrival
segment[2]  [edep, ldep, earr, larr] hour buckets
segment[3]  max stops   segment[4] airline/alliance include   segment[5] exclude
segment[6]  "YYYY-MM-DD"                   segment[7]  [max_duration_mins]
segment[8]  selected_flight (round-trip 2nd phase)
segment[9]  layover airport include        segment[11]/[12] min/max layover mins
segment[13] [1] less-emissions             segment[14] classifier: 3=outbound, 1=return
```

**Response** — a chunked, JSONP-flavoured envelope:

```text
)]}'\n\n
<byte_len>\n
[["wrb.fr", null, "<inner JSON string>"]]
<byte_len>\n
[["wrb.fr", null, "<inner JSON string>"]]
```

Two traps, both learned the hard way upstream: the length header counts **UTF-8
bytes**, not runes — so the reader works over `[]byte`; and `GetShoppingResults`
happens to emit one chunk today while `GetBookingResults` emits two, so a
`TrimPrefix(")]}'"), json.Unmarshal` shortcut will break on the next endpoint.
Write the multi-chunk reader once, correctly.

Inside a chunk: `inner[0][4]` is the shopping session id; flight rows are the
concatenation of `inner[2][0]` and `inner[3][0]`.

```text
row[1]     price block ([[], "<token>"] means "no price" — NOT zero)
row[8]     booking token          row[10] mixed cabin
detail = row[0]
  detail[2]  legs      detail[9]  total duration   detail[12] self-transfer
  detail[13] layover names/cities  detail[22] emissions block
leg[3]/[6] airports    leg[4]/[5] airport names    leg[8]/[10] dep/arr time (h, m)
leg[11] duration       leg[12] 12-slot amenities   leg[17] aircraft
leg[19] overnight      leg[20]/[21] dep/arr date   leg[22] [airline, flight_no, op_airline]
leg[30] legroom        leg[31] CO2 g
```

`(hour, minute)` tuples are inconsistently shaped — `(h, m)`, `(h,)`, or
`(null, m)` — so the time parser must tolerate all three.

**Deep-link `tfs`** — base64url (no padding) protobuf, wire-tag order:
`f1=28`, `f2=2`, `f3` repeated segment, `f8/f9/f14=1`, `f16{f1=MaxUint64}`,
`f19` = 2 one-way / 1 round-trip. Per segment: `f2` date, repeated `f4` legs
(`f1` origin, `f2` date, `f3` dest, `f5` airline, `f6` flight no),
`f13`/`f14` = `{f1=1, f2=IATA}` origin/dest.

---

## Target package map

```text
doc.go            unchanged shape; status paragraph updated per milestone
client.go         Client gains locale fields (currency, language, country)
options.go        + WithCurrency, WithLanguage, WithCountry, WithRetry
types.go          widened: Itinerary/Segment/Layover/Amenities/Emissions
search.go         Client.Search — real implementation
url.go            NEW · SearchURL(SearchRequest) (string, error) — pure, no network
errors.go         + ErrBlocked (429/consent wall), ErrUpstreamChanged
internal/encoding/
  protobuf.go     NEW · varint / tag / lengthDelim / nested writers
  tfs.go          NEW · SearchRequest -> tfs base64url token
  freq.go         NEW · SearchRequest -> f.req body string
internal/wire/
  chunks.go       NEW · ")]}'" + byte-length wrb.fr chunk reader
internal/decode/
  flight.go       NEW · row -> Itinerary; index constants named, never inline
internal/testdata/ recorded, scrubbed fixtures
examples/search/  unchanged; the //nolint:staticcheck pair is removed in M1
```

Rationale for the three `internal/` packages rather than one: encoding, framing,
and decoding fail independently and are testable independently. `internal/wire`
in particular is pure `[]byte -> []json.RawMessage` and gets fuzzed.

---

## Milestones

Each milestone is one PR with a conventional-commit title, squash-merged.
`mise run ci` green is the definition of done for every one.

### M0 — plan scaffolding · `docs: add porting plan and docs/plans convention`

**Do this first, before any code.** Docs only, no Go files touched.

- `docs/plans/porting.md` — this plan verbatim, committed as the durable
  record. Status line: `in progress`.
- `docs/plans/README.md` — the index table (see **Plan file management**),
  seeded with `scaffold.md` (implemented, `3e2f6ae`) and `porting.md`.
- `AGENTS.md` — three lines under "Repo-wide" pointing at the convention:
  plans in `docs/plans/`, wire notes in `docs/wire/`, `.omc/` stays scratch.
- Move nothing; `scaffold.md` stays as-is with its `Status: implemented`.
- `docs/wire/` is created empty-of-content until M1 populates it — do not add
  placeholder files.

### M1 — one-way search end-to-end · `feat: implement one-way flight search`

- [x] **`internal/encoding/protobuf.go`** — `appendVarint`, `appendTag`,
   `appendVarintField`, `appendLengthDelim`, plus `parseMessage` for round-trip
   tests. Table-driven varint / message tests.
- [x] **`internal/encoding/tfs.go`** — builds the `tfs` token per the spec.
   Golden test freezes a structurally round-trip-verified token (see deviations
   — no live byte capture available).
- [x] **`url.go`** — public `SearchURL(req SearchRequest) (string, error)`,
   network-free, unit-tested.
- [x] **`internal/encoding/freq.go`** — the `f.req` builder, index map as named
   constants, every inert slot commented.
- [x] **`internal/wire/chunks.go`** — byte-accurate reader for both the bare
   single-chunk and length-prefixed multi-chunk shapes, with `FuzzChunks`.
- [x] **`internal/decode/`** (`accessors.go`, `currency.go`, `flight.go`) —
   row → `Flight` via defensive `at` / `asStr` / `asInt` / `asBool` accessors. A
   bad row is skipped; if every row fails, `AllRowsFailedError` (≤3 sample
   reasons) → `search.go` maps it to `ErrUpstreamChanged` wrapping
   `ErrBadResponse`.
- [x] **Widen `types.go`** — `Layover`, `Amenities`, `Emissions`;
   `Itinerary.BookingToken` / `.Layovers` / `.Emissions` / `.SelfTransfer` /
   `.MixedCabin` / `.PrimaryCarrier`; `Segment.Legroom` / `.OperatingCarrier` /
   `.Amenities` / `.Overnight` / `.CO2Grams`. `Price` gains `Unknown bool`.
- [x] **`search.go`** — build `f.req`, POST, read chunks, decode rows, capture
   the session id on the `Client` (`SessionID()`). Non-2xx → `*HTTPError`;
   429 / bot wall / non-envelope body → `ErrBlocked`; zero rows → `ErrNoResults`.
- [x] **Fixtures + tests** — `httptest.Server` via `WithBaseURL`. Cases:
   `oneway_jfk_lax`, `no_price`, `no_results`, `error_429`, `truncated_chunk`,
   `multichunk`, `layover_buf_ath`. The two `//nolint:staticcheck` in
   `examples/search/main.go` are removed.
- [x] **Docs** — `doc.go` status paragraph, README quickstart, disclaimer
   intact, `docs/ACKNOWLEDGEMENTS.md`, `docs/wire/shopping-results.md`,
   `docs/wire/tfs.md`.

### Implementation deviations from the plan

- **No live verification (plan Verification step 6, Risk #1).** Google answers
  this RPC approach with an upstream `[13]` status for every non-browser TLS
  client — `fli` included. M1 therefore ships fixture-driven, with `ErrBlocked`
  making the live failure legible and a `WithHTTPClient` recipe in the docs. The
  one live manual check is deferred until a browser-grade transport is wired.
- **`tfs` golden is synthetic.** `tfs_test.go` freezes a token verified by
  decoding it back to the expected protobuf structure and by matching `fli`'s
  `build_tfs_token` algorithm byte for byte — not by comparison against a
  browser-captured URL, which needs a reachable endpoint.
- **Fixture names.** `oneway_jfk_lax` (not `oneway_sgn_han`) and an extra
  `no_price` / `multichunk` / `layover_buf_ath` triplet, matching the recorded
  `fli` bodies actually available.
- **`internal/decode` is three files, not one** (`accessors.go` / `currency.go`
  / `flight.go`) — the currency-token protobuf walk and the position accessors
  are independently testable.
- **`ErrNotImplemented` retained.** Still exported (it shipped in `3e2f6ae`);
  removing it would break the additive-only contract. It is now unused by the
  package itself.
- **`examples/search/main.go` honours `GFLIGHT_BASE_URL`** so `mise run example`
  can run offline against a local recording (plan Verification step 5).
- **Round-trip shipped naive and undertested in M1.** `search.go` packs both
  segments into one `f.req` (`buildFreq` appends the return segment with
  `segment[14] = 1`). This is _not_ the two-phase `segment[8]` selected-flight
  flow M2 specifies, there is no round-trip `Search` test or fixture, and yet
  `doc.go` / `README.md` / `docs/plans/README.md` claim "one-way and round-trip
  work end to end". **Done (M1.1):** every doc claim (`doc.go`, `README.md`,
  `search.go`, `types.go`, `docs/plans/README.md`) downgraded to "one-way
  verified; round-trip best-effort and unverified until M2". The real two-phase
  flow still lands in M2. Tracked in
  [Amendments](#amendments-2026-09-08-deep-analysis-review).

### M2 — round-trip · `feat: support round-trip search`

Two-phase: search outbound, then for each of the top N outbounds re-POST with
`segment[8]` = selected flight and `segment[14]` = 1 on the return segment.
Needs a bounded-concurrency helper (`errgroup`-shaped, hand-rolled — stdlib
only) and a `WithMaxConcurrency` option (default 2–4). Round-trip prices are
**totals**, not per-leg — summing double-counts.

- [x] **`SearchResult` by value** — `SearchResults` returns
  `SearchResult{Itineraries, SessionID}`; `Client.SessionID()` kept as a
  `// Deprecated:` shim (landed in `bce7c32`).
- [x] **`mapConcurrent` + `WithMaxConcurrency`** — hand-rolled bounded fan-out
  helper (`fanout.go`), default `DefaultMaxConcurrency = 3` (landed in
  `a4e2ad9`/`bce7c32`).
- [x] **`internal/encoding` segment[8]** — `FreqSelectedLeg` +
  `FreqSegment.Selected`; `buildSelectedFlight` fills `segment[8]` on the
  outbound. Structural test + encoder golden (`goldenSelectedFlightRT`).
- [x] **Two-phase flow** — `search.go` splits `executeFreq` out of
  `SearchResults`; `RoundTripTopN(ctx, req, n)` runs phase 1 then fans phase 2
  out over `mapConcurrent`, propagating the caller's `ctx`. New public
  `RoundTrip{Outbound, Return}` type; `DefaultRoundTripTopN = 3`.
- [x] **Round-trip fixture + test** — `roundtrip_test.go`: `RoundTripTopN`
  behaviour, `n` clamping, request-assertion that phase-2 bodies carry the
  pinned outbound in `segment[8]` and phase-1 does not. Phase-2 responses reuse
  the shopping-results fixtures (identical wire shape).
- [x] **Docs** — `doc.go`, `README.md`, `types.go`, `search.go`,
  `docs/wire/shopping-results.md`, this plan.

### M2 implementation deviations

- **`segment[8]` shape is structurally derived, not live-captured.** Google's
  endpoint is unreachable for a browser-grade capture (same constraint as the
  `tfs` golden). `buildSelectedFlight` encodes each outbound leg as
  `[origin, "YYYY-MM-DD", dest, null, carrier, flight_number]` wrapped one level
  deep, matching `fli`'s selected-flight builder. Frozen in
  `goldenSelectedFlightRT`; revisit against a real capture when a browser
  transport lands.
- **No dedicated round-trip response fixture.** Phase-2 replies are
  GetShoppingResults rows — byte-identical in shape to phase 1 — so
  `roundtrip_test.go` reuses `shopping_results_oneway_jfk_lax.txt` for both
  phases rather than adding a near-duplicate fixture. The phase-2 *request* is
  asserted explicitly with a request capture.
- **`RoundTripTopN` returns `[]RoundTrip`, not a `RoundTripResult` wrapper.**
  The session id is still reachable via phase 1's `SearchResults` /
  `Client.SessionID()`; a value-typed round-trip result can be added additively
  in M4 if booking options need it.
- **Partial phase-2 failure is tolerated** — an outbound whose phase-2 call
  fails is dropped; the error surfaces only when every phase-2 call fails. A
  genuinely empty return set (`ErrNoResults`) yields a `RoundTrip` with a nil
  `Return`.

### M3 — filters · `feat: expose full search filters`

Airline/alliance include+exclude, max price, bags, max duration, layover
airports and min/max duration, departure/arrival hour windows, emissions,
exclude-basic-economy, sort mode. All additive on `SearchRequest`; each maps to a
named `f.req` index already documented in M1.

Fold in here (see [Amendments](#amendments-2026-09-08-deep-analysis-review) A4):

- [x] **Filter set** — `FreqRequest.MaxPrice` / `CheckedBags` / `CarryOnBags` /
  `ExcludeBasicEconomy` (`main[7]`, `main[10]`, `main[28]`) and
  `FreqSegment.IncludeAirlines` / `ExcludeAirlines` / `MaxDurationMins` /
  `LayoverAirports` / `MinLayoverMins` / `MaxLayoverMins` / the four hour-window
  fields / `LessEmissionsOnly` (`segment[4]`, `[5]`, `[7]`, `[9]`, `[11]`,
  `[12]`, `[2]`, `[13]`). Every slot goes through a named index constant; every
  zero value stays inert, so an unfiltered request encodes exactly as it did
  before M3.
- [x] **Sort mode** — public `SortOrder` (`SortBest` zero value, `SortCheapest`,
  `SortDepartureTime`, `SortArrivalTime`, `SortDuration`) on `SearchRequest`,
  mapped onto the existing `encoding.Sort*` constants by `sortToFreq`.
- [x] **Infant passengers** — `InfantsOnLap` / `InfantsInSeat` on
  `SearchRequest`, filling `main[6]` = `[adults, children, lap, seat]`.
- [x] **Amenities** — `ACPower` / `USBPower` / `StreamingVideo` / `InSeatVideo`
  added; `Power` and `OnDemandVideo` retained as the ORs of their pairs.
- [x] **Segment fields** — `Segment.OperatingFlightNumber` (`leg[22][4]`) and
  segment-level `Airport.City`, sourced from the `inner[1]` airport directory.
- [x] **Tests** — `TestEncodeFreqFilters` (13 filters × set/inert),
  `goldenAllFiltersOneWay` encoder golden, `TestSearchFiltersReachTheWire` /
  `TestSearchUnfilteredStaysInert` request captures, decode tests for the
  amenity split, airport city and operating flight number. Response goldens
  regenerated for the new fields.
- [x] **Docs** — `doc.go`, `README.md`, `types.go`,
  `docs/wire/shopping-results.md`, this plan.

### M3 implementation deviations

- **The filter slot shapes are structurally derived, not live-captured.**
  `segment[2]`, `[4]`, `[5]`, `[7]`, `[9]`, `[11]`, `[12]`, `[13]`, `main[7]`
  and `main[10]` follow the M1 index map and `fli`'s builder; the endpoint still
  refuses non-browser TLS clients, so none of them has been round-tripped
  against a real capture. Frozen in `goldenAllFiltersOneWay`.
- **The amenity split reuses slots 7/8, not 5/9.** No fixture ever sets slot 5,
  while 1, 8, 9 and 10 carry booleans. Rather than repurpose the M1 mapping,
  `[5]` and `[9]` keep their meaning (AC power, streaming video), `[7]`/`[8]`
  were added for USB power and in-seat video, and `Power` / `OnDemandVideo` are
  the logical ORs of each pair so pre-M3 callers see no behaviour change. The
  `[7]`/`[8]` assignment is structurally derived. Slot `[10]` is set in the
  fixtures but its meaning is still unidentified.
- **`OperatingFlightNumber` has no fixture coverage.** Every recorded
  `leg[22]` is `[code, number, null, name]` — no codeshare. It is mapped to
  `leg[22][4]`, bounds-checked so it decodes to `""` today, and pinned by a
  synthetic unit test that also proves index 3 (the airline name) is not
  borrowed by mistake.
- **`Airport.City` covers searched endpoints only.** `inner[1]` is an airport
  directory whose nesting depth varies by trip type, so `airportCities` walks it
  by shape with a depth cap instead of fixed indices. It lists only the searched
  origin and destination, so a connection airport's leg usually has an empty
  `City`; layover cities continue to come from `detail[13]`.
- **`TimeWindow` uses a plain int pair, not pointers.** A zero `LatestHour`
  means "no upper bound" rather than midnight, matching Google's own slider and
  keeping "zero value = no constraint" true for the whole struct.
- **Response goldens regenerated** with the documented `-update` flag to absorb
  `City`, `OperatingFlightNumber` and the four new amenity fields.

### M4 — calendar graph + booking options · `feat: add price calendar and booking options`

`GetCalendarGraph` (≤61 days per call, ≤305 days ahead — chunk and merge) and
`GetBookingResults` (needs the session-anchored booking token; `internal/encoding`
gains `bookingToken.go`). This is where the multi-chunk reader earns its keep.

### M5 — resilience · `chore: harden upstream resilience`

Pulled earlier than the original ordering implied — a 429/block-heavy upstream
needs this before it needs calendar graphs. See
[Amendments](#amendments-2026-09-08-deep-analysis-review) A2/A3/A5.

M5 ships as three PRs (see deviations): **M5a** retry/transport stack (stdlib
only), **M5b** rate limiting (adds `golang.org/x/time/rate`), **M5c**
observability + live canary.

- [x] **Compose behaviour as layered `http.RoundTripper`s in `New()`** (M5a) —
  `assembleTransport()` stacks retry → base (`WithTransport`, else the client's
  own transport) onto a private copy of the `http.Client`, so a caller's
  `WithHTTPClient` value is never mutated. The retry/backoff concern now lives in
  `transport.go`, unit-tested in isolation with `testing/synctest`.
- [x] **`WithRetry(RetryPolicy)`** (M5a) — hand-rolled stdlib retry
  RoundTripper. Full jitter (`rand(0, min(cap, base·2^(n-1)))`), `MaxAttempts`
  total tries (`< 2` disables), retry only connection errors +
  408/425/429/500/502/503/504. **Honours `Retry-After`** (delta-seconds and
  HTTP-date) for its backoff.
- [x] **`WithTransport(http.RoundTripper)`** (M5a) — next to `WithHTTPClient`,
  the single anti-bot extension seam; retry layers on top of it. Bundles nothing.
- [x] **`*BlockedError{StatusCode, RetryAfter, DeepLink}`** (M5a) — replaces the
  bare `ErrBlocked` return (still `Unwrap`s to it); carries the parsed
  `Retry-After` and the `SearchURL` deep link so a blocked caller degrades to
  "open in browser".
- **`WithRateLimit(rps, burst)`** (M5b) using `golang.org/x/time/rate` — the one
  near-stdlib dependency worth taking. `go.sum` stops being empty; note it in the
  README and the guardrails below. `fli`'s ceiling is ~10 req/s.
- **`WithMaxConcurrency(n)`** (semaphore) — shared with M2's fan-out helper,
  landed in M2.
- Document a uTLS recipe for `WithTransport` in `examples/` (M5c).
- **Observability hook** (M5c) — `WithObserver(o)` with `OnRetry` / `OnResponse`
  / `OnParse(rows, failures)` callbacks; consumers wire Prometheus/OTel without
  the library importing either. `OnParse` failure count is the drift canary.
- **Tests** — [x] `testing/synctest` backoff / `Retry-After` / deadline tests
  for the retry transport (M5a). Pending: a `decode(encode(x)) == x` fuzz for
  `internal/encoding` (M5c); a `//go:build live` smoke test (one canonical route,
  >0 rows, required fields non-zero) excluded from `mise run ci` and run on a CI
  schedule (M5c); a goroutine-leak check (M5c).

### M5a implementation deviations

- **M5 is split into three PRs.** M5a (this one) is stdlib-only, so `go.sum`
  stays empty and the `golang.org/x/time/rate` decision is reviewed on its own
  in M5b. M5c carries the observer hook, the encode round-trip fuzz, and the
  live canary.
- **No standalone logging RoundTripper.** The plan's four-layer stack collapsed
  the logging layer into `retryTransport`, which emits one `Debug` record per
  retry via the client's `*slog.Logger`. A dedicated layer buys nothing until
  there is a second thing to log; revisit in M5c with `WithObserver`.
- **`WithRetry` takes a `RetryPolicy` value, not variadic sub-options** —
  `RetryPolicy{MaxAttempts, BaseDelay, MaxDelay}`, `MaxAttempts < 2` is the off
  switch. New knobs are new struct fields, so it stays additive.
- **`search.go` still issues one `httpClient.Do()`.** `Do()` _is_ the
  RoundTripper entry point; the stack composes underneath it. Only the
  retry/backoff logic moved out, into `transport.go`.

---

## Amendments (2026-09-08 deep-analysis review)

A review of the M1 code against `fli` (the reference RPC client), `krisukox`,
`fast-flights`, and current Go + resilience practice. The plan's direction holds
— these amend the milestone bodies above; nothing here is a new milestone.

### Architecture — sound, do not restructure

The flat root package + three-way `internal/` split (`encoding` build, `wire`
framing, `decode` rows) is idiomatic and each layer is independently testable.
The findings below are additive, not structural.

- **A1 · Session id lives on the `Client`.** `lastSessionID` / `mu` is per-call
  mutable state on a shared object. → M2: return `SearchResult{Itineraries,
  SessionID}`; deprecate `Client.SessionID()`.
- **A2 · No resilience layer at all.** No retry, no backoff, no rate limit, no
  concurrency cap — for an upstream whose common failure is 429 / bot wall. →
  M5, pulled earlier. Compose as layered `RoundTripper`s; honour `Retry-After`;
  take `golang.org/x/time/rate` (this **relaxes the "empty `go.sum`" guardrail**
  — a single, quasi-stdlib dependency, called out in the README).
- **A3 · Anti-bot seam.** Only `WithHTTPClient` today. → M5 adds
  `WithTransport(http.RoundTripper)`; document a uTLS recipe, bundle nothing.
- **A5 · Two hand-rolled protobuf parsers** (`internal/encoding/protobuf.go` and
  `internal/decode/currency.go`'s `uvarint` / `protoField`). → unify onto
  `internal/encoding` and fuzz once (M5).
- `toItinerary` duplicates `decode.Flight` field-for-field. Acceptable as an
  anti-corruption boundary; revisit only if it becomes a maintenance drag.

### Feature gaps vs `fli`

- **A0 · Round-trip is overclaimed** — see the M1 deviation above. Fix the docs
  or land the two-phase flow **before M2 proper**.
- **A4 · Filters, sort, infants, richer amenities/segments** — all fold into M3;
  every `f.req` index is already mapped in `docs/wire/shopping-results.md`.
- Deferred, unchanged: multi-city, IATA dataset, CLI, MCP server (all remain out
  of scope); calendar graph + booking options stay M4.
- `SearchURL` (deep-link builder) is a strength — resilience practice
  independently recommends "always be able to hand back a deep link". → wire it
  into `*BlockedError` (M5, A2).

### Tests — deepen

- **Fuzzing — partly landed.** `FuzzChunks` (wire), `FuzzDecodeRow` and
  `FuzzCurrencyToken` (decode) exist as of M1.1. Still missing: a
  `decode(encode(x)) == x` round-trip fuzz for `internal/encoding`. → M5.
- **Output assertions were too loose.** `Search` tests checked `len > 0` /
  `price > 0`; a `leg[]` index swap would pass. **Done (M1.1):** golden-file
  `fixture → []Itinerary` tests (`golden_test.go`, `-update` flag,
  `internal/testdata/golden/`). Round-trip golden lands with its fixture in M2.
- **No live canary.** → `//go:build live` smoke test, CI schedule, out of
  `mise run ci` (M5).
- **No partial-failure signal.** Rows that fail to parse while others succeed are
  dropped silently. → `OnParse(rows, failures)` observer (M5, A2).
- `testing/synctest` for the retry / backoff / deadline tests (M5).
- `examples/search` is 0% covered — a test running `run()` against `httptest`
  closes that and guards the quickstart.

### Other

- **Doc accuracy** — round-trip claim in `doc.go`, `README.md`, `search.go`,
  `types.go`, `docs/plans/README.md` reconciled with reality in M1.1 (A0, done).
- **`CHANGELOG.md`** — `errors.Is` targets and decoded struct fields are the
  effective contract and both move with upstream; record every behaviour change.
- **`gorelease` in CI** on release PRs (`golang.org/x/exp/cmd/gorelease`) to
  catch accidental incompatible API diffs.
- **Typed errors where structured context exists** — `*BlockedError` over the
  bare `ErrBlocked` sentinel (A2).

### Recommended sequencing

1. **M1.1 (done):** round-trip doc claim downgraded; golden-file output tests
   added; `FuzzDecodeRow` + `FuzzCurrencyToken` added (`FuzzChunks` already
   existed).
2. **M2:** real two-phase round-trip; `WithMaxConcurrency`; `SearchResult` +
   session id; round-trip fixture.
3. **M3:** full filter set + sort + infants + amenity/segment fields.
4. **M5 (pulled ahead of M4):** retry / backoff / `Retry-After` /
   `WithRateLimit` / `WithTransport` as composed RoundTrippers; live canary;
   `synctest` tests; `*BlockedError`; observer hook.
5. **M4:** calendar graph + booking options.

---

## Plan file management

The user asked for this explicitly. Convention, effective M0:

```text
docs/plans/
  README.md      index table — every plan, its status, its PR
  scaffold.md    Status: implemented (3e2f6ae)
  porting.md     Status: in progress   <- this plan
  <topic>.md     one file per plan, kebab-case, no dates in the name
```

### Rules

- **One file per plan, one plan per milestone-group.** A plan file is durable
  and lives in git; it is not a scratchpad. Ephemeral working state stays in
  `.omc/` (already gitignored).
- **Every plan file opens with a `**Status:**` line** — `proposed`,
  `in progress`, `implemented (<sha>)`, or `abandoned (<why>)`. `scaffold.md`
  already sets this precedent; keep it.
- **Tasks are GitHub-flavoured checkboxes** under each milestone heading, ticked
  in the same PR that implements them. The plan file and the code move together,
  so a reviewer sees the plan diff alongside the implementation.
- **A plan is amended, never silently diverged from.** `scaffold.md` has an
  "Implementation deviations from the plan" section — that pattern is the rule:
  when reality differs, record it in the plan rather than leaving the plan
  wrong.
- **`docs/plans/README.md` is the index** and the only file that must be touched
  by every plan PR. Keep it a table: plan · status · PR · one-line hook.
- **Reverse-engineering notes are not plans.** Field maps and index tables go in
  `docs/wire/` (`shopping-results.md`, `tfs.md`, `booking-results.md`) — they
  outlive any single plan and are what a future maintainer greps when Google
  moves an index. fli keeps these in `.reverse-eng/notes/`; we track them,
  because they _are_ the value.
- `AGENTS.md` gains three lines under "Repo-wide" pointing at this convention.

---

## Attribution

All three reference libs are MIT. We are porting design and reverse-engineered
protocol facts, not source, but the protocol knowledge is genuinely theirs.
Add `docs/ACKNOWLEDGEMENTS.md` crediting fli, krisukox, AWeirdDev, and the
MomoDeve gist with links, and reference it from the README. Cheap, correct, and
it tells a future maintainer where to look when upstream shifts. No third-party
copyright headers go into `.go` files, since no source is copied verbatim.

---

## Guardrails

### Must hold

- `go.sum` stays empty through M4. **Amended (A2):** M5 may add
  `golang.org/x/time/rate` — one quasi-stdlib dependency, no transitive fan-out —
  and nothing else. `google.golang.org/protobuf` and uTLS stay out.
- No network in `go test`, ever. Every parser test is fixture-driven.
- Every fixture is scrubbed per `.claude/skills/fixture-capture/SKILL.md` and
  carries a `_provenance` block. Raw RPC bodies are not JSON, so they land as
  `internal/testdata/<endpoint>_<case>.txt` with a sibling
  `<name>.provenance.json` — the skill already allows this; note the extension
  in the skill's Naming section.
- Public API is additive-only against commit `3e2f6ae`. The one exception is
  `Price.Amount` (M1 item 7), which must gain an "unknown" representation — do
  it in M1, before anyone depends on the current shape, and say so in the
  release notes.
- Godoc on every exported symbol; `internal/` carries the churn.
- Untrusted-input discipline: every upstream index access is bounds-checked;
  a malformed row is skipped, never panics.

### Must not

- No HTML parsing, no `goquery`, no headless browser.
- No `google.golang.org/protobuf`, no `protoc` in `mise.toml`.
- No cookie-jar scraping from the user's browser (krisukox reads
  `GOOGLE_ABUSE_EXEMPTION` off disk via `kooky` — we do not touch the user's
  browser profile).
- No embedded IATA airport/airline dataset. Codes are opaque strings, validated
  only for shape (3 letters, uppercase). Revisit post-1.0 if asked.
- No global state, no package-level `http.Client`, no logging by default.

---

## Risks

| Risk                                                                                                                                                                                                 | Mitigation                                                                                                                                                                                                                                                                                                                                                 |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Go's TLS ClientHello gets fingerprinted and blocked.** fli relies on `curl_cffi impersonate="chrome"`; we have no equivalent in stdlib. This is the single biggest unknown and could stop M1 dead. | Probe it first — M1 item 1 is preceded by a throwaway manual `curl`-vs-Go comparison against the live endpoint. If Go's default is blocked, the answer is a documented `WithHTTPClient` recipe using uTLS in the README (consumer's dependency, not ours), plus `ErrBlocked` so the failure is legible. Escalate to the user before adding any dependency. |
| Google moves a response index silently (it swapped `[5]`/`[6]` on airports once).                                                                                                                    | `docs/wire/*.md` + named constants + the "0 of N rows parsed" hard error, so breakage is loud rather than an empty result set.                                                                                                                                                                                                                             |
| Fixtures go stale and tests pass against a dead shape.                                                                                                                                               | The `live`-tagged smoke test in M5, run manually before a release, not in CI.                                                                                                                                                                                                                                                                              |
| Scope creep — fli is 3.1k★ with a CLI, MCP server, and 290 KB airport dataset.                                                                                                                       | We ship a library. No `cmd/`, no MCP, no datasets. M1–M5 are the whole map.                                                                                                                                                                                                                                                                                |

---

## Verification

Per milestone, in order:

1. `mise run ci` — tidy + vet + lint + lint:actions + test + vuln, exit 0.
2. `git diff --exit-code -- go.mod go.sum` clean after `tidy`; `go.sum` absent
   or empty.
3. `go test -race -shuffle=on ./...` green; `go test -run xxx -fuzz FuzzChunks
-fuzztime 30s` finds no crash (M1).
4. **No network during tests** — confirm by running `mise run test` with the
   network down; it must still pass.
5. `mise run example` — for M1, against a local `httptest` recording, this
   prints real itineraries; the `//nolint:staticcheck` lines are gone.
6. One live manual check per milestone, outside CI: run the example against the
   real endpoint and eyeball that prices, times, and stops match what
   google.com/travel/flights shows for the same query. Record the result in the
   plan file's deviations section.
7. `pre-commit run --all-files` clean; `zizmor --persona=regular .` no findings.

---

## Out of scope

Multi-city search · IATA dataset · CLI or MCP server · caching layer · proxy
rotation · any anti-bot evasion beyond an honest User-Agent · booking
completion.
