package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/maansaake/arbiter/pkg/module"
	"github.com/maansaake/locksmith/pkg/client"
)

const (
	defaultAcquireReleaseRate      uint = 12
	defaultDisconnectReconnectRate uint = 6
	defaultHost                         = "localhost"
	defaultPort                    uint = 9000
	defaultAcquireTimeout               = 5 * time.Second
	contentionWindow                    = 300 * time.Millisecond
	maxPort                        uint = 65535
)

type locksmithLoadModule struct {
	args module.Args
	ops  module.Ops

	host           string
	port           uint
	acquireTimeout time.Duration
	sequence       atomic.Uint64
}

func newLocksmithLoadModule() module.Module {
	m := &locksmithLoadModule{
		host:           defaultHost,
		port:           defaultPort,
		acquireTimeout: defaultAcquireTimeout,
	}

	m.args = module.Args{
		&module.Arg[string]{
			Name:  "host",
			Desc:  "Locksmith host.",
			Value: &m.host,
		},
		&module.Arg[uint]{
			Name:  "port",
			Desc:  "Locksmith port.",
			Value: &m.port,
		},
		&module.Arg[int]{
			Name: "timeout-ms",
			Desc: "Timeout for acquisition assertions in milliseconds.",
			Handler: func(v int) {
				m.acquireTimeout = time.Duration(v) * time.Millisecond
			},
			Value: intPtr(int(defaultAcquireTimeout / time.Millisecond)),
		},
	}

	m.ops = module.Ops{
		&module.Op{
			Name: "acquire-release",
			Desc: "Acquire a lock, release it, then verify another client can acquire it.",
			Rate: defaultAcquireReleaseRate,
			Do:   m.doAcquireRelease,
		},
		&module.Op{
			Name: "disconnect-reconnect",
			Desc: "Disconnect a client while it holds a lock, verify handoff, then reconnect with a new client.",
			Rate: defaultDisconnectReconnectRate,
			Do:   m.doDisconnectReconnect,
		},
	}

	return m
}

func (m *locksmithLoadModule) Name() string {
	return "locksmith-load"
}

func (m *locksmithLoadModule) Desc() string {
	return "Runs low-rate lock acquire/release and disconnect/reconnect traffic against Locksmith."
}

func (m *locksmithLoadModule) Args() module.Args {
	return m.args
}

func (m *locksmithLoadModule) Ops() module.Ops {
	return m.ops
}

func (m *locksmithLoadModule) Run() error {
	return nil
}

func (m *locksmithLoadModule) Stop() error {
	return nil
}

func (m *locksmithLoadModule) doAcquireRelease() (module.Result, error) {
	start := time.Now()
	lockTag := m.nextLockTag("acquire-release")

	holder, holderAcquired, err := m.newClient()
	if err != nil {
		return module.Result{}, err
	}
	defer holder.Close()

	if err := holder.Acquire(lockTag); err != nil {
		return module.Result{}, fmt.Errorf("holder acquire %q: %w", lockTag, err)
	}
	if err := waitForAcquire(holderAcquired, lockTag, m.acquireTimeout); err != nil {
		return module.Result{}, err
	}

	if err := holder.Release(lockTag); err != nil {
		return module.Result{}, fmt.Errorf("holder release %q: %w", lockTag, err)
	}

	follower, followerAcquired, err := m.newClient()
	if err != nil {
		return module.Result{}, err
	}
	defer follower.Close()

	if err := follower.Acquire(lockTag); err != nil {
		return module.Result{}, fmt.Errorf("follower acquire %q: %w", lockTag, err)
	}
	if err := waitForAcquire(followerAcquired, lockTag, m.acquireTimeout); err != nil {
		return module.Result{}, err
	}

	if err := follower.Release(lockTag); err != nil {
		return module.Result{}, fmt.Errorf("follower release %q: %w", lockTag, err)
	}

	return module.Result{Duration: time.Since(start)}, nil
}

func (m *locksmithLoadModule) doDisconnectReconnect() (module.Result, error) {
	start := time.Now()
	lockTag := m.nextLockTag("disconnect")
	reconnectedTag := m.nextLockTag("reconnect")

	holder, holderAcquired, err := m.newClient()
	if err != nil {
		return module.Result{}, err
	}

	waiter, waiterAcquired, err := m.newClient()
	if err != nil {
		holder.Close()
		return module.Result{}, err
	}
	defer waiter.Close()

	if err := holder.Acquire(lockTag); err != nil {
		holder.Close()
		return module.Result{}, fmt.Errorf("holder acquire %q: %w", lockTag, err)
	}
	if err := waitForAcquire(holderAcquired, lockTag, m.acquireTimeout); err != nil {
		holder.Close()
		return module.Result{}, err
	}

	if err := waiter.Acquire(lockTag); err != nil {
		holder.Close()
		return module.Result{}, fmt.Errorf("waiter acquire %q: %w", lockTag, err)
	}
	if err := ensureNotAcquired(waiterAcquired, lockTag, contentionWindow); err != nil {
		holder.Close()
		return module.Result{}, err
	}

	holder.Close()

	if err := waitForAcquire(waiterAcquired, lockTag, m.acquireTimeout); err != nil {
		return module.Result{}, err
	}
	if err := waiter.Release(lockTag); err != nil {
		return module.Result{}, fmt.Errorf("waiter release %q: %w", lockTag, err)
	}

	reconnected, reconnectedAcquired, err := m.newClient()
	if err != nil {
		return module.Result{}, err
	}
	defer reconnected.Close()

	if err := reconnected.Acquire(reconnectedTag); err != nil {
		return module.Result{}, fmt.Errorf("reconnected acquire %q: %w", reconnectedTag, err)
	}
	if err := waitForAcquire(reconnectedAcquired, reconnectedTag, m.acquireTimeout); err != nil {
		return module.Result{}, err
	}
	if err := reconnected.Release(reconnectedTag); err != nil {
		return module.Result{}, fmt.Errorf("reconnected release %q: %w", reconnectedTag, err)
	}

	return module.Result{Duration: time.Since(start)}, nil
}

func (m *locksmithLoadModule) newClient() (client.Client, <-chan string, error) {
	if m.port > maxPort {
		return nil, nil, fmt.Errorf("locksmith port %d exceeds max %d", m.port, maxPort)
	}

	acquired := make(chan string, 4)
	c := client.New(&client.Opts{
		Host: m.host,
		Port: uint16(m.port), //nolint:gosec // test input is constrained to valid port values
		OnAcquired: func(lockTag string) {
			acquired <- lockTag
		},
	})
	if err := c.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connect client: %w", err)
	}
	return c, acquired, nil
}

func (m *locksmithLoadModule) nextLockTag(prefix string) string {
	return fmt.Sprintf("load-%s-%d", prefix, m.sequence.Add(1))
}

func waitForAcquire(acquired <-chan string, lockTag string, timeout time.Duration) error {
	select {
	case got := <-acquired:
		if got != lockTag {
			return fmt.Errorf("acquired unexpected lock tag %q, want %q", got, lockTag)
		}
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timed out waiting for acquisition of %q after %s", lockTag, timeout)
	}
}

func ensureNotAcquired(acquired <-chan string, lockTag string, timeout time.Duration) error {
	select {
	case got := <-acquired:
		if got == lockTag {
			return fmt.Errorf("lock %q was acquired before the holder disconnected", lockTag)
		}
		return fmt.Errorf("acquired unexpected lock tag %q while waiting on %q", got, lockTag)
	case <-time.After(timeout):
		return nil
	}
}

func intPtr(v int) *int {
	return &v
}
