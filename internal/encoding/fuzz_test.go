package encoding

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"testing"
	"time"
)

// FuzzEncodeTFSRoundTrip asserts decode(encode(x)) == x for the leg fields of
// the `tfs` token: whatever strings a caller hands the encoder come back
// byte-identical from the protobuf, and encoding never panics.
func FuzzEncodeTFSRoundTrip(f *testing.F) {
	f.Add("JFK", "LAX", "AA", "100", int64(0), true)
	f.Add("SFO", "NRT", "", "", int64(90), false)
	f.Add("", "", "", "", int64(0), true)
	f.Add("a/b?c=d&e", "\x00\xff", "µ", "٩٩", int64(-4000), false)

	f.Fuzz(func(t *testing.T, origin, dest, airline, flightNo string, dayOffset int64, oneWay bool) {
		date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(dayOffset%4000))
		leg := TFSLeg{Origin: origin, Dest: dest, Date: date, Airline: airline, FlightNumber: flightNo}
		token, err := EncodeTFS(TFSRequest{Segments: []TFSSegment{{Legs: []TFSLeg{leg}}}, OneWay: oneWay})
		if err != nil {
			// Only a missing origin or destination is a legitimate refusal.
			if origin == "" || dest == "" {
				return
			}
			t.Fatalf("EncodeTFS(%q -> %q): %v", origin, dest, err)
		}

		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatalf("token is not base64url: %v", err)
		}
		got, ok := decodeTFSLeg(t, raw)
		if !ok {
			t.Fatal("encoded token carries no leg")
		}
		if got.Origin != leg.Origin || got.Dest != leg.Dest {
			t.Errorf("route round-trip = %q -> %q, want %q -> %q", got.Origin, got.Dest, leg.Origin, leg.Dest)
		}
		if got.Airline != leg.Airline || got.FlightNumber != leg.FlightNumber {
			t.Errorf("flight round-trip = %q %q, want %q %q",
				got.Airline, got.FlightNumber, leg.Airline, leg.FlightNumber)
		}
		if want := date.Format(tfsDateLayout); got.Date != want {
			t.Errorf("date round-trip = %q, want %q", got.Date, want)
		}
	})
}

// tfsLeg is a decoded leg: the date stays a string so the comparison is against
// the bytes on the wire, not a re-parsed time.
type tfsLeg struct {
	Origin       string
	Dest         string
	Date         string
	Airline      string
	FlightNumber string
}

// decodeTFSLeg reads the first leg back out of an encoded tfs payload —
// message field 3 (segment) -> field 4 (leg).
func decodeTFSLeg(t *testing.T, raw []byte) (tfsLeg, bool) {
	t.Helper()
	top, err := parseMessage(raw)
	if err != nil {
		t.Fatalf("parseMessage(top): %v", err)
	}
	for _, f := range top {
		if f.Field != 3 || f.Wire != 2 {
			continue
		}
		segFields, err := parseMessage(f.Bytes)
		if err != nil {
			t.Fatalf("parseMessage(segment): %v", err)
		}
		for _, sf := range segFields {
			if sf.Field != 4 || sf.Wire != 2 {
				continue
			}
			legFields, err := parseMessage(sf.Bytes)
			if err != nil {
				t.Fatalf("parseMessage(leg): %v", err)
			}
			var leg tfsLeg
			for _, lf := range legFields {
				switch lf.Field {
				case 1:
					leg.Origin = string(lf.Bytes)
				case 2:
					leg.Date = string(lf.Bytes)
				case 3:
					leg.Dest = string(lf.Bytes)
				case 5:
					leg.Airline = string(lf.Bytes)
				case 6:
					leg.FlightNumber = string(lf.Bytes)
				}
			}
			return leg, true
		}
	}
	return tfsLeg{}, false
}

// FuzzEncodeFreqRoundTrip asserts the `f.req` body is always a percent-encoded
// batchexecute envelope whose payload element is itself valid JSON — the shape
// the endpoint requires — for arbitrary filter values, and that encoding never
// panics.
func FuzzEncodeFreqRoundTrip(f *testing.F) {
	f.Add("JFK", "LAX", 1, 0, 2, 1200, false)
	f.Add("", "", 0, 0, 0, 0, true)
	f.Add("a\"b", "c\\d", -1, 99, -5, 1<<20, true)
	f.Add("µµµ", "\x00", 3, 2, 1, -60, false)

	f.Fuzz(func(t *testing.T, origin, dest string, adults, children, maxStops, maxPrice int, roundTrip bool) {
		req := FreqRequest{
			Segments: []FreqSegment{{
				Origin:   origin,
				Dest:     dest,
				Date:     time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
				MaxStops: maxStops,
			}},
			Adults:   adults,
			Children: children,
			MaxPrice: maxPrice,
		}
		if roundTrip {
			req.Segments = append(req.Segments, FreqSegment{
				Origin:   dest,
				Dest:     origin,
				Date:     time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC),
				IsReturn: true,
			})
		}

		body, err := EncodeFreq(req)
		if err != nil {
			return // a refusal is a valid outcome; a panic or bad JSON is not
		}

		decoded, err := url.QueryUnescape(Batch("LqxFAb", body))
		if err != nil {
			t.Fatalf("f.req is not percent-encoded: %v", err)
		}
		var envelope [][][]any
		if err := json.Unmarshal([]byte(decoded), &envelope); err != nil {
			t.Fatalf("f.req envelope is not JSON: %v", err)
		}
		if len(envelope) != 1 || len(envelope[0]) != 1 || len(envelope[0][0]) != 4 {
			t.Fatalf("f.req envelope shape = %v", envelope)
		}
		payload, ok := envelope[0][0][1].(string)
		if !ok {
			t.Fatalf("envelope payload is %T, want a JSON string", envelope[0][0][1])
		}
		var inner []any
		if err := json.Unmarshal([]byte(payload), &inner); err != nil {
			t.Fatalf("envelope payload is not JSON: %v", err)
		}
	})
}
