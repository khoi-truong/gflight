package gflight

import (
	"context"
	"sync"
)

// mapConcurrent applies fn to every element of in with at most limit calls in
// flight at once, and returns the results and errors in input order: out[i] and
// errs[i] come from in[i]. A limit below 1 is treated as 1.
//
// It does not cancel fn on the first error — each input is always attempted —
// but it stops launching new work once ctx is done, leaving errs[i] as
// ctx.Err() for every input not yet started. It is the hand-rolled,
// dependency-free stand-in for an errgroup with SetLimit.
func mapConcurrent[T, R any](ctx context.Context, limit int, in []T, fn func(context.Context, T) (R, error)) ([]R, []error) {
	out := make([]R, len(in))
	errs := make([]error, len(in))
	if len(in) == 0 {
		return out, errs
	}
	if limit < 1 {
		limit = 1
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, item := range in {
		if err := ctx.Err(); err != nil {
			errs[i] = err
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			errs[i] = ctx.Err()
			continue
		}
		wg.Go(func() {
			defer func() { <-sem }()
			out[i], errs[i] = fn(ctx, item)
		})
	}
	wg.Wait()
	return out, errs
}
