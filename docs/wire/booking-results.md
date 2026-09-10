# Wire format: `GetBookingResults`

Undocumented. Reverse-engineered — see [`../ACKNOWLEDGEMENTS.md`](../ACKNOWLEDGEMENTS.md).
This is the map a maintainer greps when Google moves an index and the
`0 of N rows parsed` error fires. Index constants in
`internal/encoding/booking.go` and `internal/decode/booking.go` must match this
file; keep them in sync.

`GetBookingResults` turns one itinerary's opaque `row[8]` booking token from a
[`GetShoppingResults`](shopping-results.md) reply into the vendor offers behind
it — the only way to price an itinerary Google returned without a shopping-list
price.

## Request

```text
POST https://www.google.com/_/FlightsFrontendUi/data/travel.frontend.flights.FlightsFrontendService/GetBookingResults
Content-Type: application/x-www-form-urlencoded;charset=UTF-8

f.req=<percent-encoded JSON>
```

Same locale query parameters and same double-encoding as `GetShoppingResults`:
build the body, `json.Marshal` to a string, wrap as `[null, "<that string>"]`,
marshal again, percent-encode.

```text
body[0] = [null, "<booking token>"]
body[1] = the GetShoppingResults main settings block, first 18 slots (0..17)
```

The main block is trimmed at `main[17]`; the slots past it describe the shopping
list, which is already settled once a booking token exists. `main[2]` (trip
type), `main[5]` (cabin) and `main[6]` (passengers) still matter — Google prices
the offers against them, so the booking call must reuse the search's request.

The token is required. `fli` *synthesises* one from price / airline / flight
number / currency when a row has none; we decode the real `row[8]` instead and
reject an itinerary without it.

## Response

Same `)]}'` + length-prefixed `wrb.fr` framing as `GetShoppingResults` — see
that file for how the length header is counted. Unlike shopping, this endpoint
really does emit **several** chunks, so it is the first genuine exercise of the
multi-chunk reader.

The recorded capture has 4 frames / 2 `wrb.fr` payloads plus `di`, `af.httprm`
and `e` bookkeeping rows. **The first payload carries no option list at all**; a
payload without one is normal, not an error.

```text
inner[0][3], inner[0][4]  session ids (scrubbed in the fixture)
inner[1][0]               the option rows

row[1][0]   [vendor_code, vendor_name, null, bool] — e.g. ["AA", "American"]
row[3]      flights, [[carrier, flight_no], ...]
row[5]      [display_host, null, [base_url, [[key, value], ...]]]
            the click-out link: base_url is
            "https://www.google.com/travel/clk/f" and the pairs are its query,
            in practice a single "u" token. Not the vendor's own URL.
row[7]      price block, [[..., amount], "<token>"] — identical to the shopping
            price block: the amount is the last element of the head, and the
            token is base64 protobuf whose field 3 -> field 3 is the ISO 4217
            code. Booking tokens use the **standard** base64 alphabet (they
            contain "+"), shopping tokens the URL-safe one, so the decoder
            tries both.
row[14]     [[[null, [carrier, FARE_CODE], n]]] — the same fare pair as row[21]
row[21]     [[carrier, FARE_CODE], <amenity pairs>, bool, "<fare name>"]
            [0][1] is the fare code ("BASIC ECONOMY"), [3] the human-readable
            name ("Basic Economy")
row[22]     [carrier, null, n]
```

Rows are 23–25 elements long and the length varies between rows in one reply,
so every index is bounds-checked and a malformed row is skipped, not fatal.
