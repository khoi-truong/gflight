// Package wire decodes the batchexecute response envelope Google's
// FlightsFrontendService returns. The format is undocumented; the shape below
// was reverse-engineered (see docs/wire/shopping-results.md).
//
// A response body is:
//
//	)]}'\n
//	<frame><frame>...
//
// where each <frame> is EITHER
//
//   - length-prefixed: "<utf8-byte-length>\n<json-array>\n", or
//   - bare: the JSON array alone, with no length line (single-frame replies).
//
// The length is a UTF-8 byte count that covers the newline terminating the
// length line, the JSON, and the newline before the next length line — so the
// JSON itself is length-1 bytes once the header has been consumed.
//
// Every JSON array looks like [["wrb.fr", null, "<payload>", ...], ...]. The
// third element of a "wrb.fr" row is itself a JSON string; that inner string is
// what callers want. This package returns those inner payloads, in order.
package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
)

// ErrShortRead means the body ended in the middle of a length-prefixed frame —
// a truncated download or a cut connection.
var ErrShortRead = errors.New("gflight/wire: truncated response frame")

// ErrNoEnvelope means the body did not start with the )]}' guard, so it is not
// a batchexecute response at all (an HTML error page, say).
var ErrNoEnvelope = errors.New("gflight/wire: missing anti-JSON-hijack prefix")

var antiHijackPrefix = []byte(")]}'")

// StatusError is returned when a "wrb.fr" row carries no payload but does carry
// an upstream status code (row[5] == [N]). Observed codes: 4 (rate limited),
// 13 (request structurally accepted but refused by the app). Callers map this
// to gflight.ErrBlocked.
type StatusError struct{ Code int }

func (e *StatusError) Error() string {
	return "gflight/wire: upstream returned status code " + strconv.Itoa(e.Code)
}

// Payloads extracts the inner "wrb.fr" payload strings from a raw response
// body, in the order they appear. Rows that are not "wrb.fr" (e.g. "di",
// "af.httprm" bookkeeping) are skipped.
func Payloads(body []byte) ([]json.RawMessage, error) {
	body = bytes.TrimPrefix(body, []byte("\n"))
	if !bytes.HasPrefix(body, antiHijackPrefix) {
		return nil, ErrNoEnvelope
	}
	rest := bytes.TrimPrefix(body, antiHijackPrefix)
	rest = bytes.TrimLeft(rest, "\r\n")

	var out []json.RawMessage
	for len(rest) > 0 {
		frame, remainder, err := nextFrame(rest)
		if err != nil {
			return nil, err
		}
		rest = remainder
		if len(bytes.TrimSpace(frame)) == 0 {
			continue
		}

		var rows [][]json.RawMessage
		if err := json.Unmarshal(frame, &rows); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if len(row) < 3 {
				continue
			}
			var tag string
			if err := json.Unmarshal(row[0], &tag); err != nil || tag != "wrb.fr" {
				continue
			}
			var payload string
			if bytes.Equal(bytes.TrimSpace(row[2]), []byte("null")) || json.Unmarshal(row[2], &payload) != nil {
				// Payloadless row: it may still carry a status code in row[5].
				if code, ok := statusCode(row); ok {
					return nil, &StatusError{Code: code}
				}
				continue
			}
			out = append(out, json.RawMessage(payload))
		}
	}
	return out, nil
}

// statusCode reads row[5] as a one-element [N] array, if present.
func statusCode(row []json.RawMessage) (int, bool) {
	if len(row) < 6 {
		return 0, false
	}
	var arr []int
	if err := json.Unmarshal(row[5], &arr); err != nil || len(arr) == 0 {
		return 0, false
	}
	return arr[0], true
}

// nextFrame splits one frame off the front of buf, returning the frame bytes
// and whatever follows. It handles both the length-prefixed and bare forms.
func nextFrame(buf []byte) (frame, rest []byte, err error) {
	nl := bytes.IndexByte(buf, '\n')
	if nl > 0 {
		if n, convErr := strconv.Atoi(string(bytes.TrimSpace(buf[:nl]))); convErr == nil && n >= 0 {
			start := nl + 1
			// The header counts three things: the newline that terminates the
			// header itself, the JSON, and the newline separating this frame
			// from the next header. That first newline is already consumed by
			// start, so the frame is n-1 bytes — reading n swallows the first
			// digit of the next header and desynchronises every frame after it.
			size := max(n-1, 0)
			if start+size > len(buf) {
				return nil, nil, ErrShortRead
			}
			frame = buf[start : start+size]
			rest = bytes.TrimLeft(buf[start+size:], "\r\n")
			return frame, rest, nil
		}
	}
	// Bare frame: the remainder is a single JSON array.
	return bytes.TrimRight(buf, "\r\n"), nil, nil
}
