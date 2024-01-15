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
type Thunk func(context.Context) (interface{}, error)

// Do speculatively executes a Thunk one or more times in parallel, waiting for
// the given patience duration between subsequent executions.
//
// Note that for Do to respect context cancelations, the given Thunk must
// respect them.
func Do(ctx context.Context, patience time.Duration, thunk Thunk) (interface{}, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	out := make(chan result)
	go runThunk(ctx, thunk, out)

	ticker := time.NewTicker(patience)
	defer ticker.Stop()

	for step := 1; ; step++ {
		select {
		case r := <-out:
			return r.val, r.err
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			go runThunk(ctx, thunk, out)
		}
	}
}

type result struct {
	val interface{}
	err error
}

func runThunk(ctx context.Context, thunk Thunk, out chan result) {
	var r result
	r.val, r.err = thunk(ctx)
	select {
	case out <- r:
	default:
	}
}
