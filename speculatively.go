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
// the given patience duration between subsequent executions
func Do(ctx context.Context, patience time.Duration, thunk Thunk) (interface{}, error) {
	out := make(chan result)
	done := make(chan struct{})
	defer close(done)

	go runThunk(ctx, thunk, out, done)

	go func() {
		for {
			select {
			case <-time.After(patience):
				go runThunk(ctx, thunk, out, done)
			case <-done:
				return
			}
		}
	}()

	r := <-out
	return r.val, r.err
}

type result struct {
	val interface{}
	err error
}

func runThunk(ctx context.Context, thunk Thunk, out chan result, done chan struct{}) {
	var r result
	r.val, r.err = thunk(ctx)

	select {
	case out <- r:
	default:
	}
}
