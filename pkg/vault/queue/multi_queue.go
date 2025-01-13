package queue

import (
	"math"

	"github.com/rs/zerolog/log"
)

// An implementation of the QueueLayer interface utilizaing multiple channels
// to provide a greater number of synchronization threads to a using vault.
// Enqueues lead to one of the channels picking up a message and dispatching
// to the vault's Synchronized implementation. A channel is picked based on
// a hashing mechanism, each lock tag is hashed into a number between 0 and 65535,
// and then an index is derived from that hash, based on a simple calculation:
//
//	queueIndex = hash / (MAX_HASH / len(multiQueue.queues))
//
// A message is then dispatched to the picked channel by:
//
//	multiQueue.queues[queueIndex] <- &queueItem{lockTag: lockTag, callback: callback}
//
// This method ensures the same index channel always handles the same hash(es),
// and eliminates race conditions between clients of Locksmith.
type multiQueue struct {
	queues   []chan *queueItem
	hashFunc func(string) uint16
}

// Creates a QueueLayer with multiple underlying go routines for quicker
// dispatch of lock acquisitions and releases. To dispatch, each lock tag
// is hashed into a number, each queue handles a range.
func NewMultiQueue(
	concurrency int,
	capacity int,
) QueueLayer {
	ql := &multiQueue{
		queues:   make([]chan *queueItem, concurrency),
		hashFunc: fnv1aHash,
	}

	// Initialize queues, queue[0] is responsible for the range 0 -> 65535 / numQueues and so on
	for i := 0; i < concurrency; i++ {
		ql.queues[i] = make(chan *queueItem, capacity)

		go func(i int, queue chan *queueItem) {
			log.Info().Int("number", i).Msg("starting multi queue go routine")
			for {
				qi := <-queue
				qi.action(i, qi.lockTag)
			}
		}(i, ql.queues[i])
	}

	return ql
}

// Enqueue a lock tag, expect a call to the action once the queue layer has gotten
// a hold of a synchronization Go-routine specific to the resulting hash of the
// lock tag.
func (multiQueue *multiQueue) Enqueue(lockTag string, action func(int, string)) {
	log.Debug().Str("tag", lockTag).Msg("generating hash and fetching queue index")
	hash := multiQueue.hashFunc(lockTag)
	queueIndex := multiQueue.queueIndexFromHash(hash)
	log.Debug().
		Uint16("hash", hash).
		Int("queue-index", int(queueIndex)).
		Msg("enqueueing")
	multiQueue.queues[queueIndex] <- &queueItem{lockTag: lockTag, action: action}
}

// Get a queue index from an input hash to select which queue should handle an
// Enqueue(...) call.
func (multiQueue *multiQueue) queueIndexFromHash(hash uint16) uint16 {
	if hash == math.MaxUint16 {
		//nolint:gosec
		return uint16(len(multiQueue.queues)) - 1
	}

	return uint16(float32(hash) / (float32(MAX_HASH) / float32((len(multiQueue.queues)))))
}
