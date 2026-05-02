package queue

import "github.com/rs/zerolog/log"

type SingleQueue struct {
	queue chan *item
}

func NewSingleQueue(
	size int,
) Layer {
	q := &SingleQueue{queue: make(chan *item, size)}
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
	singleQueue.queue <- &item{lockTag: lockTag, action: action}
}
