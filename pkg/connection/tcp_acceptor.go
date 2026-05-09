// Package connection implements a simple TCP server, allowing Locksmith to accept connections.
package connection

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/trebent/zerologr"
)

type TCPAcceptor interface {
	Start() error
	Stop()
}

type TCPAcceptorOptions struct {
	Handler   func(net.Conn)
	Port      uint16
	TLSConfig *tls.Config
}

type tcpAcceptorImpl struct {
	port      uint16
	handler   func(net.Conn)
	tlsConfig *tls.Config
	listener  net.Listener
	stop      chan interface{}
}

func NewTCPAcceptor(options *TCPAcceptorOptions) TCPAcceptor {
	return &tcpAcceptorImpl{
		port:      options.Port,
		handler:   options.Handler,
		tlsConfig: options.TLSConfig,
		stop:      make(chan interface{}),
	}
}

// Starts the TCP acceptor, returning any error that happened due to the call
// to net/tls.Listen(...).
// This is NOT a blocking call.
func (tcpAcceptor *tcpAcceptorImpl) Start() error {
	var err error
	if tcpAcceptor.tlsConfig == nil {
		//nolint:noctx // TODO: fix
		tcpAcceptor.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", tcpAcceptor.port))
		zerologr.Info("Starting listener", "port", tcpAcceptor.port)
	} else {
		tcpAcceptor.listener, err = tls.Listen("tcp", fmt.Sprintf(":%d", tcpAcceptor.port), tcpAcceptor.tlsConfig)
		zerologr.Info("Starting TLS listener on port", "port", tcpAcceptor.port)
	}
	if err != nil {
		return err
	}

	go tcpAcceptor.startListener()

	return nil
}

// Stop the TCP acceptor gracefully.
func (tcpAcceptor *tcpAcceptorImpl) Stop() {
	zerologr.Info("Stopping TCP acceptor")
	close(tcpAcceptor.stop)
	_ = tcpAcceptor.listener.Close()
}

// Listening loop for the TCP acceptor, is able to stop gracefully if Stop()
// is called. Any incoming connection is dispatched to the registered handler.
func (tcpAcceptor *tcpAcceptorImpl) startListener() {
	defer tcpAcceptor.listener.Close()
	for {
		conn, err := tcpAcceptor.listener.Accept()
		if err != nil {
			select {
			case <-tcpAcceptor.stop:
				zerologr.Info("Stopping accept loop gracefully")
			default:
				zerologr.Error(err, "A non stop related error occurred")
			}
			break
		}
		zerologr.V(50).Info("Listener accepted connection", "address", conn.RemoteAddr().String())

		go func() {
			defer conn.Close()
			tcpAcceptor.handler(conn)
		}()
	}
}
