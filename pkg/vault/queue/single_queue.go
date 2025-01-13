package queue

import "github.com/rs/zerolog/log"

type SingleQueue struct {
	queue chan *queueItem
}

func NewSingleQueue(
	size int,
) QueueLayer {
	q := &SingleQueue{queue: make(chan *queueItem, size)}
	go func() {
		log.Info().Msg("started single queue")
		for {
			qi := <-q.queue
			qi.action(0, qi.lockTag)
		}
	}()
	return q
}

func (singleQueue *SingleQueue) Enqueue(lockTag string, action func(int, string)) {
	singleQueue.queue <- &queueItem{lockTag: lockTag, action: action}
}
