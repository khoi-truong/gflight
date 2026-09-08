package decode

import (
	"encoding/base64"
	"testing"
)

// wrap returns a length-delimited (wire type 2) protobuf field.
func wrap(field int, payload []byte) []byte {
	tag := byte(field<<3 | 2)
	return append([]byte{tag, byte(len(payload))}, payload...)
}

func TestCurrencyFromToken(t *testing.T) {
	t.Parallel()
	// field 3 { field 3: "usd" }
	msg := wrap(3, wrap(3, []byte("usd")))

	cases := map[string]string{
		"padded":   base64.URLEncoding.EncodeToString(msg),
		"unpadded": base64.RawURLEncoding.EncodeToString(msg),
	}
	for name, tok := range cases {
		if got := currencyFromToken(tok); got != "USD" {
			t.Errorf("%s: currencyFromToken = %q, want USD", name, got)
		}
	}
}

func TestCurrencyFromTokenGarbage(t *testing.T) {
	t.Parallel()
	for _, tok := range []string{"", "!!!not base64!!!", base64.RawURLEncoding.EncodeToString([]byte("\x08\x01"))} {
		if got := currencyFromToken(tok); got != "" {
			t.Errorf("currencyFromToken(%q) = %q, want empty", tok, got)
		}
	}
}
