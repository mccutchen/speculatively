/*
Package speculatively provides a simple mechanism to speculatively execute a
task in parallel only after some initial timeout has elapsed.

This was inspired by the "Defeat your 99th percentile with speculative task"
blog post[1], which describes it nicely:

> The inspiration came from BigData world. In Spark when task execution runs
> suspiciously long the application master starts the same task speculatively
> on a different executor but it lets the long running tasks to continue. The
> solution looked elegant:
>
>  * Service response time limit is 50ms.
>
>  * If the first attempt doesn’t finish within 25ms start a new one, but
>    keep the first thread running.
>
>  * Wait for either thread to finish and take result from the first one
>    ready.

The speculative tasks implemented here are similar to "hedged requests" as
described in "The Tail at Scale"[2] and implemented in the Query example
function in "Go Concurrency Patterns: Timing out, moving on"[3], but they a)
have no knowledge of different replicas and b) wait for a caller-controlled
timeout before launching additional tasks.

[1]: https://bjankie1.github.io/blog/
[2]: http://www-inst.eecs.berkeley.edu/~cs252/sp17/papers/TheTailAtScale.pdf
[3]: https://blog.golang.org/go-concurrency-patterns-timing-out-and
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
