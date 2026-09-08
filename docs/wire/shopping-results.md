# Wire format: `GetShoppingResults`

Undocumented. Reverse-engineered — see [`../ACKNOWLEDGEMENTS.md`](../ACKNOWLEDGEMENTS.md).
This is the map a maintainer greps when Google moves an index and the
`0 of N rows parsed` error fires. Index constants in
`internal/encoding/freq.go` and `internal/decode/flight.go` must match this
file; keep them in sync.

## Request

```text
POST https://www.google.com/_/FlightsFrontendUi/data/travel.frontend.flights.FlightsFrontendService/GetShoppingResults
Content-Type: application/x-www-form-urlencoded;charset=UTF-8

f.req=<percent-encoded JSON>
```

Locale rides the query string: `curr=` (**must be uppercase** — Google
silently ignores lowercase), `hl=`, `gl=`.

The `f.req` value is double-encoded: build the filter array, `json.Marshal` to a
string, wrap as `[null, "<that string>"]`, marshal again, percent-encode.

```text
outer[0] = []                       outer[3] = 1  (all results; 0 caps at ~30)
outer[1] = main settings            outer[4] = 0  (probed, inert)
outer[2] = sort mode (1..5)         outer[5] = 1  (probed, inert)

main[2]   trip type: 1 round-trip, 2 one-way, 3 multi-city
main[4]   [] — rejects scalars; empty list is the inert form
main[5]   cabin: 1 economy, 2 premium economy, 3 business, 4 first
main[6]   [adults, children, infants_lap, infants_seat]
main[7]   [null, max_price]                 (M3)
main[10]  [checked_bags, carry_on]          (M3)
main[13]  segments
main[17]  1 (constant, set by the UI)
main[28]  exclude basic economy (0 | 1)
          — every other index is null and was probed to no effect

segment[0]   [[[IATA, 0]]] departure — exactly 3 levels; wrong depth = 0 results, no error
segment[1]   [[[IATA, 0]]] arrival
segment[2]   [earliest_dep, latest_dep, earliest_arr, latest_arr] hour buckets (M3)
segment[3]   max stops (int; 0 = no constraint)
segment[4]   airline / alliance include     (M3)
segment[5]   airline / alliance exclude     (M3)
segment[6]   "YYYY-MM-DD"
segment[7]   [max_duration_mins]            (M3)
segment[8]   selected_flight — round-trip second phase. On the outbound
             segment only: [[ leg, leg, ... ]] where each leg is
             [origin, "YYYY-MM-DD", dest, null, carrier, flight_number].
             Nesting depth is structurally derived, not live-captured.
segment[9]   layover airport include        (M3)
segment[11]  min layover minutes            (M3)
segment[12]  max layover minutes            (M3)
segment[13]  [1] = less-emissions only      (M3)
segment[14]  classifier: 3 = outbound / only leg, 1 = return leg of a round trip
```

## Response

JSONP-flavoured envelope, decoded by `internal/wire`:

```text
)]}'\n\n
<utf8_byte_len>\n         <- byte length, NOT rune count
[["wrb.fr", null, "<inner JSON string>"], ...]
<utf8_byte_len>\n
[["wrb.fr", null, "<inner JSON string>"], ...]
```

`GetShoppingResults` emits **one** chunk today and may omit the length line
entirely (bare frame); `GetBookingResults` emits several. The reader handles
both. A `wrb.fr` row with a null payload and `row[5] == [N]` is an upstream
status code: **4** = rate limited, **13** = structurally accepted but refused by
the app (what a fingerprinted TLS client currently gets).

Inside a chunk's inner JSON:

```text
inner[0][4]              shopping session id (needed for GetBookingResults, M4)
inner[2][0], inner[3][0] flight rows — concatenated in order

row[0]  = detail          row[1]  = price block   row[8] = booking token
row[10] = mixed cabin (bool)

price block: [[], "<token>"]  -> no shopping-list price; Unknown, NOT zero
             [[..., amount], "<token>"] -> amount is the last element
             the "<token>" is base64url protobuf; field 3 -> field 3 = ISO 4217

detail[0]   primary carrier code ("multi" for a codeshare mix)
detail[1]   names ([0] = carrier name)
detail[2]   legs
detail[9]   total duration (minutes)
detail[12]  self-transfer (bool)
detail[13]  layover names / cities, one entry per layover ([4] = name, [5] = city)
detail[22]  emissions block: [3] delta %, [7] this grams, [8] typical grams,
            [11] tag (1 lower, 2 typical, 3 higher)

leg[3]/[6]   dep / arr airport codes      leg[4]/[5]  dep / arr airport names
leg[8]/[10]  dep / arr time [h, m]        leg[11]     duration (minutes)
leg[12]      amenities (12 slots): [1] wifi, [5] power, [9] on-demand video,
             [11] legroom rating (2 normal / 3 extra)
leg[14]      legroom short   leg[17] aircraft   leg[19] overnight (bool)
leg[20]/[21] dep / arr date [y, m, d]
leg[22]      [airline, flight_no, operating_airline]
leg[30]      legroom long (preferred over [14])   leg[31] CO2 grams
```

`(hour, minute)` tuples come as `(h, m)`, `(h,)`, or `(null, m)` — the parser in
`internal/decode/flight.go` tolerates all three and treats missing parts as 0.
Times are naive local airport time (no zone).
