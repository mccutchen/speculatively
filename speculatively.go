/*
Package speculatively provides a simple mechanism to speculatively execute a
task in parallel only after some initial timeout has elapsed.
*/
package speculatively

import (
	"context"
	"time"
)

// Thunk is a computation to be speculatively executed
type Thunk[T any] func(context.Context) (T, error)

// Do speculatively executes a Thunk one or more times in parallel, waiting for
// the given patience duration between subsequent executions.
//
// Note that for Do to respect context cancelations, the given Thunk must
// respect them.
func Do[T any](ctx context.Context, patience time.Duration, thunk Thunk[T]) (T, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	out := make(chan result[T])
	go runThunk(ctx, thunk, out)

	ticker := time.NewTicker(patience)
	defer ticker.Stop()

	for step := 1; ; step++ {
		select {
		case r := <-out:
			return r.val, r.err
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-ticker.C:
			go runThunk(ctx, thunk, out)
		}
	}
}

type result[T any] struct {
	val T
	err error
}

func runThunk[T any](ctx context.Context, thunk Thunk[T], out chan result[T]) {
	var r result[T]
	r.val, r.err = thunk(ctx)
	select {
	case out <- r:
	default:
	}
}
