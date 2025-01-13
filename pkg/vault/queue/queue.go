// Package queue implements a way for the vault to ensure single-threaded handling of mutexes.
package queue

// The vault queue layer interface specifies a way for the vault to obtain
// a Go-routine for a given lock tag. Once the queue layer notifies the vault,
// the vault knows that for a given lock tag, it is safe to operate as the
// thread assigned to that lock tag is the one making the notifying call to
// the provided callback.
type QueueLayer interface {
	// Request a Go-routine for the given lock tag.
	Enqueue(lockTag string, action func(slot int, lockTag string))
}

type queueItem struct {
	lockTag string
	action  func(int, string)
}
