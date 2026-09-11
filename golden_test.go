package gflight_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "regenerate golden files in internal/testdata/golden")

// TestSearchGolden pins the full decoded shape of a search against a checked-in
// snapshot. It guards against silent regressions in the positional row decoder
// that a field-by-field assertion would miss. Regenerate with:
//
//	go test ./... -run TestSearchGolden -update
func TestSearchGolden(t *testing.T) {
	t.Parallel()
	cases := []string{
		"shopping_results_oneway_jfk_lax",
		"shopping_results_no_price",
		"shopping_results_layover_buf_ath",
		"shopping_results_multichunk",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := serveFixture(t, name+".txt", 200)
			its, err := c.Search(t.Context(), sampleRequest())
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			got, err := json.MarshalIndent(its, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got = append(got, '\n')

			path := filepath.Join("internal", "testdata", "golden", name+".json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("decoded itineraries differ from %s\n--- got ---\n%s", path, got)
			}
		})
	}
}
