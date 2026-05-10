// Package client provides a sample implementation of a Locksmith client.
package client

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strconv"

	"github.com/maansaake/locksmith/pkg/protocol"
	"github.com/trebent/zerologr"
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
		TLSConfig  *tls.Config
		OnAcquired func(lockTag string)
	}
	// Implements the Client interface.
	clientImpl struct {
		host       string
		port       uint16
		tlsConfig  *tls.Config
		onAcquired func(lockTag string)
		conn       net.Conn

		running bool
		stopped bool
	}
)

func New(options *Opts) Client {
	return &clientImpl{
		host:       options.Host,
		port:       options.Port,
		tlsConfig:  options.TLSConfig,
		onAcquired: options.OnAcquired,

		running: false,
		stopped: false,
	}
}

// Connect to Locksmith, returning an error in case there is some connectivity error.
// Special note: if TLS version 13 is used, the Connect() function will not return an
// error, even if something is wrong, until the first client write is issues. This is
// because of how TLS 13 is implemented.
func (c *clientImpl) Connect() error {
	if c.running {
		zerologr.Info("Client already running, skipping Connect()")
		return nil
	}

	var err error
	address := net.JoinHostPort(c.host, strconv.FormatUint(uint64(c.port), 10))
	if c.tlsConfig != nil {
		zerologr.Info("Dialing (TLS) server", "address", address)
		//nolint:noctx // TODO: fix
		c.conn, err = tls.Dial(
			"tcp",
			address,
			c.tlsConfig,
		)
	} else {
		zerologr.Info("Dialing server", "address", address)
		//nolint:noctx // TODO: fix
		c.conn, err = net.Dial("tcp", address)
	}
	if err != nil {
		zerologr.Error(err, "Failed to connect to server", "address", address)
		return err
	}
	zerologr.Info("Connected")

	c.running = true
	c.stopped = false
	go c.handleConnection()

	return nil
}

func (c *clientImpl) handleConnection() {
	const bufSize = 256

	defer c.conn.Close()

	read := make([]byte, bufSize)
	buf := &bytes.Buffer{}
	for {
		n, err := c.conn.Read(read)
		if err != nil { //nolint:nestif // it is what it is
			if errors.Is(err, io.EOF) {
				zerologr.Info("Connection closed by remote (EOF)", "address", c.conn.RemoteAddr().String())
			} else {
				if c.stopped {
					zerologr.Info("Stopping client connection gracefully")
				} else {
					zerologr.Error(err, "Connection read error")
				}
			}

			break
		}

		zerologr.V(50).Info("Read from connection", "bytes", n)
		zerologr.V(50).Info("", "read", read[:n])

		buf.Write(read[:n])

		//nolint:govet // TODO: look into
		if err := c.handleBuf(buf); err != nil {
			zerologr.Error(err, "Message handling error, closing connection", "address", c.conn.RemoteAddr().String())
			break
		}
	}

	c.running = false
}

func (c *clientImpl) handleBuf(buf *bytes.Buffer) error {
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

		msg, err := protocol.DecodeClientMessage(buf.Next(lockTagSize + headerSize))
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
		zerologr.Info("Client message type not recognized", "type", msg.Type)
	}
}

// Close disconnects from the Locksmith instance. Safe to call multiple times.
func (c *clientImpl) Close() {
	c.stopped = true
	_ = c.conn.Close()
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
