package speculatively_test

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mccutchen/speculatively"
)

// ExpensiveTask is an example of a task that takes some time to execute
type ExpensiveTask struct {
	initialLatency time.Duration
	patience       time.Duration
	callCount      int
	mu             sync.Mutex
}

// Execute will wait for some time before returning its own call count.  Every
// call will decrease the amount of time it waits by half.
func (t *ExpensiveTask) Execute(ctx context.Context) (int, error) {
	t.mu.Lock()
	initialLatency := t.initialLatency
	patience := t.patience
	call := t.callCount
	t.callCount++
	t.mu.Unlock()

	latency := initialLatency / time.Duration(int64(math.Exp2(float64(call))))
	wait := patience * time.Duration(call)

	fmt.Printf("call %d: %5s wait + %5s latency = %5s overall\n", call, wait, latency, wait+latency)

	select {
	case <-time.After(latency):
		return call, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Example demonstrates use of speculatively.Do to execute an expensive task
// one or more times in parallel.
//
// Every time the task is executed, it will wait half as long before returning.
//
// Given an operation timeout of 100ms and an initial task latency of 256ms,
// the first execution of our expensive task will always time out.  Every 20ms,
// the task will be executed again, until it returns successfully or the
// timeout expires.
//
// The overall timings of each execution look like this:
//
//	call 0:    0s wait + 256ms latency = 256ms overall
//	call 1:  20ms wait + 128ms latency = 148ms overall
//	call 2:  40ms wait +  64ms latency = 104ms overall
//	call 3:  60ms wait +  32ms latency =  92ms overall <-- winner
//	call 4:  80ms wait +  16ms latency =  96ms overall
func Example() {
	var (
		timeout        = 100 * time.Millisecond
		initialLatency = 256 * time.Millisecond
		patience       = 20 * time.Millisecond
	)

	expensiveTask := &ExpensiveTask{
		initialLatency: initialLatency,
		patience:       patience,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := speculatively.Do(ctx, patience, func(ctx context.Context) (interface{}, error) {
		return expensiveTask.Execute(ctx)
	})
	if err != nil {
		fmt.Printf("unexpected error: %s\n", err)
		return
	}
	successfulCall := result.(int)
	fmt.Printf("succeeded on call number %d\n", successfulCall)

	// Output:
	// call 0:    0s wait + 256ms latency = 256ms overall
	// call 1:  20ms wait + 128ms latency = 148ms overall
	// call 2:  40ms wait +  64ms latency = 104ms overall
	// call 3:  60ms wait +  32ms latency =  92ms overall
	// call 4:  80ms wait +  16ms latency =  96ms overall
	// succeeded on call number 3
}
