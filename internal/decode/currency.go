package decode

import (
	"encoding/base64"
	"strings"
)

// currencyFromToken extracts the ISO 4217 code from a Google Flights price
// token. The token is unpadded base64 protobuf; the currency is at field 3 →
// nested field 3. Returns "" on any decode failure — the currency is optional
// metadata and never worth failing a row over.
//
// Shopping tokens use the URL-safe alphabet, booking tokens the standard one
// (they contain "+"), so both are tried.
func currencyFromToken(token string) string {
	if token == "" {
		return ""
	}
	raw, err := decodeBase64Any(strings.TrimRight(token, "="))
	if err != nil {
		return ""
	}
	nested := protoField(raw, 3)
	if nested == nil {
		return ""
	}
	code := protoField(nested, 3)
	if code == nil {
		return ""
	}
	return strings.ToUpper(string(code))
}

// decodeBase64Any decodes unpadded base64 in either the URL-safe or the
// standard alphabet.
func decodeBase64Any(s string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// protoField returns the length-delimited (wire type 2) payload of the first
// field numbered want in buf, or nil. Only the wire types Google's price token
// uses (0 and 2) are understood; anything else aborts the walk.
func protoField(buf []byte, want int) []byte {
	for len(buf) > 0 {
		tag, n := uvarint(buf)
		if n == 0 {
			return nil
		}
		buf = buf[n:]
		field := int(tag >> 3)
		switch tag & 7 {
		case 0:
			_, n := uvarint(buf)
			if n == 0 {
				return nil
			}
			buf = buf[n:]
		case 2:
			length, n := uvarint(buf)
			if n == 0 || uint64(len(buf)-n) < length {
				return nil
			}
			buf = buf[n:]
			if field == want {
				return buf[:length]
			}
			buf = buf[length:]
		default:
			return nil
		}
	}
	return nil
}

func uvarint(buf []byte) (uint64, int) {
	var v uint64
	for i, b := range buf {
		if i == 10 {
			return 0, 0
		}
		v |= uint64(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			return v, i + 1
		}
	}
	return 0, 0
}
