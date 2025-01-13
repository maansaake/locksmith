// Package vault solves the handling of mutexes.
package vault

import (
	"errors"
	"fmt"

	"github.com/maansaake/locksmith/pkg/vault/queue"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

type (
	// The Vault interface specifies high level functions to implement in order to
	// handle the acquisition and release of mutexes.
	Vault interface {
		// Lock tag is a string identifying the lock to acquire, client the requesting party,
		// and the callback a function which will be called to either confirm acquisition or
		// including an error in case the client is misbehaving. The callback may return an
		// error in case feedback handling encounters an error.
		Acquire(lockTag, client string, callback func(error) error)
		Release(lockTag, client string, callback func(error) error)
		Cleanup(locktag, client string)
	}
	QueueType string
	Opts      struct {
		// Single queue mode should only be used for testing.
		QueueType

		// Only for multi-mode queues, determines the number of
		// supporting Go-routines able to handle work given to the
		// queueing layer.
		QueueConcurrency int

		// Sets the capacity of the underlying queue(s), the max amount
		// of buffered work for a queue. In a multi queue setting, the
		// capacity indicates the buffer size per queue.
		QueueCapacity int
	}
	lockState bool
	lock      struct {
		owner string
		state lockState
	}
	// Implementation of the Vault interface. By use of a queue layer, the vault ensures
	// lock states are only manipulated from one Go-routine at a time. Read more in the
	// QueueLayer interface description.
	vaultImpl struct {
		slots      []map[string]*lock
		queueLayer queue.QueueLayer
		waitList   map[string][]*func(slot int, lockTag string)
	}
)

var (
	ErrUnnecessaryAcquire = errors.New(
		"client tried to acquire a lock that it already had acquired",
	)
	ErrUnnecessaryRelease = errors.New(
		"client tried to release a lock that had not been acquired",
	)
	ErrBadManners = errors.New(
		"client tried to release lock that it did not own",
	)

	locksGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "locksmith_total_locked_locks",
		Help: "The total number of locked locks",
	})
	acquireCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "locksmith_acquires",
		Help: "The number of processed acquires",
	})
	releaseCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "locksmith_releases",
		Help: "The number of processed releases",
	})
	rejectionCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "locksmith_rejections",
		Help: "The number of rejections due to bad manners and unnecessary releases/acquires",
	}, []string{"reason"})
)

const (
	Single QueueType = "single"
	Multi  QueueType = "multi"

	LOCKED   lockState = true
	UNLOCKED lockState = false
)

func New(options *Opts) Vault {
	vault := &vaultImpl{
		waitList: make(map[string][]*func(int, string)),
	}
	if options.QueueType == Single {
		vault.queueLayer = queue.NewSingleQueue(options.QueueCapacity)
		vault.slots = make([]map[string]*lock, 1)
		vault.slots[0] = map[string]*lock{}
	} else {
		vault.queueLayer = queue.NewMultiQueue(
			options.QueueConcurrency, options.QueueCapacity,
		)
		vault.slots = make([]map[string]*lock, options.QueueConcurrency)
		for i := range vault.slots {
			vault.slots[i] = map[string]*lock{}
		}
	}

	return vault
}

// Acquire attempts to acquire a lock. If the lock is currently busy, the
// request in put on a waiting list for the lock tag in question, leading to a
// notification once the holder has released the lock.
func (vault *vaultImpl) Acquire(
	lockTag string,
	client string,
	callback func(error) error,
) {
	log.Info().
		Str("client", client).
		Str("tag", lockTag).
		Msg("acquiring")
	vault.queueLayer.Enqueue(
		lockTag, vault.acquireAction(client, callback),
	)
}

// Returns a callback to call once the vault has gotten hold of a
// synchronization Go-routine. The returned action callback contains
// handling for what should happen when a client requests to acquire
// a lock. The returned callback is the only piece of code allowed to
// handle acquiring locks.
func (vault *vaultImpl) acquireAction(
	client string,
	callback func(error) error,
) func(int, string) {
	return func(slot int, lockTag string) {
		log.Debug().Str("tag", lockTag).Str("client", client).Int("slot", slot).Msg("acquire")
		lock := vault.fetch(slot, lockTag)
		// a second acquire is a protocol offense, callback with error and
		// release the lock, pop waitlisted client.
		//nolint:gocritic
		if lock.isOwner(client) {
			lock.unlock()
			locksGauge.Dec()
			rejectionCounter.With(prometheus.Labels{"reason": "unnecessary_acquire"}).Inc()

			_ = callback(ErrUnnecessaryAcquire)

			vault.popWaitlist(slot, lockTag)
			// client didn't match, and the lock state is LOCKED, waitlist the
			// client
		} else if lock.isLocked() {
			vault.waitlist(
				lockTag, vault.acquireAction(client, callback),
			)
		} else {
			// This means a write failure occurred and the client that was
			// acquiring the lock has NW issues or something.
			if err := callback(nil); err != nil {
				// don't touch the lock state, pop from waitlist
				vault.popWaitlist(slot, lockTag)
			} else {
				lock.lock(client)
				locksGauge.Inc()
				acquireCounter.Inc()
			}
		}
	}
}

// Release releases a lock, leading to a queued acquire calling the vault
// callback.
func (vault *vaultImpl) Release(
	lockTag string,
	client string,
	callback func(error) error,
) {
	log.Info().
		Str("client", client).
		Str("tag", lockTag).
		Msg("releasing")
	vault.queueLayer.Enqueue(lockTag, vault.releaseAction(client, callback))
}

// Returns a callback that handles the release of locks. This is the only piece
// of code allowed to touch release-handling, similarly to the acquireAction
// function. The returned function must only be called from the scope of a
// synchronization Go-routine.
func (vault *vaultImpl) releaseAction(
	client string,
	callback func(error) error,
) func(int, string) {
	return func(slot int, lockTag string) {
		log.Debug().Str("tag", lockTag).Str("client", client).Int("slot", slot).Msg("release")
		currentState := vault.fetch(slot, lockTag)
		// if already unlocked, kill the client for not following the protocol
		//nolint:gocritic
		if !currentState.isLocked() {
			rejectionCounter.With(prometheus.Labels{"reason": "unnecessary_release"}).Inc()

			_ = callback(ErrUnnecessaryRelease)
			// else, the lock is in LOCKED state, so check the owner, if
			// client isn't the owner, it's misbehaving and needs to be killed
		} else if !currentState.isOwner(client) {
			rejectionCounter.With(prometheus.Labels{"reason": "bad_manners"}).Inc()

			_ = callback(ErrBadManners)
			// else, client is the owner of the lock, release it and call
			// callback
		} else {
			currentState.unlock()
			locksGauge.Dec()
			releaseCounter.Inc()

			_ = callback(nil) // We don't care about release errors

			vault.popWaitlist(slot, lockTag)
		}
	}
}

// Cleans up a locktag associated with a given client.
func (vault *vaultImpl) Cleanup(lockTag, client string) {
	log.Info().Str("client", client).Str("tag", lockTag).Msg("cleaning up")
	vault.queueLayer.Enqueue(
		lockTag, vault.cleanupAction(client),
	)
}

// Returns a callback that handles the cleanup of a client for a given lock tag.
// This function must only be called from the scope of a synchronization
// Go-routine, because just like the acquire- and releaseAction functions, it
// handles the vault's lock states.
func (vault *vaultImpl) cleanupAction(client string) func(int, string) {
	return func(slot int, lockTag string) {
		log.Debug().Str("tag", lockTag).Str("client", client).Int("slot", slot).Msg("cleanup")
		if currentState := vault.fetch(slot, lockTag); currentState.isOwner(client) {
			currentState.unlock()
			locksGauge.Dec()
			releaseCounter.Inc()

			vault.popWaitlist(slot, lockTag)
		}
	}
}

func (vault *vaultImpl) fetch(slot int, lockTag string) *lock {
	lock, ok := vault.slots[slot][lockTag]
	if !ok {
		lock = newlock()
		vault.slots[slot][lockTag] = lock
	}

	return lock
}

// IMPORTANT: only call from synchronized Go-routines.
// Waitlist the input action, related to the given lock tag. Appends the action
// to the back of the waitlist of the lock tag.
func (vault *vaultImpl) waitlist(lockTag string, callback func(int, string)) {
	log.Debug().Str("tag", lockTag).Msg("waitlisting")
	_, ok := vault.waitList[lockTag]
	if !ok {
		vault.waitList[lockTag] = []*func(int, string){&callback}
	} else {
		vault.waitList[lockTag] = append(vault.waitList[lockTag], &callback)
	}
	log.Debug().Str("tag", lockTag).Int("waitlisted", len(vault.waitList[lockTag])).Send()
}

// IMPORTANT: only call from synchronized Go-routines.
// Pop from the waitlist belonging to the input lock tag, results in a waitlisted
// action being called directly.
func (vault *vaultImpl) popWaitlist(slot int, lockTag string) {
	log.Debug().Str("tag", lockTag).Msg("popping from waitlist")
	if wl, ok := vault.waitList[lockTag]; ok && len(wl) > 0 {
		first := wl[0]

		if len(wl) == 1 {
			delete(vault.waitList, lockTag)
		} else {
			vault.waitList[lockTag] = wl[1:]
		}
		log.Debug().Str("tag", lockTag).Interface("waitlisted", len(wl)-1).Send()

		f := *first
		f(slot, lockTag)
	} else {
		log.Debug().Msg("no waitlisted clients found")
	}
}

func newlock() *lock {
	return &lock{owner: "", state: UNLOCKED}
}

// implies lock is in LOCKED state
func (l *lock) isOwner(client string) bool {
	return l.owner == client
}

func (l *lock) isLocked() bool {
	return l.state == LOCKED
}

func (l *lock) unlock() {
	l.state = UNLOCKED
	l.owner = ""
}

func (l *lock) lock(client string) {
	l.state = LOCKED
	l.owner = client
}

func (l *lock) String() string {
	return fmt.Sprintf("&lock{c: %s, s: %v}", l.owner, l.state)
}
