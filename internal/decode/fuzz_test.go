package decode

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/khoi-truong/gflight/internal/wire"
)

// FuzzDecodeRow asserts that parseRow never panics, whatever JSON shape the
// upstream sends. Real rows from every fixture seed the corpus; the fuzzer then
// mutates them into arbitrary nested arrays, nulls and scalars.
func FuzzDecodeRow(f *testing.F) {
	for _, name := range []string{
		"shopping_results_oneway_jfk_lax.txt",
		"shopping_results_no_price.txt",
		"shopping_results_layover_buf_ath.txt",
		"shopping_results_multichunk.txt",
	} {
		body, err := os.ReadFile(filepath.Join("..", "testdata", name))
		if err != nil {
			continue
		}
		payloads, err := wire.Payloads(body)
		if err != nil {
			continue
		}
		for _, p := range payloads {
			var inner any
			if json.Unmarshal(p, &inner) != nil {
				continue
			}
			for _, i := range []int{2, 3} {
				block := at(inner, i)
				if !isSlice(block) {
					continue
				}
				for _, row := range sliceOf(at(block, 0)) {
					if b, err := json.Marshal(row); err == nil {
						f.Add(b)
					}
				}
			}
		}
	}
	f.Add([]byte(`null`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`[[],[[[]]],"x",1,true,null]`))
	f.Add([]byte(`{"not":"an array"}`))

	f.Fuzz(func(_ *testing.T, data []byte) {
		var row any
		if json.Unmarshal(data, &row) != nil {
			return
		}
		_, _ = parseRow(row)
	})
}

// FuzzCurrencyToken asserts that currencyFromToken never panics on arbitrary
// input — the price token is untrusted upstream base64 protobuf.
func FuzzCurrencyToken(f *testing.F) {
	f.Add("")
	f.Add("!!!not base64!!!")
	f.Add(base64.RawURLEncoding.EncodeToString(wrap(3, wrap(3, []byte("usd")))))
	f.Add(base64.RawURLEncoding.EncodeToString([]byte("\x08\x01")))

	f.Fuzz(func(_ *testing.T, token string) {
		_ = currencyFromToken(token)
	})
}
