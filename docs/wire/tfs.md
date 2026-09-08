# Wire format: the `tfs` deep-link parameter

`tfs` is the token in a shareable `google.com/travel/flights?tfs=...` URL. It is
base64url with **no padding**, wrapping a protobuf message. `gflight` builds it
in `internal/encoding/tfs.go` (`EncodeTFS`) with a hand-rolled encoder — no
protobuf runtime dependency — and `SearchURL` in the root package is the public
entry point. Pure; no network.

Field-number table (merged from the MomoDeve gist and AWeirdDev's
`flights.proto`; see [`../ACKNOWLEDGEMENTS.md`](../ACKNOWLEDGEMENTS.md)):

```text
message TFS {
  1  int    = 28            constant
  2  int    = 2             constant
  3  Segment  (repeated)    one per journey leg group
  8  int    = 1             constant
  9  int    = 1             constant
  14 int    = 1             constant
  16 { 1: uint64 = MaxUint64 }
  19 int                    trip type: 2 = one-way / multi-city, 1 = round-trip
}

message Segment {
  2  string               date "YYYY-MM-DD"
  4  Leg  (repeated)       flights in this segment
  13 { 1: 1, 2: string }  origin  IATA
  14 { 1: 1, 2: string }  dest    IATA
}

message Leg {
  1  string   origin IATA
  2  string   date "YYYY-MM-DD"
  3  string   dest IATA
  5  string   airline      (omitted for a plain route search)
  6  string   flight number (omitted for a plain route search)
}
```

Wire-tag emission order matters for a byte-exact match against a
browser-captured token: `1, 2, 3…, 8, 9, 14, 16, 19`, and within a segment
`2, 4…, 13, 14`.

The golden test in `tfs_test.go` currently freezes a token that is
**round-trip-verified structurally** but is not a live capture — Google's
endpoint is unreachable for a byte comparison. See the deviations section of
`docs/plans/porting.md`.
