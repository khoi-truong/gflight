package gflight_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/khoi-truong/gflight"
)

// serveBookingFixture answers every request with the named fixture and asserts
// the call went to GetBookingResults with a booking token in the body.
func serveBookingFixture(t *testing.T, name string, status int) *gflight.Client {
	t.Helper()
	body := fixtureBytes(t, name)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "GetBookingResults") {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if freq := r.PostForm.Get("f.req"); !strings.Contains(freq, "tok-123") {
			t.Errorf("f.req does not carry the booking token: %s", freq)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return gflight.New(gflight.WithBaseURL(srv.URL), gflight.WithHTTPClient(srv.Client()))
}

func sampleItinerary() gflight.Itinerary {
	return gflight.Itinerary{BookingToken: "tok-123"}
}

func TestBookingOptions(t *testing.T) {
	t.Parallel()
	c := serveBookingFixture(t, "booking_results_aa_jfk_lax.txt", http.StatusOK)

	opts, err := c.BookingOptions(t.Context(), sampleRequest(), sampleItinerary())
	if err != nil {
		t.Fatalf("BookingOptions: %v", err)
	}
	want := []gflight.BookingOption{
		{Vendor: "American", VendorCode: "AA", Price: gflight.Price{Amount: 347, Currency: "USD"},
			FareName: "Basic Economy", FareCode: "BASIC ECONOMY"},
		{Vendor: "American", VendorCode: "AA", Price: gflight.Price{Amount: 457, Currency: "USD"},
			FareName: "Main Cabin", FareCode: "MAIN CABIN"},
		{Vendor: "American", VendorCode: "AA", Price: gflight.Price{Amount: 649, Currency: "USD"},
			FareName: "Main Plus", FareCode: "MAIN PLUS"},
	}
	if len(opts) != len(want) {
		t.Fatalf("got %d options, want %d: %+v", len(opts), len(want), opts)
	}
	for i, w := range want {
		got := opts[i]
		if !strings.HasPrefix(got.URL, "https://www.google.com/travel/clk/f?u=") {
			t.Errorf("option %d URL = %q, want a click-out link", i, got.URL)
		}
		got.URL = ""
		if got != w {
			t.Errorf("option %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestBookingOptionsNeedsToken(t *testing.T) {
	t.Parallel()
	c := gflight.New()

	_, err := c.BookingOptions(t.Context(), sampleRequest(), gflight.Itinerary{})
	if !errors.Is(err, gflight.ErrBadResponse) {
		t.Fatalf("err = %v, want ErrBadResponse", err)
	}
}

// TestBookingOptionsNoResults covers a well-formed reply whose option list is
// empty — Google found the itinerary but nobody sells it.
func TestBookingOptionsNoResults(t *testing.T) {
	t.Parallel()
	inner := `[[null,null,0,\"sid\",\"sid\"],[[]]]`
	frame := `[["wrb.fr",null,"` + inner + `"]]`
	body := ")]}'\n\n" + strconv.Itoa(len(frame)+2) + "\n" + frame + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := gflight.New(gflight.WithBaseURL(srv.URL), gflight.WithHTTPClient(srv.Client()))

	_, err := c.BookingOptions(t.Context(), sampleRequest(), sampleItinerary())
	if !errors.Is(err, gflight.ErrNoResults) {
		t.Fatalf("err = %v, want ErrNoResults", err)
	}
}

func TestBookingOptionsBlocked(t *testing.T) {
	t.Parallel()
	c := serveBookingFixture(t, "shopping_results_error_429.txt", http.StatusOK)

	_, err := c.BookingOptions(t.Context(), sampleRequest(), sampleItinerary())
	if !errors.Is(err, gflight.ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}
