// Package server ties together the Locksmith server logic.
package locksmith

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"slices"

	"github.com/maansaake/locksmith/pkg/connection"
	"github.com/maansaake/locksmith/pkg/protocol"
	"github.com/maansaake/locksmith/pkg/vault"
	"github.com/rs/zerolog/log"
)

type (
	// Locksmith is the root level object containing the implementation of the Locksmith server.
	Locksmith struct {
		tcpAcceptor connection.TCPAcceptor
		vault       vault.Vault
	}
	// Opts exposes the possible options to pass to a new Locksmith instance.
	Opts struct {
		// Denotes the port which will listen for incoming connections.
		Port uint16
		// Selects the type of queue layer the vault will use.
		QueueType vault.QueueType
		// Sets the number of synchronization threads, the higher the number the less the chance of congestion.
		QueueConcurrency int
		// Determines the buffer size of each synchronization thread, after the buffer limit is reached, calls
		// to the queue layer will block until the congestion is resolved.
		QueueCapacity int
		// TLS configuration for the TCP acceptor.
		TlsConfig *tls.Config
	}
	clientContext struct {
		conn     net.Conn
		lockTags []string
	}
)

// Create a new Locksmith instance with the provided options.
func New(options *Opts) *Locksmith {
	locksmith := &Locksmith{
		vault: vault.New(&vault.Opts{
			QueueType:        options.QueueType,
			QueueConcurrency: options.QueueConcurrency,
			QueueCapacity:    options.QueueCapacity,
		}),
	}
	locksmith.tcpAcceptor = connection.NewTCPAcceptor(&connection.TCPAcceptorOptions{
		Handler:   locksmith.handleConnection,
		Port:      options.Port,
		TlsConfig: options.TlsConfig,
	})

	return locksmith
}

// Starts the Locksmith instance. This is a blocking call that can be unblocked
// by cancelling the provided context.
func (locksmith *Locksmith) Start(ctx context.Context) error {
	err := locksmith.tcpAcceptor.Start()
	if err != nil {
		log.Error().Msg("failed to start TCP acceptor")
		return err
	}
	log.Info().Msg("started locksmith")

	<-ctx.Done()
	log.Info().Msg("stopping locksmith")
	locksmith.tcpAcceptor.Stop()

	return err
}

// Handler for connections accepted by the TCP acceptor. This function contains
// a connection loop which only ends upon the client connection encountering an
// error, either due to a problem or shutdown of the client connection. Gotten
// messages will be attempted to be decoded, if decoding fails the loop is
// broken and the client connection disconnected.
func (locksmith *Locksmith) handleConnection(conn net.Conn) {
	log.Info().
		Str("address", conn.RemoteAddr().String()).
		Msg("connection accepted")

	// On connection close, clean up client data
	clientContext := &clientContext{
		conn:     conn,
		lockTags: []string{},
	}
	defer locksmith.cleanup(clientContext)

	read := make([]byte, 256)
	buf := &bytes.Buffer{}
	for {
		n, err := clientContext.conn.Read(read)
		if err != nil {
			if err == io.EOF {
				log.Info().
					Str("address", clientContext.conn.RemoteAddr().String()).
					Msg("connection closed by remote (EOF)")
			} else {
				log.Error().
					Err(err).
					Msg("connection read error, closing connection")
			}

			break
		}

		log.Debug().Int("bytes", n).Msg("read from connection")
		log.Debug().Bytes("read", read[:n]).Send()

		buf.Write(read[:n])

		if err := locksmith.handleBuf(buf, clientContext); err != nil {
			log.Error().
				Err(err).
				Str("address", conn.RemoteAddr().String()).
				Msg("message handling error, closing connection")
			break
		}
	}

	// Close the connection, help cleanup in case of decoding issues.
	// May return an error if the remote closed it, but that doesn't matter,
	// ignore.
	_ = clientContext.conn.Close()
}

func (locksmith *Locksmith) handleBuf(buf *bytes.Buffer, clientContext *clientContext) error {
	contents := buf.Bytes()
	i := 0
	for {
		// Minimum size of a server message is 3 bytes, for a locktag of 1 char.
		if len(contents[i:]) < 3 {
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

		msg, err := protocol.DecodeServerMessage(buf.Next(lockTagSize + 2))
		if err != nil {
			return err
		}
		i += lockTagSize

		locksmith.handleMsg(clientContext, msg)
	}
}

// After decoding, this function determines the handling of the decoded
// message.
func (locksmith *Locksmith) handleMsg(
	clientContext *clientContext,
	serverMessage *protocol.ServerMessage,
) {
	switch serverMessage.Type {
	case protocol.Acquire:
		locksmith.vault.Acquire(
			serverMessage.LockTag,
			clientContext.conn.RemoteAddr().String(),
			locksmith.acquireCallback(clientContext.conn, serverMessage.LockTag),
		)
		clientContext.add(serverMessage.LockTag)
	case protocol.Release:
		locksmith.vault.Release(
			serverMessage.LockTag,
			clientContext.conn.RemoteAddr().String(),
			locksmith.releaseCallback(clientContext.conn),
		)
		clientContext.remove(serverMessage.LockTag)
	default:
		log.Error().Msg("invalid message type")
	}
}

func (locksmith *Locksmith) cleanup(clientContext *clientContext) {
	for _, lockTag := range clientContext.lockTags {
		locksmith.vault.Cleanup(lockTag, clientContext.conn.RemoteAddr().String())
	}
}

// Returns a callback function to call once a lock has been acquired, to send
// feedback down the client connection. If the callback is called with an error,
// the client has misbehaved in some way and needs to be disconnected.
func (locksmith *Locksmith) acquireCallback(
	conn net.Conn,
	lockTag string,
) func(error) error {
	return func(err error) error {
		if err != nil {
			log.Error().Err(err).Msg("got error in acquire callback")
			conn.Close()
			return nil
		}

		log.Debug().Str("locktag", lockTag).Msg("notifying client of acquisition")
		_, writeErr := conn.Write(protocol.EncodeClientMessage(&protocol.ClientMessage{
			Type:    protocol.Acquired,
			LockTag: lockTag,
		}))
		if writeErr != nil {
			log.Error().Err(writeErr).Msg("failed to write to client")
			return writeErr
		}

		return nil
	}
}

// Returns a callback function to call once a lock has been released. If the
// callback is called with an error, the client has misbehaved in some way and
// needs to be disconnected.
func (locksmith *Locksmith) releaseCallback(
	conn net.Conn,
) func(error) error {
	return func(err error) error {
		if err != nil {
			log.Error().Err(err).Msg("got error in release callback")
			conn.Close()
		}

		return nil
	}
}

func (clientContext *clientContext) add(lockTag string) {
	clientContext.lockTags = append(clientContext.lockTags, lockTag)
}

func (clientContext *clientContext) remove(lockTag string) {
	clientContext.lockTags = slices.DeleteFunc(clientContext.lockTags, func(lt string) bool { return lt == lockTag })
}
