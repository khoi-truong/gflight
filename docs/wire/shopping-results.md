# Wire format: shopping results

Undocumented. Reverse-engineered — see [`../ACKNOWLEDGEMENTS.md`](../ACKNOWLEDGEMENTS.md).
This is the map a maintainer greps when Google moves an index and the
`0 of N rows parsed` error fires. Index constants in
`internal/encoding/freq.go` and `internal/decode/flight.go` must match this
file; keep them in sync.

## Request

```text
POST https://www.google.com/_/FlightsFrontendUi/data/batchexecute?rpcids=LqxFAb&source-path=%2Ftravel%2Fflights&curr=USD&hl=en&gl=US
Content-Type: application/x-www-form-urlencoded;charset=UTF-8

f.req=<percent-encoded JSON>
```

`LqxFAb` is `GetShoppingResultsPrefetch`. It is the only rpc id an anonymous
caller can reach; `GetShoppingResults` itself (`Sipqjf`) returns the same
payload but is BotGuard-gated, as is the per-method path style
(`…/FlightsFrontendService/GetShoppingResults`). See
[`botguard.md`](botguard.md).

Locale rides the query string: `curr=` (**must be uppercase** — Google
silently ignores lowercase), `hl=`, `gl=`.

The `f.req` value is double-encoded: build the filter array, `json.Marshal` to a
string, then wrap it in the batchexecute envelope
`[[["LqxFAb", "<that string>", null, "generic"]]]`, marshal again, and
percent-encode. The rpc id appears both here and in the query string; both must
name the same method.

```text
outer[0] = []                       outer[3] = 1  (all results; 0 caps at ~30)
outer[1] = main settings            outer[4] = 0  (probed, inert)
outer[2] = sort mode (1..5)         outer[5] = 1  (probed, inert)

main[2]   trip type: 1 round-trip, 2 one-way, 3 multi-city
main[4]   [] — rejects scalars; empty list is the inert form
main[5]   cabin: 1 economy, 2 premium economy, 3 business, 4 first
main[6]   [adults, children, infants_lap, infants_seat]
main[7]   [null, max_price] — omitted (null) when uncapped
main[10]  [checked_bags, carry_on] — omitted (null) when both are 0
main[13]  segments
main[17]  1 (constant, set by the UI)
main[28]  exclude basic economy (0 | 1)
          — every other index is null and was probed to no effect

segment[0]   [[[IATA, 0]]] departure — exactly 3 levels; wrong depth = 0 results, no error
segment[1]   [[[IATA, 0]]] arrival
segment[2]   [earliest_dep, latest_dep, earliest_arr, latest_arr] hour buckets;
             an unset bound is null, an all-unset window is null
segment[3]   max stops (int; 0 = no constraint)
segment[4]   airline / alliance include — flat list of IATA codes or alliance
             names ("STAR_ALLIANCE", "SKYTEAM", "ONEWORLD"); null when empty
segment[5]   airline / alliance exclude — same shape as [4]
segment[6]   "YYYY-MM-DD"
segment[7]   [max_duration_mins]; null when uncapped
segment[8]   selected_flight — round-trip second phase. On the outbound
             segment only: [[ leg, leg, ... ]] where each leg is
             [origin, "YYYY-MM-DD", dest, null, carrier, flight_number].
             Nesting depth is structurally derived, not live-captured.
segment[9]   layover airport include — flat list of IATA codes; null when empty
segment[11]  min layover minutes (scalar int; null when unset)
segment[12]  max layover minutes (scalar int; null when unset)
segment[13]  [1] = less-emissions only; null when off
segment[14]  classifier: 3 = outbound / only leg, 1 = return leg of a round trip
```

The exact shape of the M3 filter slots — [2], [4], [5], [7], [9], [11], [12],
[13], main[7] and main[10] — is **structurally derived** from the index map
above and from `fli`'s builder, not from a live capture. Each is frozen in
`internal/encoding/freq_test.go` (`goldenAllFiltersOneWay`). They have since
been exercised against the live endpoint — the exclude-basic-economy filter
measurably changes the response size — but the goldens themselves remain
synthetic.

## Response

JSONP-flavoured envelope, decoded by `internal/wire`:

```text
)]}'\n\n
<utf8_byte_len>\n         <- byte length, NOT rune count
[["wrb.fr", null, "<inner JSON string>"], ...]
<utf8_byte_len>\n
[["wrb.fr", null, "<inner JSON string>"], ...]
```

The length is a **byte** count, not a rune count. Two conventions are in
circulation: the count covers the JSON alone, or it also covers the newlines
around it. Since the JSON is minified and contains no newline of its own,
`internal/wire` ends each frame at the last newline inside the counted span,
which is correct under either. Getting this wrong swallows the first digit of
the next header and desynchronises every frame after it — invisible on a
single-chunk reply, fatal on a multi-chunk one.

The framing appears only when the request carries `&rt=c`. Without it the live
route answers with a **bare, unframed** body: the `)]}'` guard and then the
array, no length lines at all. The reader handles both, and handles a reply that
is a single bare frame.

A `wrb.fr` row with a null payload and `row[5] == [N]` is an upstream status
code: **3** = INVALID_ARGUMENT (the method is open, the payload was wrong),
**4** = rate limited, **13** = refused by the app. A **13** on this endpoint
means the method is BotGuard-gated, not that the client looks wrong; see
[`botguard.md`](botguard.md).

Inside a chunk's inner JSON:

```text
inner[0][4]              shopping session id; surfaced as SearchResult.SessionID
inner[1]                 airport directory. Nesting depth varies with trip type,
                         so the decoder walks it by shape; each leaf entry is
                         [[IATA, 0], "<airport name>", ["<mid>", "<city>", …],
                         [lat, lng], "<country code>", bool, "<country>"].
                         Only the searched endpoints appear — a connection
                         airport's city comes from detail[13] instead.
inner[2][0], inner[3][0] flight rows — concatenated in order

row[0]  = detail          row[1]  = price block   row[8] = booking token
                          (opaque; the method that spends it is gated)
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
leg[12]      amenities (12 slots): [1] wifi, [5] in-seat AC power,
             [7] USB power, [8] in-seat (seat-back) video,
             [9] streaming / on-demand video,
             [11] legroom rating (2 normal / 3 extra)
             — [5]/[9] were mapped in M1; the [7]/[8] split is structurally
             derived (M3). Slot [10] is set in recorded fixtures but its
             meaning is unidentified; slots [0], [2], [3], [4], [6] are always
             null in every fixture. `decode.Amenities.Power` is the OR of
             [5]|[7] and `.OnDemandVideo` the OR of [8]|[9], so the pre-M3
             folded fields keep their meaning.
leg[14]      legroom short   leg[17] aircraft   leg[19] overnight (bool)
leg[20]/[21] dep / arr date [y, m, d]
leg[22]      [airline, flight_no, operating_airline, airline_name,
             operating_flight_no] — index [4] is structurally derived; no
             recorded fixture carries a codeshare, so [2] and [4] are null in
             all of them
leg[30]      legroom long (preferred over [14])   leg[31] CO2 grams
```

`(hour, minute)` tuples come as `(h, m)`, `(h,)`, or `(null, m)` — the parser in
`internal/decode/flight.go` tolerates all three and treats missing parts as 0.
Times are naive local airport time (no zone).
