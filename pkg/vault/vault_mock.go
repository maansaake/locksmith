package vault

import (
	"errors"
	"time"
)

type Mock struct {
	AcquireCount int
	ReleaseCount int
	CleanupCount int

	awaits       bool
	acquireAwait chan string
	releaseAwait chan string
}

var _ Vault = &Mock{}

func NewMock() *Mock {
	return &Mock{
		acquireAwait: make(chan string, 100),
	}
}

func (v *Mock) EnableAwaits() {
	v.awaits = true
	v.acquireAwait = make(chan string, 100)
	v.releaseAwait = make(chan string, 100)
}

// Acquire implements Vault.
func (v *Mock) Acquire(lockTag string, client string, callback func(error) error) {
	v.AcquireCount++
	if v.awaits {
		v.acquireAwait <- lockTag
	}
}

func (v *Mock) AwaitAcquire(lockTag string) error {
	if !v.awaits {
		return errors.New("awaits are not enabled")
	}

	select {
	case lt := <-v.acquireAwait:
		if lt == lockTag {
			return nil
		}
		return errors.New("awaited lock tag mismatch")
	case <-time.After(1 * time.Second):
		return errors.New("acquire await timed out")
	}
}

// Cleanup implements Vault.
func (v *Mock) Cleanup(locktag string, client string) {
	v.CleanupCount++
}

// Release implements Vault.
func (v *Mock) Release(lockTag string, client string, callback func(error) error) {
	v.ReleaseCount++

	if v.awaits {
		v.releaseAwait <- lockTag
	}
}

func (v *Mock) AwaitRelease(lockTag string) error {
	if !v.awaits {
		return errors.New("awaits are not enabled")
	}

	select {
	case lt := <-v.releaseAwait:
		if lt == lockTag {
			return nil
		}
		return errors.New("awaited lock tag mismatch")
	case <-time.After(1 * time.Second):
		return errors.New("release await timed out")
	}
}
