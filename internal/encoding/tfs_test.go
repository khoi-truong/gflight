package encoding

import (
	"encoding/base64"
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(tfsDateLayout, s)
	if err != nil {
		t.Fatalf("bad date %q: %v", s, err)
	}
	return d
}

// The golden token below is round-trip-verified structurally (see
// TestEncodeTFSStructure) but is NOT a live capture — Google's endpoint is
// currently unreachable for verification. See docs/plans/porting.md deviations.
const goldenOneWaySGNHAN = "CBwQAho2EgoyMDI2LTEwLTAxIhYKA1NHThIKMjAyNi0xMC0wMRoDSEFOagcIARIDU0dOcgcIARIDSEFOQAFIAXABggELCP___________wGYAQI"

func TestEncodeTFSGolden(t *testing.T) {
	t.Parallel()
	got, err := EncodeTFS(TFSRequest{
		OneWay: true,
		Segments: []TFSSegment{{
			Legs: []TFSLeg{{Origin: "SGN", Dest: "HAN", Date: mustDate(t, "2026-10-01")}},
		}},
	})
	if err != nil {
		t.Fatalf("EncodeTFS: %v", err)
	}
	if got != goldenOneWaySGNHAN {
		t.Errorf("token drift:\n got %q\nwant %q", got, goldenOneWaySGNHAN)
	}
}

func TestEncodeTFSStructure(t *testing.T) {
	t.Parallel()
	token, err := EncodeTFS(TFSRequest{
		OneWay: true,
		Segments: []TFSSegment{{
			Legs: []TFSLeg{{Origin: "JFK", Dest: "LAX", Date: mustDate(t, "2026-06-28")}},
		}},
	})
	if err != nil {
		t.Fatalf("EncodeTFS: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not base64url: %v", err)
	}
	fields, err := parseMessage(raw)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}

	byField := map[int]pbField{}
	for _, f := range fields {
		byField[f.Field] = f
	}
	if byField[1].Varint != 28 {
		t.Errorf("field 1 = %d, want 28", byField[1].Varint)
	}
	if byField[19].Varint != 2 {
		t.Errorf("field 19 (trip type) = %d, want 2 (one-way)", byField[19].Varint)
	}
	seg, ok := byField[3]
	if !ok {
		t.Fatal("no segment field 3")
	}
	segFields, err := parseMessage(seg.Bytes)
	if err != nil {
		t.Fatalf("parse segment: %v", err)
	}
	var sawDate, sawLeg bool
	for _, sf := range segFields {
		switch sf.Field {
		case 2:
			if string(sf.Bytes) != "2026-06-28" {
				t.Errorf("segment date = %q", sf.Bytes)
			}
			sawDate = true
		case 4:
			legFields, err := parseMessage(sf.Bytes)
			if err != nil {
				t.Fatalf("parse leg: %v", err)
			}
			lf := map[int]string{}
			for _, x := range legFields {
				lf[x.Field] = string(x.Bytes)
			}
			if lf[1] != "JFK" || lf[3] != "LAX" {
				t.Errorf("leg airports = %q -> %q", lf[1], lf[3])
			}
			sawLeg = true
		}
	}
	if !sawDate || !sawLeg {
		t.Errorf("segment missing date=%v or leg=%v", sawDate, sawLeg)
	}
}

func TestEncodeTFSRoundTripTwoWay(t *testing.T) {
	t.Parallel()
	token, err := EncodeTFS(TFSRequest{
		Segments: []TFSSegment{
			{Legs: []TFSLeg{{Origin: "SGN", Dest: "HAN", Date: mustDate(t, "2026-10-01")}}},
			{Legs: []TFSLeg{{Origin: "HAN", Dest: "SGN", Date: mustDate(t, "2026-10-08")}}},
		},
	})
	if err != nil {
		t.Fatalf("EncodeTFS: %v", err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(token)
	fields, err := parseMessage(raw)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	var segs, tripType int
	for _, f := range fields {
		if f.Field == 3 {
			segs++
		}
		if f.Field == 19 {
			tripType = int(f.Varint)
		}
	}
	if segs != 2 {
		t.Errorf("got %d segments, want 2", segs)
	}
	if tripType != 1 {
		t.Errorf("trip type = %d, want 1 (round trip)", tripType)
	}
}

func TestEncodeTFSErrors(t *testing.T) {
	t.Parallel()
	if _, err := EncodeTFS(TFSRequest{}); err == nil {
		t.Error("expected error for no segments")
	}
	if _, err := EncodeTFS(TFSRequest{Segments: []TFSSegment{{}}}); err == nil {
		t.Error("expected error for segment with no legs")
	}
}
