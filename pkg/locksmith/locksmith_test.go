package locksmith

import (
	"context"
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
	locksmith := New(&LocksmithOptions{Port: 30001})

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
	locksmith := New(&LocksmithOptions{
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

	client := client.NewClient(&client.ClientOptions{Host: "localhost", Port: 30001, OnAcquired: onAcquired})
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

	// clean up
	cancel()
	<-done
}
