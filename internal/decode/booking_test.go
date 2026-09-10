package decode

import (
	"errors"
	"testing"
)

// bookingRow builds an option row with only the slots the decoder reads set,
// so a test can knock one out at a time.
func bookingRow(mutate func(row []any)) any {
	row := make([]any, 23)
	row[bookingVendorIdx] = []any{[]any{"AA", "American", nil, true}}
	row[bookingLinkIdx] = []any{"www.aa.com/...", nil, []any{
		"https://www.google.com/travel/clk/f", []any{[]any{"u", "TOKEN"}},
	}}
	row[bookingPriceIdx] = []any{[]any{nil, 347.0}, ""}
	row[bookingFareInfoIdx] = []any{[]any{"AA", "MAIN CABIN"}, nil, nil, "Main Cabin"}
	if mutate != nil {
		mutate(row)
	}
	return row
}

func bookingPayload(rows ...any) any {
	return []any{nil, []any{rows}}
}

func TestBookingOptionsRow(t *testing.T) {
	t.Parallel()
	opts, stats, err := BookingOptions(bookingPayload(bookingRow(nil)))
	if err != nil {
		t.Fatalf("BookingOptions: %v", err)
	}
	if stats.Rows != 1 || stats.Failures != 0 {
		t.Errorf("stats = %+v, want {Rows:1 Failures:0}", stats)
	}
	want := BookingOption{
		VendorCode: "AA",
		VendorName: "American",
		Amount:     347,
		URL:        "https://www.google.com/travel/clk/f?u=TOKEN",
		FareCode:   "MAIN CABIN",
		FareName:   "Main Cabin",
	}
	if opts[0] != want {
		t.Errorf("option = %+v, want %+v", opts[0], want)
	}
}

func TestBookingOptionsCurrencyToken(t *testing.T) {
	t.Parallel()
	// A real booking price token: standard base64 alphabet, so it contains
	// "+", which the URL-safe decoder alone rejects.
	const token = "EglvcHRpb25zOjAaCwj4jgIQAhoDVVNEOChw+I4C"
	opts, _, err := BookingOptions(bookingPayload(bookingRow(func(row []any) {
		row[bookingPriceIdx] = []any{[]any{nil, 347.0}, token}
	})))
	if err != nil {
		t.Fatalf("BookingOptions: %v", err)
	}
	if opts[0].Currency != "USD" {
		t.Errorf("currency = %q, want USD", opts[0].Currency)
	}
}

func TestBookingOptionsSkipsMalformedRows(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(row []any)
	}{
		{"no price block", func(row []any) { row[bookingPriceIdx] = nil }},
		{"unknown price", func(row []any) { row[bookingPriceIdx] = []any{[]any{}, ""} }},
		{"no vendor", func(row []any) { row[bookingVendorIdx] = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, stats, err := BookingOptions(bookingPayload(bookingRow(nil), bookingRow(tc.mutate)))
			if err != nil {
				t.Fatalf("BookingOptions: %v", err)
			}
			if len(opts) != 1 {
				t.Fatalf("got %d options, want 1", len(opts))
			}
			if stats.Rows != 2 || stats.Failures != 1 {
				t.Errorf("stats = %+v, want {Rows:2 Failures:1}", stats)
			}
		})
	}
}

func TestBookingOptionsAllRowsFailed(t *testing.T) {
	t.Parallel()
	_, _, err := BookingOptions(bookingPayload(bookingRow(func(row []any) {
		row[bookingPriceIdx] = nil
	})))
	var allFailed *AllRowsFailedError
	if !errors.As(err, &allFailed) {
		t.Fatalf("err = %v, want *AllRowsFailedError", err)
	}
}

func TestBookingOptionsNoOptionList(t *testing.T) {
	t.Parallel()
	// The first chunk of a real booking response carries no option list.
	for _, payload := range []any{
		[]any{nil},             // inner[1] absent
		[]any{nil, []any{}},    // present but empty
		[]any{nil, []any{nil}}, // element 0 is not a list
	} {
		opts, stats, err := BookingOptions(payload)
		if err != nil || opts != nil || stats.Rows != 0 {
			t.Errorf("BookingOptions(%v) = %v, %+v, %v; want nil, {}, nil", payload, opts, stats, err)
		}
	}
}
