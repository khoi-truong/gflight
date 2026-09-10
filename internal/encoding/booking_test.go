package encoding

import (
	"testing"
	"time"
)

func bookingRequest() FreqRequest {
	return FreqRequest{
		Segments: []FreqSegment{{
			Origin: "JFK", Dest: "LAX",
			Date: time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC),
		}},
		Adults: 2,
	}
}

func TestEncodeBooking(t *testing.T) {
	t.Parallel()
	enc, err := EncodeBooking(bookingRequest(), "tok-123")
	if err != nil {
		t.Fatalf("EncodeBooking: %v", err)
	}
	body := decodeFreq(t, enc)

	if len(body) != 2 {
		t.Fatalf("body has %d elements, want 2: %v", len(body), body)
	}
	head, ok := body[0].([]any)
	if !ok || len(head) != 2 || head[0] != nil || head[1] != "tok-123" {
		t.Fatalf("body[0] = %v, want [null, \"tok-123\"]", body[0])
	}
	main, ok := body[1].([]any)
	if !ok {
		t.Fatalf("body[1] is not a list: %v", body[1])
	}
	if len(main) != bookingMainLen {
		t.Fatalf("main has %d slots, want %d", len(main), bookingMainLen)
	}
	// The trimmed block must still carry the settings the booking call prices
	// against: trip type, cabin and passengers.
	if got := main[mainTripTypeIdx]; got != float64(tripOneWay) {
		t.Errorf("trip type = %v, want %d", got, tripOneWay)
	}
	if got := main[mainCabinIdx]; got != float64(cabinEconomy) {
		t.Errorf("cabin = %v, want %d", got, cabinEconomy)
	}
	pax, ok := main[mainPassengersIdx].([]any)
	if !ok || len(pax) != 4 || pax[0] != float64(2) {
		t.Errorf("passengers = %v, want [2 0 0 0]", main[mainPassengersIdx])
	}
	if got := main[mainConstant17Idx]; got != float64(1) {
		t.Errorf("main[17] = %v, want 1", got)
	}
}

func TestEncodeBookingErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		req   FreqRequest
		token string
	}{
		{"no token", bookingRequest(), ""},
		{"no segments", FreqRequest{}, "tok-123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := EncodeBooking(tc.req, tc.token); err == nil {
				t.Fatal("EncodeBooking: want error, got nil")
			}
		})
	}
}
