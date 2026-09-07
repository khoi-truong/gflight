---
name: fixture-capture
description: Capture, scrub, and commit a Google Flights HTTP response as a test fixture in internal/testdata/. Use when adding or refreshing a fixture, when a parser test needs new recorded data, or when an existing fixture has gone stale against upstream.
---

# Fixture capture

Google Flights has no sandbox and `go test` never touches the network, so every
parser test is driven by a recorded response in `internal/testdata/`. A fixture
that still carries session data is a leak; a fixture that carries a timestamp is
a flake. This procedure prevents both.

## Rule

**Never commit an unscrubbed response.** If you cannot complete the scrub
checklist below, delete the capture and start over.

## Naming

`internal/testdata/<endpoint>_<case>.json` — lowercase, snake_case, no dates in
the filename.

- `<endpoint>` — the upstream call, e.g. `search`, `batchexecute`.
- `<case>` — what makes the case distinct, e.g. `oneway_sgn_han`,
  `roundtrip_multistop`, `no_results`, `error_429`.

Examples: `search_oneway_sgn_han.json`, `search_no_results.json`,
`batchexecute_error_429.json`.

## Procedure

1. **Decide the case first.** One fixture per behaviour under test, not one per
   ad-hoc query. If an existing fixture already covers the shape, refresh it
   rather than adding a near-duplicate.
2. **Capture.** Issue the live request with the same parameters the test will
   use, and save the raw response body plus the status code and the response
   headers you actually assert on. Record the exact request that produced it.
3. **Scrub** — see the checklist below. Scrub before the file ever enters the
   working tree in a committable state.
4. **Add the provenance header** as the fixture's first key (see below).
5. **Pin the dynamic values.** Any date the parser reads must be either a fixed
   date the test also uses, or clearly relative and handled by the test's clock
   injection. Never let a fixture's meaning depend on today's date.
6. **Wire it up.** Serve the fixture from an `httptest.Server` and point the
   client at it with `gflight.WithBaseURL(srv.URL)` (or `WithHTTPClient`).
7. **Verify.** Run `mise run test` and confirm the affected test fails against
   the old fixture and passes against the new one. Then run `mise run ci`.
8. **Commit** the fixture and the test together, never separately.

## Scrub checklist

Every item must be checked before commit:

- [ ] **Cookies** — no `Set-Cookie` headers, no `NID`, `SID`, `HSID`, `SSID`,
      `APISID`, `SAPISID`, `__Secure-*`, `SIDCC`, or consent cookies anywhere.
- [ ] **Session and auth identifiers** — no `at`, `f.sid`, `bl`, `SNlM0e`,
      `_reqid`, `rt`, or bearer tokens; replace each with a fixed placeholder
      such as `"REDACTED"`.
- [ ] **User identity** — no account id, email, display name, avatar URL,
      profile photo, or Google account number.
- [ ] **Location and device** — no IP address, precise geolocation, device id,
      or full User-Agent that identifies your machine.
- [ ] **Timestamps** — every absolute clock value pinned to a fixed instant the
      test knows about; no "now"-relative values left implicit.
- [ ] **Tracing** — no `x-client-data`, `x-goog-*` request ids, or trace headers.
- [ ] **URLs** — query strings stripped of the above; keep only the parameters
      the parser reads.
- [ ] **Size** — trim to the smallest response that still exercises the case;
      large blobs trip `check-added-large-files` in pre-commit.
- [ ] **Final read-through** — open the finished file and read it end to end.
      Automated scrubbing misses fields you have never seen before.

## Provenance header

Every fixture starts with a `_provenance` object so a future reader knows what
it is and when it stopped being trustworthy:

```json
{
  "_provenance": {
    "captured_at": "2026-09-07",
    "endpoint": "https://www.google.com/travel/flights",
    "request": "SGN->HAN one-way, depart 2026-10-15, 1 adult, economy, VND",
    "scrubbed": true,
    "notes": "trimmed to the first 3 itineraries"
  },
  "...": "response body follows"
}
```

If the response format cannot carry a header (raw HTML, a non-JSON blob), put
the same fields in a sibling `<name>.provenance.json`.

## When upstream changes

A parser test failing against a fixture means the fixture is stale *or* upstream
changed. Re-capture before changing the parser — a fixture refresh and a parser
fix belong in separate commits so the diff shows which one moved.
