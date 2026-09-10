package wire

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestPayloadsFixtures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file  string
		count int
	}{
		{"shopping_results_oneway_jfk_lax.txt", 1},
		{"shopping_results_no_price.txt", 1},
		{"shopping_results_no_results.txt", 1},
		{"shopping_results_layover_buf_ath.txt", 1},
		{"shopping_results_multichunk.txt", 3},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			payloads, err := Payloads(fixture(t, tc.file))
			if err != nil {
				t.Fatalf("Payloads: %v", err)
			}
			if len(payloads) != tc.count {
				t.Errorf("got %d payloads, want %d", len(payloads), tc.count)
			}
			for i, p := range payloads {
				if len(p) == 0 {
					t.Errorf("payload %d is empty", i)
				}
			}
		})
	}
}

func TestPayloadsStatusError(t *testing.T) {
	t.Parallel()
	_, err := Payloads(fixture(t, "shopping_results_error_429.txt"))
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *StatusError", err)
	}
	if se.Code != 4 {
		t.Errorf("status code = %d, want 4", se.Code)
	}
}

func TestPayloadsTruncated(t *testing.T) {
	t.Parallel()
	_, err := Payloads(fixture(t, "shopping_results_truncated_chunk.txt"))
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("err = %v, want ErrShortRead", err)
	}
}

func TestPayloadsNoEnvelope(t *testing.T) {
	t.Parallel()
	_, err := Payloads([]byte("<html>nope</html>"))
	if !errors.Is(err, ErrNoEnvelope) {
		t.Fatalf("err = %v, want ErrNoEnvelope", err)
	}
}

func TestPayloadsEmptyEnvelope(t *testing.T) {
	t.Parallel()
	payloads, err := Payloads([]byte(")]}'\n\n"))
	if err != nil {
		t.Fatalf("Payloads: %v", err)
	}
	if len(payloads) != 0 {
		t.Errorf("got %d payloads, want 0", len(payloads))
	}
}

// TestPayloadsFrameLengthConvention pins how the length header is counted:
// header newline + JSON + separating newline. The body below is built to that
// convention, so a reader that consumes the full declared length instead of
// length-1 eats the leading "4" of the second header, reads frame 2 as 5369
// bytes, and fails. Both frames must come back intact.
func TestPayloadsFrameLengthConvention(t *testing.T) {
	t.Parallel()
	first := `[["wrb.fr",null,"first"]]`
	second := `[["wrb.fr",null,"second"]]`
	body := ")]}'\n\n" +
		strconv.Itoa(len(first)+2) + "\n" + first + "\n" +
		strconv.Itoa(len(second)+2) + "\n" + second + "\n"

	payloads, err := Payloads([]byte(body))
	if err != nil {
		t.Fatalf("Payloads: %v", err)
	}
	want := []string{"first", "second"}
	if len(payloads) != len(want) {
		t.Fatalf("got %d payloads, want %d", len(payloads), len(want))
	}
	for i, w := range want {
		if string(payloads[i]) != w {
			t.Errorf("payload %d = %q, want %q", i, payloads[i], w)
		}
	}
}

func FuzzChunks(f *testing.F) {
	for _, name := range []string{
		"shopping_results_oneway_jfk_lax.txt",
		"shopping_results_error_429.txt",
		"shopping_results_truncated_chunk.txt",
		"shopping_results_multichunk.txt",
	} {
		if b, err := os.ReadFile(filepath.Join("..", "testdata", name)); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte(")]}'\n\n5\n[[\"wrb.fr\",null,\"x\"]]\n"))
	f.Add([]byte(""))

	f.Fuzz(func(_ *testing.T, body []byte) {
		// Contract: never panics, whatever the bytes.
		_, _ = Payloads(body)
	})
}
