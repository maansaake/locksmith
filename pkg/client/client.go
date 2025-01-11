// Package client provides a sample implementation of a Locksmith client.
package client

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"github.com/maansaake/locksmith/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// Client provides a simple interface for a Locksmith client.
type Client interface {
	Acquire(lockTag string) error
	Release(lockTag string) error
	Connect() error
	Close()
}

// ClientOptions to provide at client instantiation.
type ClientOptions struct {
	Host       string
	Port       uint16
	TlsConfig  *tls.Config
	OnAcquired func(lockTag string)
}

// Implements the Client interface.
type clientImpl struct {
	host       string
	port       uint16
	tlsConfig  *tls.Config
	onAcquired func(lockTag string)
	conn       net.Conn
	stop       chan interface{}
}

func New(options *ClientOptions) Client {
	return &clientImpl{
		host:       options.Host,
		port:       options.Port,
		tlsConfig:  options.TlsConfig,
		onAcquired: options.OnAcquired,
		stop:       make(chan interface{}),
	}
}

// Connect to Locksmith, returning an error in case there is some connectivity error.
// Special note: if TLS version 13 is used, the Connect() function will not return an
// error, even if something is wrong, until the first client write is issues. This is
// because of how TLS 13 is implemented.
func (clientImpl *clientImpl) Connect() (err error) {
	address := fmt.Sprintf("%s:%d", clientImpl.host, clientImpl.port)
	if clientImpl.tlsConfig != nil {
		log.Info().
			Str("address", address).
			Msg("dialing (TLS) server")
		clientImpl.conn, err = tls.Dial(
			"tcp",
			address,
			clientImpl.tlsConfig,
		)
	} else {
		log.Info().
			Str("address", address).
			Msg("dialing server")
		clientImpl.conn, err = net.Dial("tcp", address)
	}
	if err != nil {
		return err
	}
	log.Info().Msg("connected")

	go func(conn net.Conn) {
		defer conn.Close()
		buffer := make([]byte, 257)
		for {
			n, readErr := conn.Read(buffer)
			if readErr != nil {
				if readErr == io.EOF {
					log.Info().
						Str("address", conn.RemoteAddr().String()).
						Msg("connection closed by remote (EOF)")
				} else {
					select {
					case <-clientImpl.stop:
						log.Info().Msg("stopping client connection gracefully")
					default:
						log.Error().
							Err(readErr).
							Msg("connection read error: ")
					}
				}

				break
			}

			clientMessage, decodeErr := protocol.DecodeClientMessage(buffer[:n])
			if decodeErr != nil {
				log.Error().
					Err(decodeErr).
					Msg("failed to decode message")
				continue
			}

			switch clientMessage.Type {
			case protocol.Acquired:
				clientImpl.onAcquired(clientMessage.LockTag)
			default:
				log.Error().
					Str("type", string(clientMessage.Type)).
					Msg("Client message type not recognized: ")
			}
		}
	}(clientImpl.conn)

	return nil
}

// Close disconnects from the Locksmith instance.
func (clientImpl *clientImpl) Close() {
	close(clientImpl.stop)
	clientImpl.conn.Close()
}

// Acquire the given lock tag.
// When the server responds, the onAcquired callback is called with the acquired lock tag.
func (clientImpl *clientImpl) Acquire(lockTag string) error {
	_, writeErr := clientImpl.conn.Write(
		protocol.EncodeServerMessage(
			&protocol.ServerMessage{Type: protocol.Acquire, LockTag: lockTag},
		),
	)

	return writeErr
}

// Release the given lock tag.
func (clientImpl *clientImpl) Release(lockTag string) error {
	_, writeErr := clientImpl.conn.Write(
		protocol.EncodeServerMessage(
			&protocol.ServerMessage{Type: protocol.Release, LockTag: lockTag},
		),
	)

	return writeErr
}
