package locksmith

import (
	"bytes"
	"context"
	"net"
	"slices"
	"testing"

	"github.com/maansaake/locksmith/pkg/client"
	"github.com/maansaake/locksmith/pkg/vault"
)

func TestClientContext_addRemove(t *testing.T) {
	clientCtx := &clientContext{lockTags: []string{"test1", "test2"}}
	clientCtx.add("test3")

	if len(clientCtx.lockTags) != 3 {
		t.Fatal("expected 3 lock tags, got ", len(clientCtx.lockTags))
	}

	clientCtx.remove("test2")
	if len(clientCtx.lockTags) != 2 {
		t.Fatal("expected 2 lock tags, got ", len(clientCtx.lockTags))
	}

	if -1 != slices.IndexFunc(clientCtx.lockTags, func(item string) bool { return item == "test2" }) {
		t.Fatal("expected 'test2' to be removed")
	}
}

func TestServer_Stop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	locksmith := New(&Opts{Port: 30001})

	go func() {
		cancel()
	}()

	err := locksmith.Start(ctx)
	if err != nil {
		t.Error("Error from Locksmith.Start: ", err)
	}

	t.Log("Locksmith stopped")
}

func TestServer_handleClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	locksmith := New(&Opts{
		Port:          30001,
		QueueType:     vault.Single,
		QueueCapacity: 1,
	})
	done := make(chan bool)

	go func() {
		err := locksmith.Start(ctx)
		if err != nil {
			t.Error("Error from Locksmith.Start: ", err)
		}

		done <- true
	}()

	acquired := make(chan bool)
	onAcquired := func(lockTag string) {
		if lockTag != "test" {
			t.Fatal("expected locktag to be 'test', got ", lockTag)
		}

		acquired <- true
	}

	client := client.New(&client.Opts{
		Host:       "localhost",
		Port:       30001,
		OnAcquired: onAcquired,
	})
	err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	err = client.Acquire("test")
	if err != nil {
		t.Fatal(err)
	}
	<-acquired

	err = client.Release("test")
	if err != nil {
		t.Fatal(err)
	}

	// clean up
	cancel()
	<-done
}

func TestServer_handleBuf(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.Write([]byte{0, 3, 3, 3, 3})

	vaultMock := &vault.Mock{}
	locksmith := &Locksmith{
		vault: vaultMock,
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	err := locksmith.handleBuf(buf, &clientContext{conn: client})
	if err != nil {
		t.Fatal(err)
	}

	if vaultMock.AcquireCount != 1 {
		t.Fatal("should be 1 acquire")
	}

	err = locksmith.handleBuf(buf, &clientContext{conn: client})
	if err != nil {
		t.Fatal(err)
	}

	if vaultMock.AcquireCount != 1 {
		t.Fatal("should be 1 acquire")
	}
}

func TestServer_handleBufMultipleMessages(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.Write([]byte{0, 3, 3, 3, 3})
	buf.Write([]byte{0, 6, 3, 3, 3, 4, 5, 6})
	buf.Write([]byte{1, 4, 3, 4, 5, 6})
	buf.Write([]byte{1, 10, 49, 49, 49, 49, 49, 49, 3, 4, 5, 6})

	vaultMock := vault.NewMock()
	locksmith := &Locksmith{
		vault: vaultMock,
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	t.Log("handling and clearing buffer")
	err := locksmith.handleBuf(buf, &clientContext{conn: client})
	if err != nil {
		t.Fatal(err)
	}

	if vaultMock.AcquireCount != 2 {
		t.Fatal("should be 2 acquires:", vaultMock.AcquireCount)
	}
	if vaultMock.ReleaseCount != 2 {
		t.Fatal("should be 2 releases:", vaultMock.ReleaseCount)
	}

	t.Log("second buffer clear with no new messages, nothing happens")
	err = locksmith.handleBuf(buf, &clientContext{conn: client})
	if err != nil {
		t.Fatal(err)
	}

	if vaultMock.AcquireCount != 2 {
		t.Fatal("should be 2 acquires:", vaultMock.AcquireCount)
	}

	t.Log("third buffer clear with one new message")
	buf.Write([]byte{0, 5, 70, 70, 70, 70, 70})
	err = locksmith.handleBuf(buf, &clientContext{conn: client})
	if err != nil {
		t.Fatal(err)
	}

	if vaultMock.AcquireCount != 3 {
		t.Fatal("should be 3 acquires:", vaultMock.AcquireCount)
	}
}

func TestServer_handleBufPartialMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.Write([]byte{0, 3, 3, 3})

	vaultMock := vault.NewMock()
	locksmith := &Locksmith{
		vault: vaultMock,
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	err := locksmith.handleBuf(buf, &clientContext{conn: client})
	if err != nil {
		t.Fatal(err)
	}

	if vaultMock.AcquireCount != 0 {
		t.Fatal("should be 0 acquires:", vaultMock.AcquireCount)
	}

	buf.Write([]byte{3})

	err = locksmith.handleBuf(buf, &clientContext{conn: client})
	if err != nil {
		t.Fatal(err)
	}

	if vaultMock.AcquireCount != 1 {
		t.Fatal("should be 1 acquires:", vaultMock.AcquireCount)
	}
}

func TestServer_handleBufInvalidDecoding(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.Write([]byte{0, 3, 0x80, 0xBF, 0})

	vaultMock := vault.NewMock()
	locksmith := &Locksmith{
		vault: vaultMock,
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	err := locksmith.handleBuf(buf, &clientContext{conn: client})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestServer_handleConnection(t *testing.T) {
	vaultMock := vault.NewMock()
	vaultMock.EnableAwaits()
	locksmith := &Locksmith{
		vault: vaultMock,
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go locksmith.handleConnection(client)

	server.Write([]byte{0, 3, 70, 70, 70})
	err := vaultMock.AwaitAcquire("FFF")
	if err != nil {
		t.Fatal(err)
	}

	server.Write([]byte{1, 3, 70, 70, 70})
	err = vaultMock.AwaitRelease("FFF")
	if err != nil {
		t.Fatal(err)
	}
}
