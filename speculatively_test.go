package speculatively

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type testThunk struct {
	results []result
	delays  []time.Duration
	count   int
	mu      sync.Mutex
}

func (t *testThunk) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

func (t *testThunk) call(ctx context.Context) (interface{}, error) {
	t.mu.Lock()
	id := t.count
	d := t.delays[id%len(t.delays)]
	r := t.results[id%len(t.results)]
	t.count++
	t.mu.Unlock()

	select {
	case <-time.After(d):
		return r.val, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func newTestThunk(results []result, delays []time.Duration) *testThunk {
	return &testThunk{
		results: results,
		delays:  delays,
	}
}

func newSimpleTestThunk(val interface{}, err error, delay time.Duration) *testThunk {
	results := []result{
		{val: val, err: err},
	}
	delays := []time.Duration{delay}
	return newTestThunk(results, delays)
}

func TestSingleThunk(t *testing.T) {
	thunk := newSimpleTestThunk(1, nil, 5*time.Millisecond)
	patience := 20 * time.Millisecond

	result, err := Do(context.Background(), patience, thunk.call)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	val := result.(int)
	if val != 1 {
		t.Errorf("expected val = %d, got %d", 1, val)
	}
	if callCount := thunk.callCount(); callCount != 1 {
		t.Errorf("expected Thunk to run once, got %d", callCount)
	}
}

func TestSpeculativeThunkStarted(t *testing.T) {
	thunk := newSimpleTestThunk(1, nil, 75*time.Millisecond)

	timeout := 125 * time.Millisecond
	patience := 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := Do(ctx, patience, thunk.call)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	val := result.(int)
	if val != 1 {
		t.Errorf("expected val = %d, got %d", 1, val)
	}
	if callCount := thunk.callCount(); callCount != 2 {
		t.Errorf("expected Thunk to run %d times, got %d", 2, callCount)
	}
}

func TestSpeculativeThunkFinishesFirst(t *testing.T) {
	results := []result{
		{val: 1, err: nil},
		{val: 2, err: nil},
	}
	delays := []time.Duration{
		5000 * time.Millisecond,
		10 * time.Millisecond,
	}
	thunk := newTestThunk(results, delays)

	timeout := 100 * time.Millisecond
	patience := 25 * time.Millisecond

	// The first thunk will take too long, so we expect the second value to be
	// returned
	expectedValue := results[1].val

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := Do(ctx, patience, thunk.call)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	val := result.(int)
	if val != expectedValue {
		t.Errorf("expected val = %d, got %d", expectedValue, val)
	}
	if callCount := thunk.callCount(); callCount != 2 {
		t.Errorf("expected Thunk to run %d times, got %d", 2, callCount)
	}
}

func TestSpeculativeErrors(t *testing.T) {

	t.Run("first task finishes first with error", func(t *testing.T) {
		results := []result{
			{val: 1, err: errors.New("error")},
			{val: 2, err: nil},
		}
		delays := []time.Duration{
			20 * time.Millisecond,
			200 * time.Millisecond,
		}
		thunk := newTestThunk(results, delays)

		timeout := 50 * time.Millisecond
		patience := 10 * time.Millisecond

		expectedErr := results[0].err

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		_, err := Do(ctx, patience, thunk.call)
		if err != expectedErr {
			t.Errorf("expected err = %s, got %s", expectedErr, err)
		}
	})

	t.Run("first task finishes second with error", func(t *testing.T) {
		results := []result{
			{val: 1, err: errors.New("error")},
			{val: 2, err: nil},
		}
		delays := []time.Duration{
			200 * time.Millisecond,
			20 * time.Millisecond,
		}
		thunk := newTestThunk(results, delays)

		timeout := 50 * time.Millisecond
		patience := 10 * time.Millisecond

		// The first thunk will take too long, so we expect the second value to be
		// returned
		expectedVal := results[1].val

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		result, err := Do(ctx, patience, thunk.call)
		if err != nil {
			t.Fatalf("unexpected error: %q", err)
		}

		val := result.(int)
		if val != expectedVal {
			t.Errorf("expected val = %#v, got %#v", expectedVal, val)
		}
	})
}

func TestContextPropagated(t *testing.T) {
	thunk := newSimpleTestThunk(1, nil, 10*time.Second)

	timeout := 25 * time.Millisecond
	patience := 15 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := Do(ctx, patience, thunk.call)
	if err == nil {
		t.Fatalf("expected context cancelation, got result %#v with no error", result)
	}
	if callCount := thunk.callCount(); callCount != 2 {
		t.Errorf("expected Thunk to run %d times, got %d", 2, callCount)
	}
}
