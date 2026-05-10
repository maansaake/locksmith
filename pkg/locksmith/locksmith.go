// Package locksmith ties together the Locksmith server logic.
package locksmith

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"slices"

	"github.com/maansaake/locksmith/pkg/connection"
	"github.com/maansaake/locksmith/pkg/protocol"
	"github.com/maansaake/locksmith/pkg/vault"
	"github.com/trebent/zerologr"
)

type (
	// Locksmith is the root level object containing the implementation of the Locksmith server.
	Locksmith struct {
		tcpAcceptor connection.TCPAcceptor
		vault       vault.Vault
	}
	// Opts exposes the possible options to pass to a new Locksmith instance.
	Opts struct {
		// Used for telemetry.
		Version string

		// Port denotes the port which will listen for incoming connections.
		Port uint16

		// QueueType selects the type of queue layer the vault will use.
		QueueType vault.QueueType

		// QueueConcurrency sets the number of synchronization threads, the higher the number the less the chance of congestion.
		QueueConcurrency int

		// QueueCapacity determines the buffer size of each synchronization thread, after the buffer limit is reached, calls
		// to the queue layer will block until the congestion is resolved.
		QueueCapacity int

		// TLSConfig for the TCP acceptor.
		TLSConfig *tls.Config
	}
	clientContext struct {
		conn     net.Conn
		lockTags []string
	}
)

// New creates a new Locksmith instance with the provided options.
func New(opts *Opts) (*Locksmith, error) {
	vault, err := vault.New(&vault.Opts{
		Version:          opts.Version,
		QueueType:        opts.QueueType,
		QueueConcurrency: opts.QueueConcurrency,
		QueueCapacity:    opts.QueueCapacity,
	})
	if err != nil {
		return nil, err
	}

	locksmith := &Locksmith{vault: vault}
	locksmith.tcpAcceptor = connection.NewTCPAcceptor(&connection.TCPAcceptorOptions{
		Handler:   locksmith.handleConnection,
		Port:      opts.Port,
		TLSConfig: opts.TLSConfig,
	})

	return locksmith, nil
}

// Start the Locksmith instance. This is a blocking call that can be unblocked
// by cancelling the provided context.
func (l *Locksmith) Start(ctx context.Context) error {
	err := l.tcpAcceptor.Start()
	if err != nil {
		return err
	}
	zerologr.Info("Started locksmith")

	<-ctx.Done()
	zerologr.Info("Stopping locksmith")
	l.tcpAcceptor.Stop()

	return nil
}

// Handler for connections accepted by the TCP acceptor. This function contains
// a connection loop which only ends upon the client connection encountering an
// error, either due to a problem or shutdown of the client connection. Gotten
// messages will be attempted to be decoded, if decoding fails the loop is
// broken and the client connection disconnected.
func (l *Locksmith) handleConnection(conn net.Conn) {
	const bufSize = 256

	zerologr.Info("Connection accepted", "address", conn.RemoteAddr().String())

	// On connection close, clean up client data
	clientContext := &clientContext{
		conn:     conn,
		lockTags: []string{},
	}
	defer l.cleanup(clientContext)

	read := make([]byte, bufSize)
	buf := &bytes.Buffer{}
	for {
		n, err := clientContext.conn.Read(read)
		if err != nil {
			if errors.Is(err, io.EOF) {
				zerologr.Info("Connection closed by remote (EOF)", "address", clientContext.conn.RemoteAddr().String())
			} else {
				zerologr.Error(err, "Connection read error, closing connection")
			}

			break
		}

		zerologr.V(50).Info("Read from connection", "bytes", n)
		zerologr.V(50).Info("", "read", read[:n])

		buf.Write(read[:n])

		//nolint:govet // TODO: look into
		if err := l.handleBuf(buf, clientContext); err != nil {
			zerologr.Error(err, "Message handling error, closing connection", "address", conn.RemoteAddr().String())
			break
		}
	}
}

func (l *Locksmith) handleBuf(buf *bytes.Buffer, clientContext *clientContext) error {
	const minMsgSize = 3

	contents := buf.Bytes()
	i := 0
	for {
		// Minimum size of a server message is 3 bytes, for a locktag of 1 char.
		if len(contents[i:]) < minMsgSize {
			return nil
		}

		// Peek the buffer, check message type and lock tag size. If the full lock
		// tag size exists then a server message decode is done leading to a
		// consequent message handling.
		// Otherwise, put everything back in the buffer and return.

		// No need to check msg type since they all currently have a locktag to
		// verify.
		// msgType := contents[i]
		i++
		lockTagSize := int(contents[i])
		i++
		if len(contents[i:]) < lockTagSize {
			return nil
		}

		// Header size is 2 bytes, for the message type and lock tag size.
		const headerSize = 2

		msg, err := protocol.DecodeServerMessage(buf.Next(lockTagSize + headerSize))
		if err != nil {
			return err
		}
		i += lockTagSize

		l.handleMsg(clientContext, msg)
	}
}

// After decoding, this function determines the handling of the decoded
// message.
func (l *Locksmith) handleMsg(
	clientContext *clientContext,
	serverMessage *protocol.ServerMessage,
) {
	switch serverMessage.Type {
	case protocol.Acquire:
		l.vault.Acquire(
			serverMessage.LockTag,
			clientContext.conn.RemoteAddr().String(),
			l.acquireCallback(clientContext.conn, serverMessage.LockTag),
		)
		clientContext.add(serverMessage.LockTag)
	case protocol.Release:
		l.vault.Release(
			serverMessage.LockTag,
			clientContext.conn.RemoteAddr().String(),
			l.releaseCallback(clientContext.conn),
		)
		clientContext.remove(serverMessage.LockTag)
	default:
		zerologr.Info("Invalid message type")
	}
}

func (l *Locksmith) cleanup(clientContext *clientContext) {
	_ = clientContext.conn.Close()
	for _, lockTag := range clientContext.lockTags {
		l.vault.Cleanup(lockTag, clientContext.conn.RemoteAddr().String())
	}
}

// Returns a callback function to call once a lock has been acquired, to send
// feedback down the client connection. If the callback is called with an error,
// the client has misbehaved in some way and needs to be disconnected.
func (l *Locksmith) acquireCallback(
	conn net.Conn,
	lockTag string,
) func(error) error {
	return func(err error) error {
		if err != nil {
			zerologr.Error(err, "Got error in acquire callback")
			_ = conn.Close()
			return nil
		}

		zerologr.V(50).Info("Notifying client of acquisition", "locktag", lockTag)
		_, writeErr := conn.Write(protocol.EncodeClientMessage(&protocol.ClientMessage{
			Type:    protocol.Acquired,
			LockTag: lockTag,
		}))
		if writeErr != nil {
			zerologr.Error(writeErr, "Failed to write to client")
			return writeErr
		}

		return nil
	}
}

// Returns a callback function to call once a lock has been released. If the
// callback is called with an error, the client has misbehaved in some way and
// needs to be disconnected.
func (l *Locksmith) releaseCallback(
	conn net.Conn,
) func(error) error {
	return func(err error) error {
		if err != nil {
			zerologr.Error(err, "Got error in release callback")
			_ = conn.Close()
		}

		return nil
	}
}

func (cc *clientContext) add(lockTag string) {
	cc.lockTags = append(cc.lockTags, lockTag)
}

func (cc *clientContext) remove(lockTag string) {
	cc.lockTags = slices.DeleteFunc(cc.lockTags, func(lt string) bool { return lt == lockTag })
}
