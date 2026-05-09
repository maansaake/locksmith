// Package vault solves the handling of mutexes.
package vault

import (
	"context"
	"errors"
	"fmt"

	"github.com/maansaake/locksmith/pkg/vault/queue"
	"github.com/trebent/zerologr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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
		// Used for telemetry.
		Version string

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
		queueLayer queue.Layer
		waitList   map[string][]*func(slot int, lockTag string)

		lockGauge        metric.Int64Gauge
		acquireCounter   metric.Int64Counter
		releaseCounter   metric.Int64Counter
		rejectionCounter metric.Int64Counter
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
)

const (
	Single QueueType = "single"
	Multi  QueueType = "multi"

	LOCKED   lockState = true
	UNLOCKED lockState = false

	counterReasonLabel = "reason"
)

func New(opts *Opts) (Vault, error) {
	vault := &vaultImpl{
		waitList: make(map[string][]*func(int, string)),
	}

	m := otel.GetMeterProvider().Meter(
		"github.com/maansaake/locksmith/pkg/vault", metric.WithInstrumentationVersion(opts.Version),
	)

	locksGauge, err := m.Int64Gauge(
		"locksmith.locks",
		metric.WithDescription("Number of held locks"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating locks gauge: %w", err)
	}
	vault.lockGauge = locksGauge

	acquireCounter, err := m.Int64Counter(
		"locksmith.acquires",
		metric.WithDescription("Number of processed acquires"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating acquire counter: %w", err)
	}
	vault.acquireCounter = acquireCounter

	releaseCounter, err := m.Int64Counter(
		"locksmith.releases",
		metric.WithDescription("Number of processed releases"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating release counter: %w", err)
	}
	vault.releaseCounter = releaseCounter

	rejectionCounter, err := m.Int64Counter(
		"locksmith.rejections",
		metric.WithDescription(
			"Number of rejections due to bad manners and unnecessary releases/acquires",
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating rejection counter: %w", err)
	}
	vault.rejectionCounter = rejectionCounter

	if opts.QueueType == Single {
		vault.queueLayer = queue.NewSingleQueue(opts.QueueCapacity)
		vault.slots = make([]map[string]*lock, 1)
		vault.slots[0] = map[string]*lock{}
	} else {
		vault.queueLayer = queue.NewMultiQueue(
			opts.QueueConcurrency, opts.QueueCapacity,
		)
		vault.slots = make([]map[string]*lock, opts.QueueConcurrency)
		for i := range vault.slots {
			vault.slots[i] = map[string]*lock{}
		}
	}

	return vault, nil
}

// Acquire attempts to acquire a lock. If the lock is currently busy, the
// request in put on a waiting list for the lock tag in question, leading to a
// notification once the holder has released the lock.
func (vault *vaultImpl) Acquire(
	lockTag string,
	client string,
	callback func(error) error,
) {
	zerologr.Info("Acquiring", "client", client, "tag", lockTag)
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
		zerologr.V(50).Info("Acquire", "tag", lockTag, "client", client, "slot", slot)
		lock := vault.fetch(slot, lockTag)
		// a second acquire is a protocol offense, callback with error and
		// release the lock, pop waitlisted client.
		//nolint:gocritic // why not
		if lock.isOwner(client) {
			lock.unlock()
			vault.lockGauge.Record(context.TODO(), -1)
			vault.rejectionCounter.Add(
				context.TODO(),
				1,
				metric.WithAttributeSet(attribute.NewSet(attribute.String(counterReasonLabel, "unnecessary_acquire"))),
			)

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
				vault.lockGauge.Record(context.TODO(), 1)
				vault.acquireCounter.Add(context.TODO(), 1)
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
	zerologr.Info("Releasing", "client", client, "tag", lockTag)
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
		zerologr.V(50).Info("Release", "tag", lockTag, "client", client, "slot", slot)
		currentState := vault.fetch(slot, lockTag)
		// if already unlocked, kill the client for not following the protocol
		//nolint:gocritic // why not
		if !currentState.isLocked() {
			vault.rejectionCounter.Add(
				context.TODO(),
				1,
				metric.WithAttributeSet(attribute.NewSet(attribute.String(counterReasonLabel, "unnecessary_release"))),
			)

			_ = callback(ErrUnnecessaryRelease)
			// else, the lock is in LOCKED state, so check the owner, if
			// client isn't the owner, it's misbehaving and needs to be killed
		} else if !currentState.isOwner(client) {
			vault.rejectionCounter.Add(
				context.TODO(),
				1,
				metric.WithAttributeSet(attribute.NewSet(attribute.String(counterReasonLabel, "bad_manners"))),
			)

			_ = callback(ErrBadManners)
			// else, client is the owner of the lock, release it and call
			// callback
		} else {
			currentState.unlock()
			vault.lockGauge.Record(context.TODO(), -1)
			vault.releaseCounter.Add(context.TODO(), 1)

			_ = callback(nil) // We don't care about release errors

			vault.popWaitlist(slot, lockTag)
		}
	}
}

// Cleans up a locktag associated with a given client.
func (vault *vaultImpl) Cleanup(lockTag, client string) {
	zerologr.Info("Cleaning up", "client", client, "tag", lockTag)
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
		zerologr.V(50).Info("Cleanup", "tag", lockTag, "client", client, "slot", slot)
		if currentState := vault.fetch(slot, lockTag); currentState.isOwner(client) {
			currentState.unlock()
			vault.lockGauge.Record(context.TODO(), -1)
			vault.releaseCounter.Add(context.TODO(), 1)

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
	zerologr.V(50).Info("Waitlisting", "tag", lockTag)
	_, ok := vault.waitList[lockTag]
	if !ok {
		vault.waitList[lockTag] = []*func(int, string){&callback}
	} else {
		vault.waitList[lockTag] = append(vault.waitList[lockTag], &callback)
	}
	zerologr.V(50).Info("", "tag", lockTag, "waitlisted", len(vault.waitList[lockTag]))
}

// IMPORTANT: only call from synchronized Go-routines.
// Pop from the waitlist belonging to the input lock tag, results in a waitlisted
// action being called directly.
func (vault *vaultImpl) popWaitlist(slot int, lockTag string) {
	zerologr.V(50).Info("Popping from waitlist", "tag", lockTag)
	if wl, ok := vault.waitList[lockTag]; ok && len(wl) > 0 {
		first := wl[0]

		if len(wl) == 1 {
			delete(vault.waitList, lockTag)
		} else {
			vault.waitList[lockTag] = wl[1:]
		}
		zerologr.V(50).Info("", "tag", lockTag, "waitlisted", len(wl)-1)

		f := *first
		f(slot, lockTag)
	} else {
		zerologr.V(50).Info("No waitlisted clients found")
	}
}

func newlock() *lock {
	return &lock{owner: "", state: UNLOCKED}
}

// implies lock is in LOCKED state.
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
