package speculatively

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testThunk struct {
	results []result[int]
	delays  []time.Duration
	count   int
	mu      sync.Mutex
}

func (t *testThunk) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

func (t *testThunk) call(ctx context.Context) (int, error) {
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
		return 0, ctx.Err()
	}
}

func newTestThunk(results []result[int], delays []time.Duration) *testThunk {
	return &testThunk{
		results: results,
		delays:  delays,
	}
}

func newSimpleTestThunk(val int, err error, delay time.Duration) *testThunk {
	results := []result[int]{
		{val: val, err: err},
	}
	delays := []time.Duration{delay}
	return newTestThunk(results, delays)
}

func TestSingleThunk(t *testing.T) {
	t.Parallel()

	thunk := newSimpleTestThunk(1, nil, 5*time.Millisecond)
	patience := 20 * time.Millisecond

	val, err := Do(context.Background(), patience, thunk.call)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if val != 1 {
		t.Errorf("expected val = %d, got %d", 1, val)
	}
	if callCount := thunk.callCount(); callCount != 1 {
		t.Errorf("expected Thunk to run once, got %d", callCount)
	}
}

func TestSpeculativeThunkStarted(t *testing.T) {
	t.Parallel()

	thunk := newSimpleTestThunk(1, nil, 75*time.Millisecond)

	timeout := 125 * time.Millisecond
	patience := 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	val, err := Do(ctx, patience, thunk.call)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if val != 1 {
		t.Errorf("expected val = %d, got %d", 1, val)
	}
	if callCount := thunk.callCount(); callCount != 2 {
		t.Errorf("expected Thunk to run %d times, got %d", 2, callCount)
	}
}

func TestSpeculativeThunkFinishesFirst(t *testing.T) {
	t.Parallel()

	results := []result[int]{
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

	val, err := Do(ctx, patience, thunk.call)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if val != expectedValue {
		t.Errorf("expected val = %d, got %d", expectedValue, val)
	}
	if callCount := thunk.callCount(); callCount != 2 {
		t.Errorf("expected Thunk to run %d times, got %d", 2, callCount)
	}
}

func TestSpeculativeErrors(t *testing.T) {
	t.Run("first task finishes first with error", func(t *testing.T) {
		t.Parallel()

		results := []result[int]{
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
		t.Parallel()

		results := []result[int]{
			{val: 0, err: errors.New("error")},
			{val: 2, err: nil},
		}
		delays := []time.Duration{
			200 * time.Millisecond,
			20 * time.Millisecond,
		}
		thunk := newTestThunk(results, delays)

		patience := 20 * time.Millisecond
		timeout := 100 * time.Millisecond

		// The first thunk will take too long, so we expect the second value to be
		// returned
		expectedVal := results[1].val

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		val, err := Do(ctx, patience, thunk.call)
		if err != nil {
			t.Fatalf("unexpected error: %q", err)
		}
		if val != expectedVal {
			t.Errorf("expected val = %#v, got %#v", expectedVal, val)
		}
	})
}

func TestContextPropagated(t *testing.T) {
	t.Parallel()

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

func TestCompletionCancelsOutstandingThunks(t *testing.T) {
	t.Parallel()

	var (
		timeout     = 75 * time.Millisecond
		duration    = 50 * time.Millisecond
		patience    = 20 * time.Millisecond
		callCount   int64
		cancelCount int64
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup
	result, err := Do(ctx, patience, func(ctx context.Context) (interface{}, error) {
		wg.Add(1)
		defer wg.Done()

		inflight := atomic.AddInt64(&callCount, 1)
		t.Logf("testThunk: starting call %d", inflight)
		select {
		case <-ctx.Done():
			t.Logf("testThunk: call %d canceled", inflight)
			if ctx.Err() != context.Canceled {
				t.Fatalf("unexpected context error in thunk %d: %s", inflight, ctx.Err())
			}
			atomic.AddInt64(&cancelCount, 1)
			return nil, ctx.Err()
		case <-time.After(duration):
			return 42, nil
		}
	})

	wg.Wait()

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	val, ok := result.(int)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result)
	}
	if val != 42 {
		t.Fatalf("unexpected result value: %d != 42", val)
	}

	if callCount != 3 {
		t.Fatalf("unexpected call count: %d", callCount)
	}
	if cancelCount != callCount-1 {
		t.Fatalf("unexpected cancel count: %d != %d", cancelCount, callCount-1)
	}
}
