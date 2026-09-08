package encoding

import (
	"encoding/base64"
	"errors"
	"time"
)

// TFSLeg is one physical flight within a [TFSSegment]. For a route search only
// Origin, Dest and Date are set; Airline and FlightNumber pin the leg to a
// specific operated flight (used when deep-linking a chosen itinerary).
type TFSLeg struct {
	Origin       string
	Dest         string
	Date         time.Time
	Airline      string
	FlightNumber string
}

// TFSSegment is one travel direction: outbound, or the return of a round trip.
type TFSSegment struct {
	Legs []TFSLeg
}

// TFSRequest is the itinerary shape encoded into the `tfs` deep-link parameter.
type TFSRequest struct {
	Segments []TFSSegment
	OneWay   bool
}

const tfsDateLayout = "2006-01-02"

// maxUint64 is the f16 constant Google's UI emits verbatim.
const maxUint64 = uint64(1<<64 - 1)

// EncodeTFS builds the base64url (unpadded) protobuf `tfs` query-parameter
// value. Field numbers follow the reverse-engineered layout in
// docs/wire/tfs.md.
func EncodeTFS(req TFSRequest) (string, error) {
	if len(req.Segments) == 0 {
		return "", errors.New("gflight/encoding: tfs needs at least one segment")
	}

	var payload []byte
	payload = appendVarintField(payload, 1, 28)
	payload = appendVarintField(payload, 2, 2)

	for _, seg := range req.Segments {
		if len(seg.Legs) == 0 {
			return "", errors.New("gflight/encoding: tfs segment has no legs")
		}
		segProto, err := encodeTFSSegment(seg)
		if err != nil {
			return "", err
		}
		payload = appendLengthDelim(payload, 3, segProto)
	}

	payload = appendVarintField(payload, 8, 1)
	payload = appendVarintField(payload, 9, 1)
	payload = appendVarintField(payload, 14, 1)
	payload = appendLengthDelim(payload, 16, appendVarintField(nil, 1, maxUint64))
	if req.OneWay {
		payload = appendVarintField(payload, 19, 2)
	} else {
		payload = appendVarintField(payload, 19, 1)
	}

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func encodeTFSSegment(seg TFSSegment) ([]byte, error) {
	origin := seg.Legs[0].Origin
	dest := seg.Legs[len(seg.Legs)-1].Dest
	if origin == "" || dest == "" {
		return nil, errors.New("gflight/encoding: tfs leg missing origin or destination")
	}
	date := seg.Legs[0].Date
	if date.IsZero() {
		return nil, errors.New("gflight/encoding: tfs segment missing date")
	}

	var out []byte
	out = appendLengthDelim(out, 2, []byte(date.Format(tfsDateLayout)))

	for _, leg := range seg.Legs {
		var legProto []byte
		legProto = appendLengthDelim(legProto, 1, []byte(leg.Origin))
		legProto = appendLengthDelim(legProto, 2, []byte(leg.Date.Format(tfsDateLayout)))
		legProto = appendLengthDelim(legProto, 3, []byte(leg.Dest))
		if leg.Airline != "" {
			legProto = appendLengthDelim(legProto, 5, []byte(leg.Airline))
		}
		if leg.FlightNumber != "" {
			legProto = appendLengthDelim(legProto, 6, []byte(leg.FlightNumber))
		}
		out = appendLengthDelim(out, 4, legProto)
	}

	out = appendLengthDelim(out, 13, appendLengthDelim(appendVarintField(nil, 1, 1), 2, []byte(origin)))
	out = appendLengthDelim(out, 14, appendLengthDelim(appendVarintField(nil, 1, 1), 2, []byte(dest)))
	return out, nil
}
