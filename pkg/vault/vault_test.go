package vault

import (
	"errors"
	"sync"
	"testing"
)

type tql struct{}

func (t *tql) Enqueue(locktag string, action func(string)) {
	action(locktag)
}

func Test_Acquire(t *testing.T) {
	vault := New(&Opts{})
	v := vault.(*vaultImpl)
	v.queueLayer = &tql{}
	wg := sync.WaitGroup{}
	wg.Add(1)

	called := false
	v.Acquire("lt", "client", func(err error) error {
		t.Log("Acquire callback called!")
		called = true
		wg.Done()
		return nil
	})
	wg.Wait()

	if !called {
		t.Error("Acquire callback wasn't called")
	}

	if li := v.fetch("lt"); !(li.isOwner("client") && li.isLocked()) {
		t.Error("Expected client to be the owner and the lock to be locked")
	}
}

func Test_Release(t *testing.T) {
	v := &vaultImpl{
		state:      make(map[string]*lock),
		queueLayer: &tql{},
	}
	wg := sync.WaitGroup{}
	wg.Add(2)

	v.Acquire("lt", "client", func(err error) error {
		t.Log("Acquire callback called!")
		wg.Done()
		return nil
	})

	called := false
	v.Release("lt", "client", func(err error) error {
		t.Log("Release callback called!")
		called = true
		wg.Done()
		return nil
	})

	if !called {
		t.Error("Release callback wasn't called")
	}

	if li := v.fetch("lt"); li.isLocked() {
		t.Error("Expected the lock to not be.... well locked")
	}
}

func Test_Waitlist(t *testing.T) {
	v := &vaultImpl{
		state:      make(map[string]*lock),
		waitList:   make(map[string][]*func(string)),
		queueLayer: &tql{},
	}

	order := make([]string, 0, 3)

	wg := sync.WaitGroup{}
	wg.Add(3)
	v.Acquire("lt", "client1", func(err error) error {
		t.Log("Acquire client1 callback called!")
		wg.Done()
		order = append(order, "client1")

		return nil
	})
	v.Acquire("lt", "client2", func(err error) error {
		t.Log("Acquire client2 callback called!")
		wg.Done()
		order = append(order, "client2")

		return nil
	})
	v.Release("lt", "client1", func(err error) error {
		t.Log("Release client1 callback called!")
		wg.Done()
		order = append(order, "client1")

		return nil
	})
	wg.Wait()

	t.Log(order)

	// Check order of operations...
	if order[0] != "client1" {
		t.Error("First operation was not client1")
	}
	if order[1] != "client1" {
		t.Error("Second operation was not client1")
	}
	if order[2] != "client2" {
		t.Error("Third operation was not client2")
	}
}

func Test_ReleaseBadManners(t *testing.T) {
	v := &vaultImpl{
		state:      make(map[string]*lock),
		queueLayer: &tql{},
	}
	wg := sync.WaitGroup{}
	wg.Add(2)

	v.Acquire("lt", "client1", func(err error) error {
		t.Log("Acquire client1 callback called!")
		wg.Done()
		return nil
	})
	v.Release("lt", "client2", func(err error) error {
		t.Log("Release client2 callback called with error:", err)
		if !errors.Is(err, ErrBadManners) {
			t.Error("Expected BadMannersError")
		}
		wg.Done()
		return nil
	})

	wg.Wait()
}

func Test_UnecessaryRelease(t *testing.T) {
	v := &vaultImpl{
		state:      make(map[string]*lock),
		queueLayer: &tql{},
	}
	wg := sync.WaitGroup{}
	wg.Add(1)

	v.Release("lt", "client", func(err error) error {
		t.Log("Release client callback called with error:", err)
		if !errors.Is(err, ErrUnnecessaryRelease) {
			t.Error("Expected UnecessaryReleaseError")
		}
		wg.Done()

		return nil
	})

	wg.Wait()
}

func Test_UnecessaryAcquire(t *testing.T) {
	v := &vaultImpl{
		state:      make(map[string]*lock),
		queueLayer: &tql{},
	}
	wg := sync.WaitGroup{}
	wg.Add(2)

	v.Acquire("lt", "client", func(err error) error {
		t.Log("Acquire client callback called with error:", err)
		wg.Done()
		return nil
	})
	v.Acquire("lt", "client", func(err error) error {
		t.Log("Acquire client callback called with error:", err)
		if !errors.Is(err, ErrUnnecessaryAcquire) {
			t.Error("Expected UnecesasryAcquireError")
		}
		wg.Done()

		return nil
	})

	wg.Wait()
}

func Test_CallbackError(t *testing.T) {
	v := &vaultImpl{
		state:      make(map[string]*lock),
		queueLayer: &tql{},
	}
	wg := sync.WaitGroup{}
	wg.Add(1)

	v.Acquire("lt", "client", func(err error) error {
		t.Log("Acquire client callback called with error:", err)
		wg.Done()
		// Because of the returned error, another client is able to acquire the lock
		return errors.New("some kind of error")
	})
	wg.Wait()

	if l, ok := v.state["lt"]; ok {
		if l.owner != "" || l.state != UNLOCKED {
			t.Error("Unexpected lock state")
		}
	}

	wg.Add(1)

	v.Acquire("lt", "client2", func(err error) error {
		t.Log("Acquire client2 callback called with error:", err)
		wg.Done()
		return nil
	})

	wg.Wait()

	if l, ok := v.state["lt"]; ok {
		if l.owner != "client2" || l.state != LOCKED {
			t.Error("Expected client2 to have acquired the lock")
		}
	}
}

func TestVault_Cleanup(t *testing.T) {
	v := &vaultImpl{queueLayer: &tql{}, state: make(map[string]*lock)}

	l := v.fetch("test")
	l.lock("client")

	v.Cleanup("test", "client")

	if l.isLocked() {
		t.Fatal("lock was still locked")
	}

	if l.isOwner("client") {
		t.Fatal("owner was still 'client'")
	}
}

func TestVault_CleanupWaitlist(t *testing.T) {
	vault := New(&Opts{QueueType: Single})
	v := vault.(*vaultImpl)
	v.queueLayer = &tql{}
	v.state = make(map[string]*lock)

	v.Acquire("test", "client", func(error) error { return nil })
	v.Acquire("test", "client2", func(error) error { return nil })

	v.Cleanup("test", "client")

	l := v.fetch("test")

	if !l.isLocked() {
		t.Fatal("lock was not locked")
	}

	if !l.isOwner("client2") {
		t.Fatal("owner was not 'client2'")
	}
}
