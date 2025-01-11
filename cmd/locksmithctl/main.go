// This package implements a simple CLI for Locksmith.
package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/maansaake/locksmith/pkg/client"
	"github.com/rs/zerolog"
)

const USAGE = `Starts a session towards a Locksmith instance using the sample Locksmith
client implementation.`

const COMMANDS = `Session started, the following commands are supported:

acquire [lock]
release [lock]`

var (
	ErrExit              = errors.New("exiting")
	host                 string
	port                 uint
	clientCertPath       string
	clientPrivateKeyPath string
	caCertPath           string
	c                    client.Client
)

func main() {
	// Disables logging globally
	zerolog.SetGlobalLevel(zerolog.NoLevel)

	flag.StringVar(&host, "host", "localhost", "Locksmith hostname or IP address.")
	flag.UintVar(&port, "port", 9000, "Locksmith port number.")
	flag.StringVar(&clientCertPath, "cert", "", "Absolute path to a PEM encoded certificate.")
	flag.StringVar(&clientPrivateKeyPath, "private-key", "", "Absolute path to a PEM encoded private key.")
	flag.StringVar(&caCertPath, "ca-cert", "", "Absolute path to a PEM encoded CA certificate which signed the server certificate.")

	flag.Usage = func() {
		fmt.Println(USAGE)
		fmt.Println()
		flag.CommandLine.PrintDefaults()
	}

	flag.Parse()
	if err := run(); err != nil && !errors.Is(err, ErrExit) {
		fmt.Printf("ran into an error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("Starting Locksmith shell...")
	err := initClient()
	if err != nil {
		return err
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		c.Close()
		fmt.Println("CLOSED:", fmt.Sprintf("%s:%d", host, port))
		os.Exit(0)
	}()

	fmt.Println("CONNECTED:", fmt.Sprintf("%s:%d", host, port))
	fmt.Println("")
	fmt.Println(COMMANDS)

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		text, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		cleanedText := strings.Split(text[:len(text)-1], " ")

		err = handleCommand(cleanedText)
		if err != nil {
			return err
		}
	}
}

func handleCommand(
	cmd []string,
) error {
	switch cmd[0] {
	case "exit":
		return ErrExit

	case "acquire":
		if len(cmd) != 2 {
			return errors.New("expected 'acquire' followed by a lock")
		}
		lock := cmd[1]
		err := c.Acquire(lock)
		if err != nil {
			return err
		}

	case "release":
		if len(cmd) != 2 {
			return errors.New("expected 'release' followed by a lock")
		}
		lock := cmd[1]
		err := c.Release(lock)
		if err != nil {
			return err
		}

	default:
		if cmd[0] != "" {
			fmt.Println("did not recognize command:", cmd[0])
		}
	}

	return nil
}

func initClient() error {
	var tlsConfig *tls.Config
	if (clientCertPath != "" && clientPrivateKeyPath != "") || caCertPath != "" {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS13}

		if clientCertPath != "" && clientPrivateKeyPath != "" {
			clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientPrivateKeyPath)
			if err != nil {
				return err
			}
			tlsConfig.Certificates = []tls.Certificate{clientCert}
		}

		if caCertPath != "" {
			caCert, err := os.ReadFile(caCertPath)
			if err != nil {
				return err
			}
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCert)

			tlsConfig.RootCAs = pool
		}
	}

	c = client.New(&client.ClientOptions{
		Host: host,
		//nolint:gosec
		Port:      uint16(port),
		TlsConfig: tlsConfig,
		OnAcquired: func(lock string) {
			fmt.Println("acquired ", lock)
		},
	})

	return c.Connect()
}
