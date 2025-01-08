package queuetest

import (
	"sync"
	"testing"
	"time"

	"github.com/maansaake/locksmith/pkg/vault/queue"
)

// This test is meant to run manually to compare queue implementations.
// TODO: replace with client E2E test.
func Test_MultiQueueTimeTaken(t *testing.T) {
	t.Skip()
	start := time.Now()
	mq := queue.NewMultiQueue(10, 100)
	numEnqueues := 10000
	wg := sync.WaitGroup{}
	wg.Add(numEnqueues)

	t.Log("Starting to Enqueue", numEnqueues, "items at", time.Now())
	for i := 0; i < numEnqueues; i++ {
		mq.Enqueue(randSeq(50), func(lockTag string) {
			time.Sleep(1 * time.Millisecond)
			wg.Done()
		})
	}
	t.Log("Enqueueing done at", time.Now())

	wg.Wait()

	t.Log("Wait done at", time.Now())

	t.Log("Took", time.Since(start))
}

func Test_Multi_Enqueue(t *testing.T) {
	expectedCallCount := 100
	q := queue.NewMultiQueue(10, 300)
	wg := sync.WaitGroup{}
	wg.Add(expectedCallCount)

	for i := 0; i < expectedCallCount; i++ {
		q.Enqueue(randSeq(50), func(lockTag string) {
			wg.Done()
		})
	}

	wg.Wait()
}
