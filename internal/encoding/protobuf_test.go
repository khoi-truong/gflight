package encoding

import (
	"bytes"
	"testing"
)

func TestVarintRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []uint64{0, 1, 127, 128, 300, 16384, 1<<32 - 1, maxUint64}
	for _, want := range cases {
		buf := appendVarint(nil, want)
		got, n := readVarint(buf)
		if n != len(buf) {
			t.Errorf("readVarint(%d): consumed %d of %d bytes", want, n, len(buf))
		}
		if got != want {
			t.Errorf("readVarint round-trip: got %d, want %d", got, want)
		}
	}
}

func TestReadVarintTruncated(t *testing.T) {
	t.Parallel()
	// A continuation byte with nothing after it.
	if _, n := readVarint([]byte{0x80}); n != 0 {
		t.Errorf("truncated varint: got n=%d, want 0", n)
	}
	// Overlong (>10 bytes) is rejected.
	overlong := bytes.Repeat([]byte{0x80}, 11)
	if _, n := readVarint(overlong); n != 0 {
		t.Errorf("overlong varint: got n=%d, want 0", n)
	}
}

func TestParseMessageRoundTrip(t *testing.T) {
	t.Parallel()
	var msg []byte
	msg = appendVarintField(msg, 1, 28)
	msg = appendLengthDelim(msg, 2, []byte("SGN"))
	msg = appendLengthDelim(msg, 3, appendVarintField(nil, 1, 7))

	fields, err := parseMessage(msg)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("got %d fields, want 3", len(fields))
	}
	if fields[0].Field != 1 || fields[0].Wire != 0 || fields[0].Varint != 28 {
		t.Errorf("field 0 = %+v", fields[0])
	}
	if fields[1].Field != 2 || fields[1].Wire != 2 || string(fields[1].Bytes) != "SGN" {
		t.Errorf("field 1 = %+v", fields[1])
	}
	nested, err := parseMessage(fields[2].Bytes)
	if err != nil {
		t.Fatalf("nested parseMessage: %v", err)
	}
	if len(nested) != 1 || nested[0].Field != 1 || nested[0].Varint != 7 {
		t.Errorf("nested = %+v", nested)
	}
}

func TestParseMessageTruncated(t *testing.T) {
	t.Parallel()
	msg := appendLengthDelim(nil, 2, []byte("hello"))
	if _, err := parseMessage(msg[:len(msg)-2]); err == nil {
		t.Fatal("expected error on truncated length-delimited field")
	}
}
