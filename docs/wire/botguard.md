# BotGuard: why only one method is reachable

**Captured:** 2026-09-11, against `www.google.com` from a residential IP.

`FlightsFrontendService` exposes ~29 methods. Exactly one of them answers an
anonymous caller. This document records how that was established, so the next
person does not repeat the investigation.

## The gate

Every method except `GetShoppingResultsPrefetch` requires an
`X-Goog-BatchExecute-Bgr` request header. It carries a BotGuard token minted by
obfuscated in-page JavaScript.

A header-ablation replay of a live browser request — captured over the Chrome
DevTools Protocol, then re-sent from outside the browser with one header removed
at a time — isolates it:

```text
all captured headers      http=200 bytes=1450 OK
without Bgr               http=200 bytes=132  BLOCKED(13)
without X-Same-Domain     http=200 bytes=1450 OK
without cookies           http=200 bytes=1450 OK
only Bgr + content-type   http=200 bytes=1449 OK
```

The token alone is sufficient and necessary. Cookies, `X-Same-Domain`, the user
agent and the session id are all irrelevant.

It is also **bound to the exact request body**. Replaying a valid token against a
payload differing only in dates, or only in route, is refused:

```text
same token, different dates    http=200 bytes=132 BLOCKED(13)
same token, different route    http=200 bytes=132 BLOCKED(13)
```

So a token cannot be harvested once and reused. Producing one means running
Google's JavaScript per request.

Two things this is **not**:

- **Not TLS fingerprinting.** A uTLS `HelloChrome_Auto` client over
  `golang.org/x/net/http2` reproduces Chrome's ClientHello exactly and is still
  refused. The theory was tested and falsified.
- **Not a retired endpoint.** The per-method path style
  (`…/travel.frontend.flights.FlightsFrontendService/<Method>`) is alive and is
  what the UI uses. It is gated, not gone. `batchexecute` is gated identically —
  the transport is not what matters.

## Telling gated from open

The two failure modes are distinguishable, which is what makes the table below
verifiable:

| observation | meaning |
| --- | --- |
| HTTP 200, `wrb.fr` status `[3]` | method is **open**; the payload was wrong |
| HTTP 500, `er` frame ending `13` | method is **gated** |

A gated method returns 500 even when handed the UI's own byte-exact payload.

## The rpc id table

`batchexecute` names methods by an opaque rpc id rather than a path segment. The
ids are registered in the UI's JS bundle as
`new _.Lo("<rpcid>", …, "/FlightsFrontendService.<Method>")` and can be read out
of it. They are unversioned and can change without notice.

| method | rpc id | anonymous |
| --- | --- | --- |
| GetShoppingResultsPrefetch | `LqxFAb` | **open** |
| GetShoppingResults | `Sipqjf` | gated |
| GetBookingResults | `cgyvtd` | gated |
| GetCalendarGrid | `z1rpkf` | gated |
| GetCalendarPicker | `j1QEDd` | gated |
| GetCalendarGraph | `YMjVO` | gated |
| GetSolutionPrices | `d9FBod` | gated |
| GetPartnerLinkPrices | `Fb9Q7` | gated |
| GetBookingLink | `FJdy9d` | open, payload shape unknown |
| GetSolutions | `pkqvib` | open, signed-in only (returns `[]`) |
| GetLocations | `tDoGIe` | open, payload shape unknown |
| GetMarketSummaries | `kcx4Ke` | open, payload shape unknown |

`GetCalendarGraph` still exists as an rpc id but the UI no longer calls it; the
live calendar methods are `GetCalendarGrid` (7-day window) and
`GetCalendarPicker` (month window), both taking `[null, main[:18], [from, to]]`.
All three are gated.

## Why the prefetch route is open

`GetShoppingResultsPrefetch` has to answer before any JavaScript runs — it is
what renders the initial page — so it cannot require a JS-minted token. It
returns the same shopping payload as `GetShoppingResults`, booking tokens
included, and honours every filter. Verified on the live endpoint:

```text
round trip phase 1      bytes=56354  OK   (outbound options)
round trip phase 2      bytes=32006  OK   (selected-flight flow)
exclude basic economy   bytes=62736  OK   (filter bites)
baseline                bytes=64736  OK
```

This is the single point of failure for the whole library. If Google gates
`LqxFAb`, nothing here works, and there is no known fallback.

## Consequences

- `Client.Search`, `Client.SearchResults` and `Client.RoundTripTopN` work.
- Vendor fares (`GetBookingResults`) are unreachable. `BookingOptions` was
  removed rather than shipped as a method that always fails.
- A price calendar is unreachable. Deriving one would mean one shopping call per
  departure date.
- `Itinerary.BookingToken` is still decoded and exposed, but nothing in this
  library can spend it.
