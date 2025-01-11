// Package client provides a sample implementation of a Locksmith client.
package client

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"github.com/maansaake/locksmith/pkg/protocol"
	"github.com/rs/zerolog/log"
)

type (
	// Client provides a simple interface for a Locksmith client.
	Client interface {
		Acquire(lockTag string) error
		Release(lockTag string) error
		Connect() error
		Close()
	}
	// Opts to provide at client instantiation.
	Opts struct {
		Host       string
		Port       uint16
		TlsConfig  *tls.Config
		OnAcquired func(lockTag string)
	}
	// Implements the Client interface.
	clientImpl struct {
		host       string
		port       uint16
		tlsConfig  *tls.Config
		onAcquired func(lockTag string)
		conn       net.Conn
		stop       chan interface{}
	}
)

func New(options *Opts) Client {
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
func (c *clientImpl) Connect() (err error) {
	address := fmt.Sprintf("%s:%d", c.host, c.port)
	if c.tlsConfig != nil {
		log.Info().
			Str("address", address).
			Msg("dialing (TLS) server")
		c.conn, err = tls.Dial(
			"tcp",
			address,
			c.tlsConfig,
		)
	} else {
		log.Info().
			Str("address", address).
			Msg("dialing server")
		c.conn, err = net.Dial("tcp", address)
	}
	if err != nil {
		return err
	}
	log.Info().Msg("connected")

	go c.handleConnection()

	return nil
}

func (c *clientImpl) handleConnection() {
	defer c.conn.Close()

	read := make([]byte, 256)
	buf := &bytes.Buffer{}
	for {
		n, err := c.conn.Read(read)
		if err != nil {
			if err == io.EOF {
				log.Info().
					Str("address", c.conn.RemoteAddr().String()).
					Msg("connection closed by remote (EOF)")
			} else {
				select {
				case <-c.stop:
					log.Info().Msg("stopping client connection gracefully")
				default:
					log.Error().
						Err(err).
						Msg("connection read error: ")
				}
			}

			break
		}

		log.Debug().Int("bytes", n).Msg("read from connection")
		log.Debug().Bytes("read", read[:n]).Send()

		buf.Write(read[:n])

		if err := c.handleBuf(buf); err != nil {
			log.Error().
				Err(err).
				Str("address", c.conn.RemoteAddr().String()).
				Msg("message handling error, closing connection")
			break
		}
	}
}

func (c *clientImpl) handleBuf(buf *bytes.Buffer) error {
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

		msg, err := protocol.DecodeClientMessage(buf.Next(lockTagSize + 2))
		if err != nil {
			return err
		}
		i += lockTagSize

		c.handleMsg(msg)
	}
}

func (c *clientImpl) handleMsg(msg *protocol.ClientMessage) {
	switch msg.Type {
	case protocol.Acquired:
		c.onAcquired(msg.LockTag)
	default:
		log.Error().
			Str("type", string(msg.Type)).
			Msg("Client message type not recognized: ")
	}
}

// Close disconnects from the Locksmith instance.
func (c *clientImpl) Close() {
	close(c.stop)
	c.conn.Close()
}

// Acquire the given lock tag.
// When the server responds, the onAcquired callback is called with the acquired lock tag.
func (c *clientImpl) Acquire(lockTag string) error {
	_, writeErr := c.conn.Write(
		protocol.EncodeServerMessage(
			&protocol.ServerMessage{Type: protocol.Acquire, LockTag: lockTag},
		),
	)

	return writeErr
}

// Release the given lock tag.
func (c *clientImpl) Release(lockTag string) error {
	_, writeErr := c.conn.Write(
		protocol.EncodeServerMessage(
			&protocol.ServerMessage{Type: protocol.Release, LockTag: lockTag},
		),
	)

	return writeErr
}
