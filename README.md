# speculatively

Package `speculatively` provides a simple mechanism to speculatively execute a
task in parallel only after some initial timeout has elapsed:

```go
// An example task that will wait for a random amount of time before returning
task := func(ctx context.Context) (interface{}, error) {
    delay := time.Duration(float64(250*time.Millisecond) * rand.Float64())
    select {
    case <-time.After(delay):
        return "success", nil
    case <-ctx.Done():
        return "timeout", ctx.Err()
    }
}

ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
defer cancel()

// If task doesn't return within 20ms, it will be executed again in parallel
result, err := speculatively.Do(ctx, 20*time.Millisecond, task)
```

This was inspired by the ["Defeat your 99th percentile with speculative task"
blog post][1], which describes it nicely:

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
described in [The Tail at Scale][2] and implemented in the Query example
function in [Go Concurrency Patterns: Timing out, moving on][3], but they a)
have no knowledge of different replicas and b) wait for a caller-controlled
timeout before launching additional tasks.

[1]: https://bjankie1.github.io/blog/
[2]: http://www-inst.eecs.berkeley.edu/~cs252/sp17/papers/TheTailAtScale.pdf
[3]: https://blog.golang.org/go-concurrency-patterns-timing-out-and
