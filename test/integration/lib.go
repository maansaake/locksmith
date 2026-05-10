package integration

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/maansaake/locksmith/pkg/client"
)

var (
	locksmithHost = "localhost"
	locksmithPort = uint16(9000)
	metricsPort   = uint16(9464)
)

const readyTimeout = 15 * time.Second

// waitForLocksmith polls until the locksmith TCP port accepts connections or the
// timeout is reached.
func waitForLocksmith() error {
	addr := net.JoinHostPort(locksmithHost, strconv.FormatUint(uint64(locksmithPort), 10))
	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("locksmith at %s not ready after %v", addr, readyTimeout)
}

// newClient creates a connected client that is automatically closed when the
// test finishes. The returned channel receives every lock tag notification from
// the server (buffered to avoid dropping messages).
func newClient(t *testing.T) (client.Client, <-chan string) {
	t.Helper()
	acquired := make(chan string, 16)
	c := client.New(&client.Opts{
		Host: locksmithHost,
		Port: locksmithPort,
		OnAcquired: func(lockTag string) {
			acquired <- lockTag
		},
	})
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(c.Close)
	return c, acquired
}
