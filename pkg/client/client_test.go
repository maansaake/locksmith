package client

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/maansaake/locksmith/pkg/protocol"
)

func Test_ClientLifecycle(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:30005")
	if err != nil {
		t.Fatal("Failed to start listener:", err)
	}

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			t.Log("Client established connection")
			wg.Done()

			for {
				t.Log("Reading from client connection")
				buffer := make([]byte, 100)
				_, err = conn.Read(buffer)
				t.Log("Read from client connection")
				if err == io.EOF {
					t.Log("Client closed connection")
					wg.Done()
					return
				}
			}
		}
	}()

	client := NewClient(&ClientOptions{Host: "localhost", Port: 30005})
	startErr := client.Connect()
	if err != nil {
		t.Fatal("Failed to start client:", startErr)
	}
	client.Close()

	wg.Wait()

	listener.Close()
}

func Test_ClientAcquireRelease(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:30006")
	if err != nil {
		t.Fatal("Failed to start listener:", err)
	}

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			t.Log("Client established connection")

			for {
				t.Log("Reading from client connection")
				buffer := make([]byte, 100)
				n, err := conn.Read(buffer)
				t.Log("Read from client connection")
				if err == io.EOF {
					t.Log("Client closed connection")
					return
				}

				serverMessage, err := protocol.DecodeServerMessage(buffer[:n])
				if err != nil {
					t.Error("Error decoding server message:", err)
					return
				}

				if serverMessage.Type == protocol.Acquire {
					t.Log("Acquire received")
					wg.Done()
				} else if serverMessage.Type == protocol.Release {
					t.Log("Release received")
					wg.Done()
				}
			}
		}
	}()

	client := NewClient(&ClientOptions{Host: "localhost", Port: 30006})
	startErr := client.Connect()
	if err != nil {
		t.Fatal("Failed to start client:", startErr)
	}
	_ = client.Acquire("123")
	time.Sleep(1 * time.Millisecond)
	_ = client.Release("123")

	wg.Wait()

	client.Close()
	listener.Close()
}

func Test_ClientOnAcquired(t *testing.T) {
	EXPECTED_LOCK_TAG := "locktag"

	listener, err := net.Listen("tcp", "localhost:30007")
	if err != nil {
		t.Fatal("Failed to start listener:", err)
	}

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			t.Log("Client established connection")

			for {
				t.Log("Reading from client connection")
				buffer := make([]byte, 100)
				n, err := conn.Read(buffer)
				t.Log("Read from client connection")
				if err == io.EOF {
					t.Log("Client closed connection")
					return
				}

				serverMessage, err := protocol.DecodeServerMessage(buffer[:n])
				if err != nil {
					t.Error("Error decoding server message:", err)
					return
				}

				if serverMessage.Type == protocol.Acquire {
					t.Log("Acquire received")
					wg.Done()

					_, err := conn.Write(protocol.EncodeClientMessage(
						&protocol.ClientMessage{Type: protocol.Acquired, LockTag: serverMessage.LockTag},
					))
					if err != nil {
						t.Error("Got error on write:", err)
					}
				}
			}
		}
	}()

	client := NewClient(&ClientOptions{Host: "localhost", Port: 30007, OnAcquired: func(lockTag string) {
		if lockTag == EXPECTED_LOCK_TAG {
			t.Log("OnAcquired called")
			wg.Done()
		}
	}})
	startErr := client.Connect()
	if err != nil {
		t.Fatal("Failed to start client:", startErr)
	}
	_ = client.Acquire(EXPECTED_LOCK_TAG)

	wg.Wait()
	client.Close()
	listener.Close()
}

func Test_MutualTls(t *testing.T) {
	cert, err := tls.LoadX509KeyPair("testcerts/testcert.pem", "testcerts/testkey.key")
	if err != nil {
		t.Error("Error when loading cert and key pair", err)
	}

	clientCaCert, err := os.ReadFile("testcerts/rootCACert.pem")
	if err != nil {
		t.Error("Failed to read client CA cert:", err)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(clientCaCert)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS13,
	}

	t.Log("Creating listener")
	listener, err := tls.Listen("tcp", "localhost:30008", tlsConfig)
	if err != nil {
		t.Fatal(err)
	}

	// Lifecycle wait group for client/listener tests.
	wg := sync.WaitGroup{}
	wg.Add(2)

	shutdownWg := sync.WaitGroup{}
	shutdownWg.Add(1)
	go func() {
		for {
			t.Log("Waiting for connections...")
			conn, err := listener.Accept()
			if err != nil {
				t.Log("Listener error:", err)
				break
			}

			t.Log("Accepted connection from", conn.RemoteAddr().String())
			go func(conn net.Conn) {
				defer conn.Close()
				_, err := conn.Read(make([]byte, 25))
				t.Log("Got bytes from client...")
				if err != nil {
					t.Error("Error reading:", err)
					wg.Done()
				}

				//nolint
				conn.Write(protocol.EncodeClientMessage(
					&protocol.ClientMessage{
						Type:    protocol.Acquired,
						LockTag: "abc",
					},
				))

				wg.Done()
			}(conn)
		}

		shutdownWg.Done()
	}()

	clientCert, err := tls.LoadX509KeyPair("testcerts/testcert.pem", "testcerts/testkey.key")
	if err != nil {
		t.Error(err)
	}

	t.Log("Creating client")
	c := &clientImpl{
		host: "localhost",
		port: 30008,
		onAcquired: func(lockTag string) {
			t.Log("Client got acquired signal for lock tag:", lockTag)
			wg.Done()
		},
		tlsConfig: &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      pool,
			MinVersion:   tls.VersionTLS13,
		},
		stop: make(chan interface{}),
	}
	t.Log("Connecting client")
	err = c.Connect()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Client connected")

	writeErr := c.Acquire("abc")
	if writeErr != nil {
		t.Error("Got unexpected write error:", writeErr)
	}

	t.Log("waiting on listener read")
	wg.Wait()
	t.Log("done waiting on listener read")

	t.Log("Shutting down listener and client...")
	listener.Close()
	c.Close()

	t.Log("waiting for listener to exit accept loop")
	shutdownWg.Wait()
}
