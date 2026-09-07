# Plan: port a real Google Flights client into `gflight`

**Status:** in progress · **Module:** `github.com/khoi-truong/gflight` · Go 1.26 · MIT
**Supersedes nothing.** Follows `docs/plans/scaffold.md` (commit `3e2f6ae`).

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

1. **`internal/encoding/protobuf.go`** — `varint`, `tag(field, wire)`,
   `lengthDelim(field, payload)`, `varintField(field, v)`, plus a `reader` for
   round-trip tests. Table-driven tests against known-good vectors decoded from
   real Google URLs.
2. **`internal/encoding/tfs.go`** — build the `tfs` token per the spec above.
   Verified by a golden test: a token we generate must byte-match one captured
   from a live Google Flights URL for the same query.
3. **`url.go`** — public `SearchURL(req SearchRequest) (string, error)`.
   Network-free, so it is fully unit-tested and useful on its own.
4. **`internal/encoding/freq.go`** — the `f.req` builder. The index map above
   becomes **named constants**, not magic numbers; every `null` slot carries the
   comment explaining it was probed and had no observable effect.
5. **`internal/wire/chunks.go`** — the byte-accurate multi-chunk reader.
   Handles both the headerless single-chunk shape and the length-prefixed
   multi-chunk shape. Add `FuzzChunks` — this parses untrusted upstream bytes.
6. **`internal/decode/flight.go`** — row → `Itinerary`. Defensive accessors
   (`safeIndex`, `asString`, `asInt`, `asBool` — fli's `_helpers.py` pattern) so
   the decoder reads as a list of position lookups. A row that fails to decode is
   **skipped**; if _every_ row fails, return `ErrUpstreamChanged` wrapping
   `ErrBadResponse` with up to three sample reasons — do not silently return
   empty.
7. **Widen `types.go`** — `Layover`, `Amenities`, `Emissions`; add
   `Itinerary.BookingToken`, `.Layovers`, `.Emissions`, `.SelfTransfer`,
   `.MixedCabin`, `.PrimaryCarrier`; add `Segment.Aircraft` (exists),
   `.Legroom`, `.OperatingCarrier`, `.Amenities`, `.Overnight`, `.CO2Grams`.
   **`Price.Amount` must become a pointer or gain `Price.Unknown bool`** —
   `[[], "token"]` means Google declined to price the row (routine for
   premium-cabin round trips) and must not decode to `0`.
8. **`search.go`** — wire it together: build `f.req`, POST, read chunks, decode
   rows, capture the session id on the `Client` for M4. Non-2xx → `*HTTPError`;
   429 or a consent interstitial → `ErrBlocked`; zero rows → `ErrNoResults`.
9. **Fixtures + tests** — `httptest.Server` serving recorded bodies via
   `WithBaseURL`. Cases: `oneway_sgn_han`, `oneway_no_price`, `no_results`,
   `error_429`, `truncated_chunk`. Remove the two `//nolint:staticcheck` in
   `examples/search/main.go`.
10. **Docs** — `doc.go` status paragraph, README quickstart, disclaimer intact.

### M2 — round-trip · `feat: support round-trip search`

Two-phase: search outbound, then for each of the top N outbounds re-POST with
`segment[8]` = selected flight and `segment[14]` = 1 on the return segment.
Needs a bounded-concurrency helper (`errgroup`-shaped, hand-rolled — stdlib
only) and a `WithMaxConcurrency` option. Round-trip prices are **totals**, not
per-leg — summing double-counts.

### M3 — filters · `feat: expose full search filters`

Airline/alliance include+exclude, max price, bags, max duration, layover
airports and min/max duration, departure/arrival hour windows, emissions,
exclude-basic-economy, sort mode. All additive on `SearchRequest`; each maps to a
named `f.req` index already documented in M1.

### M4 — calendar graph + booking options · `feat: add price calendar and booking options`

`GetCalendarGraph` (≤61 days per call, ≤305 days ahead — chunk and merge) and
`GetBookingResults` (needs the session-anchored booking token; `internal/encoding`
gains `bookingToken.go`). This is where the multi-chunk reader earns its keep.

### M5 — hardening · `chore: harden upstream resilience`

Retry with backoff on 429/5xx behind `WithRetry` (hand-rolled, stdlib), a
documented rate-limit note (Google's ceiling is ~10 req/s), a build-tagged
`//go:build live` smoke test excluded from `mise run ci`, and a `FuzzDecodeRow`.

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

- `go.sum` stays empty. `go.mod` has no `require` block.
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
