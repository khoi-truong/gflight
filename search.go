package gflight

import (
	"context"
	"fmt"
)

// Search returns the itineraries matching req.
//
// The signature is final; the implementation is not — Search currently returns
// [ErrNotImplemented] for every input. When no itinerary matches it will
// return [ErrNoResults]; when the upstream reply cannot be parsed it will
// return an error wrapping [ErrBadResponse].
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]Itinerary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("search %s->%s: %w", req.Origin, req.Destination, ErrNotImplemented)
}
