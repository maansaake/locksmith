package integration

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/trebent/zerologr"
)

func TestMain(m *testing.M) {
	if h := os.Getenv("LOCKSMITH_HOST"); h != "" {
		locksmithHost = h
	}
	if p := os.Getenv("LOCKSMITH_PORT"); p != "" {
		if port, err := strconv.ParseUint(p, 10, 16); err == nil {
			locksmithPort = uint16(port) //nolint:gosec // validated range
		}
	}
	if mp := os.Getenv("LOCKSMITH_METRICS_PORT"); mp != "" {
		if port, err := strconv.ParseUint(mp, 10, 16); err == nil {
			metricsPort = uint16(port) //nolint:gosec // validated range
		}
	}

	zerologr.Set(zerologr.New(&zerologr.Opts{Caller: true, Console: true, V: 0}))

	if err := waitForLocksmith(); err != nil {
		fmt.Fprintf(os.Stderr, "locksmith not ready: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// TestSingleClientAcquireRelease verifies that a single client can acquire and
// release a lock.
func TestSingleClientAcquireRelease(t *testing.T) {
	t.Parallel()
	const lockTag = "single-acquire-release"

	c, acquired := newClient(t)

	if err := c.Acquire(lockTag); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	select {
	case tag := <-acquired:
		if tag != lockTag {
			t.Errorf("got lock tag %q, want %q", tag, lockTag)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for lock acquisition")
	}

	if err := c.Release(lockTag); err != nil {
		t.Fatalf("release: %v", err)
	}
}

// TestTwoClientsContendSameLock verifies that a second client is blocked from
// acquiring a lock held by a first client, and unblocked once the first client
// releases it.
func TestTwoClientsContendSameLock(t *testing.T) {
	t.Parallel()
	const lockTag = "contended-lock"

	c1, acquired1 := newClient(t)
	c2, acquired2 := newClient(t)

	// c1 acquires the lock.
	if err := c1.Acquire(lockTag); err != nil {
		t.Fatalf("c1 acquire: %v", err)
	}
	select {
	case <-acquired1:
	case <-time.After(5 * time.Second):
		t.Fatal("c1 timed out waiting for lock acquisition")
	}

	// c2 requests the same lock while c1 holds it.
	if err := c2.Acquire(lockTag); err != nil {
		t.Fatalf("c2 acquire: %v", err)
	}
	select {
	case <-acquired2:
		t.Fatal("c2 acquired the lock while c1 still holds it")
	case <-time.After(300 * time.Millisecond):
		// Expected: c2 is blocked.
	}

	// c1 releases; c2 should now be granted the lock.
	if err := c1.Release(lockTag); err != nil {
		t.Fatalf("c1 release: %v", err)
	}
	select {
	case tag := <-acquired2:
		if tag != lockTag {
			t.Errorf("c2 got lock tag %q, want %q", tag, lockTag)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("c2 timed out waiting for lock acquisition after c1 released")
	}
}

// TestMultipleClientsIndependentLocks verifies that multiple clients can
// concurrently acquire distinct lock tags without blocking each other.
func TestMultipleClientsIndependentLocks(t *testing.T) {
	t.Parallel()
	const numClients = 5

	var wg sync.WaitGroup
	for i := range numClients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			lockTag := fmt.Sprintf("independent-lock-%d", i)

			c, acquired := newClient(t)

			if err := c.Acquire(lockTag); err != nil {
				t.Errorf("client %d acquire: %v", i, err)
				return
			}
			select {
			case tag := <-acquired:
				if tag != lockTag {
					t.Errorf("client %d: got lock tag %q, want %q", i, tag, lockTag)
				}
			case <-time.After(5 * time.Second):
				t.Errorf("client %d timed out waiting for acquisition", i)
				return
			}

			if err := c.Release(lockTag); err != nil {
				t.Errorf("client %d release: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
}

// TestManyClientsQueueForOneLock verifies that multiple clients contending the
// same lock are served exclusively: at most one client holds the lock at any
// given time.
func TestManyClientsQueueForOneLock(t *testing.T) {
	t.Parallel()
	const lockTag = "queued-lock"
	const numClients = 5

	var (
		concurrent int
		violations int
	)

	var wg sync.WaitGroup
	for range numClients {
		wg.Add(1)
		c, acquired := newClient(t)
		go func() {
			defer wg.Done()

			if err := c.Acquire(lockTag); err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			select {
			case tag := <-acquired:
				if tag != lockTag {
					t.Errorf("got lock tag %q, want %q", tag, lockTag)
				}
			case <-time.After(5 * time.Second):
				t.Errorf("timed out waiting for lock acquisition")
				return
			}

			concurrent++
			if concurrent > 1 {
				violations++
			}

			// Hold the lock briefly to expose any concurrency violations.
			time.Sleep(20 * time.Millisecond)

			concurrent--

			if err := c.Release(lockTag); err != nil {
				t.Errorf("release: %v", err)
			}
		}()
	}
	wg.Wait()

	if violations > 0 {
		t.Errorf("lock was held by more than one client simultaneously (%d violation(s))", violations)
	}
}

// TestClientDisconnectReleasesLock verifies that an abrupt client disconnect
// causes the server to release that client's held locks, unblocking any waiting
// clients.
func TestClientDisconnectReleasesLock(t *testing.T) {
	t.Parallel()
	const lockTag = "disconnect-lock"

	c1, acquired1 := newClient(t)

	if err := c1.Acquire(lockTag); err != nil {
		t.Fatalf("c1 acquire: %v", err)
	}
	select {
	case <-acquired1:
	case <-time.After(5 * time.Second):
		t.Fatal("c1 timed out waiting for acquisition")
	}

	c2, acquired2 := newClient(t)

	if err := c2.Acquire(lockTag); err != nil {
		t.Fatalf("c2 acquire: %v", err)
	}

	// c1 disconnects without releasing explicitly.
	c1.Close()

	select {
	case tag := <-acquired2:
		if tag != lockTag {
			t.Errorf("c2 got lock tag %q, want %q", tag, lockTag)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("c2 timed out waiting for lock after c1 disconnected")
	}
}
