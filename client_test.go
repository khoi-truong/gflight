package gflight_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/khoi-truong/gflight"
)

func TestNewAppliesOptions(t *testing.T) {
	t.Parallel()

	custom := &http.Client{Timeout: time.Second}

	tests := []struct {
		name          string
		opts          []gflight.Option
		wantUserAgent string
		wantBaseURL   string
		wantHTTP      *http.Client
	}{
		{
			name:          "defaults",
			wantUserAgent: gflight.DefaultUserAgent,
			wantBaseURL:   gflight.DefaultBaseURL,
		},
		{
			name:          "overrides",
			opts:          []gflight.Option{gflight.WithUserAgent("ua/1"), gflight.WithBaseURL("http://example.test/"), gflight.WithHTTPClient(custom)},
			wantUserAgent: "ua/1",
			wantBaseURL:   "http://example.test",
			wantHTTP:      custom,
		},
		{
			name:          "empty and nil values are ignored",
			opts:          []gflight.Option{gflight.WithUserAgent(""), gflight.WithBaseURL(""), gflight.WithHTTPClient(nil), gflight.WithLogger(nil)},
			wantUserAgent: gflight.DefaultUserAgent,
			wantBaseURL:   gflight.DefaultBaseURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := gflight.New(tt.opts...)
			if got := c.UserAgent(); got != tt.wantUserAgent {
				t.Errorf("UserAgent() = %q, want %q", got, tt.wantUserAgent)
			}
			if got := c.BaseURL(); got != tt.wantBaseURL {
				t.Errorf("BaseURL() = %q, want %q", got, tt.wantBaseURL)
			}
			if c.HTTPClient() == nil {
				t.Fatal("HTTPClient() = nil, want non-nil")
			}
			if tt.wantHTTP != nil && c.HTTPClient() != tt.wantHTTP {
				t.Errorf("HTTPClient() = %p, want %p", c.HTTPClient(), tt.wantHTTP)
			}
		})
	}
}

func TestSearchNotImplemented(t *testing.T) {
	t.Parallel()

	c := gflight.New()

	got, err := c.Search(t.Context(), gflight.SearchRequest{Origin: "SGN", Destination: "HAN"})
	if got != nil {
		t.Errorf("Search() itineraries = %v, want nil", got)
	}
	if !errors.Is(err, gflight.ErrNotImplemented) {
		t.Fatalf("Search() error = %v, want %v", err, gflight.ErrNotImplemented)
	}
}

func TestSearchHonoursCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := gflight.New().Search(ctx, gflight.SearchRequest{Origin: "SGN", Destination: "HAN"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search() error = %v, want %v", err, context.Canceled)
	}
}

func TestHTTPErrorUnwrapsToBadResponse(t *testing.T) {
	t.Parallel()

	var err error = &gflight.HTTPError{StatusCode: 429, Status: "429 Too Many Requests", URL: "http://example.test/x"}

	if !errors.Is(err, gflight.ErrBadResponse) {
		t.Errorf("errors.Is(%v, ErrBadResponse) = false, want true", err)
	}

	var httpErr *gflight.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("errors.As(%v, *HTTPError) = false, want true", err)
	}
	if httpErr.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", httpErr.StatusCode)
	}
	if httpErr.Error() == "" {
		t.Error("Error() = empty string, want a message")
	}
}
