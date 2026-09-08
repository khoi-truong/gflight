package gflight

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapConcurrentOrderAndResults(t *testing.T) {
	t.Parallel()
	in := []int{1, 2, 3, 4, 5}
	out, errs := mapConcurrent(t.Context(), 2, in, func(_ context.Context, n int) (int, error) {
		return n * n, nil
	})
	want := []int{1, 4, 9, 16, 25}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("out[%d] = %d, want %d", i, out[i], want[i])
		}
		if errs[i] != nil {
			t.Errorf("errs[%d] = %v, want nil", i, errs[i])
		}
	}
}

func TestMapConcurrentRespectsLimit(t *testing.T) {
	t.Parallel()
	const limit = 3
	var (
		mu       sync.Mutex
		inFlight int
		peak     int
	)
	in := make([]int, 20)
	_, errs := mapConcurrent(t.Context(), limit, in, func(_ context.Context, _ int) (int, error) {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()
		time.Sleep(2 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		return 0, nil
	})
	for _, err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if peak > limit {
		t.Errorf("peak in-flight = %d, want <= %d", peak, limit)
	}
	if peak == 0 {
		t.Error("fn never ran")
	}
}

func TestMapConcurrentPropagatesErrors(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	in := []int{0, 1, 2, 3}
	out, errs := mapConcurrent(t.Context(), 4, in, func(_ context.Context, n int) (int, error) {
		if n%2 == 1 {
			return 0, sentinel
		}
		return n, nil
	})
	for i, n := range in {
		if n%2 == 1 {
			if !errors.Is(errs[i], sentinel) {
				t.Errorf("errs[%d] = %v, want sentinel", i, errs[i])
			}
		} else if errs[i] != nil || out[i] != n {
			t.Errorf("errs[%d]=%v out[%d]=%d, want nil/%d", i, errs[i], i, out[i], n)
		}
	}
}

func TestMapConcurrentStopsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var ran atomic.Int64
	in := make([]int, 10)
	_, errs := mapConcurrent(ctx, 2, in, func(_ context.Context, _ int) (int, error) {
		ran.Add(1)
		return 0, nil
	})
	if ran.Load() != 0 {
		t.Errorf("fn ran %d times after cancel, want 0", ran.Load())
	}
	for i := range errs {
		if !errors.Is(errs[i], context.Canceled) {
			t.Errorf("errs[%d] = %v, want context.Canceled", i, errs[i])
		}
	}
}

func TestMapConcurrentEmpty(t *testing.T) {
	t.Parallel()
	out, errs := mapConcurrent(t.Context(), 4, nil, func(_ context.Context, n int) (int, error) {
		return n, nil
	})
	if len(out) != 0 || len(errs) != 0 {
		t.Errorf("out=%v errs=%v, want empty", out, errs)
	}
}
