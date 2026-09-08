package encoding

import "errors"

// Minimal protobuf wire-format primitives — just the three cases the Google
// Flights `tfs` deep-link token needs: varints, length-delimited fields, and
// nested messages. Hand-rolled so the module keeps a bare go.mod with no
// require block; see docs/plans/porting.md.

// appendVarint appends v to dst as a base-128 varint (protobuf wire type 0).
func appendVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

// appendTag appends the field tag (field number << 3 | wireType).
func appendTag(dst []byte, field int, wireType int) []byte {
	return appendVarint(dst, uint64(field)<<3|uint64(wireType))
}

// appendVarintField appends a wire-type-0 field.
func appendVarintField(dst []byte, field int, v uint64) []byte {
	return appendVarint(appendTag(dst, field, 0), v)
}

// appendLengthDelim appends a wire-type-2 field carrying payload (a UTF-8
// string, raw bytes, or a nested encoded message).
func appendLengthDelim(dst []byte, field int, payload []byte) []byte {
	dst = appendTag(dst, field, 2)
	dst = appendVarint(dst, uint64(len(payload)))
	return append(dst, payload...)
}

// pbField is one entry read back by parseMessage.
type pbField struct {
	Field int
	Wire  int
	// Varint holds the value for wire type 0; Bytes holds the payload for
	// wire type 2. Only one is meaningful per Wire.
	Varint uint64
	Bytes  []byte
}

var errTruncatedProto = errors.New("gflight/encoding: truncated protobuf")

// parseMessage decodes buf into a flat list of fields. It only understands
// wire types 0 and 2 (the two this package emits); any other wire type is an
// error. It exists for the round-trip tests in protobuf_test.go.
func parseMessage(buf []byte) ([]pbField, error) {
	var out []pbField
	for len(buf) > 0 {
		tag, n := readVarint(buf)
		if n == 0 {
			return nil, errTruncatedProto
		}
		buf = buf[n:]
		f := pbField{Field: int(tag >> 3), Wire: int(tag & 0x7)}
		switch f.Wire {
		case 0:
			v, n := readVarint(buf)
			if n == 0 {
				return nil, errTruncatedProto
			}
			f.Varint = v
			buf = buf[n:]
		case 2:
			length, n := readVarint(buf)
			if n == 0 {
				return nil, errTruncatedProto
			}
			buf = buf[n:]
			if uint64(len(buf)) < length {
				return nil, errTruncatedProto
			}
			f.Bytes = buf[:length]
			buf = buf[length:]
		default:
			return nil, errors.New("gflight/encoding: unsupported wire type")
		}
		out = append(out, f)
	}
	return out, nil
}

// readVarint decodes a varint from the front of buf, returning the value and
// the number of bytes consumed (0 on truncation or overflow).
func readVarint(buf []byte) (uint64, int) {
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
