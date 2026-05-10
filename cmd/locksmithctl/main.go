// Package main implements an interactive CLI for Locksmith.
//
// nolint
package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/maansaake/locksmith/pkg/client"
	"github.com/trebent/zerologr"
)

const usageText = `locksmithctl is an interactive CLI for acquiring and releasing Locksmith locks.`

const helpText = `Commands:
  acquire <lock>  Acquire a lock with the given tag
  release <lock>  Release a lock with the given tag
  list            List all currently acquired locks
  reconnect       Reconnect to the Locksmith server
  help            Show this help message
  exit            Exit locksmithctl`

var (
	address              string
	clientCertPath       string
	clientPrivateKeyPath string
	caCertPath           string
)

var errExit = errors.New("exit")

// lockApp holds the state for the interactive CLI session.
type lockApp struct {
	mu        sync.Mutex
	locks     []string
	c         client.Client
	connected bool
	host      string
	port      uint16
	tlsConfig *tls.Config
}

func main() {
	zerologr.Set(logr.Discard())

	flag.StringVar(&address, "address", "localhost:9090", "Locksmith server address (host:port).")
	flag.StringVar(&clientCertPath, "cert", "", "Absolute path to a PEM encoded certificate.")
	flag.StringVar(&clientPrivateKeyPath, "private-key", "", "Absolute path to a PEM encoded private key.")
	flag.StringVar(
		&caCertPath,
		"ca-cert",
		"",
		"Absolute path to a PEM encoded CA certificate which signed the server certificate.",
	)

	flag.Usage = func() {
		fmt.Println(usageText)
		fmt.Println()
		flag.CommandLine.PrintDefaults()
	}

	flag.Parse()

	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid address %q: %v\n", address, err)
		os.Exit(1)
	}

	portNum, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid port in address %q: %v\n", address, err)
		os.Exit(1)
	}

	tlsConfig, err := buildTLSConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading TLS config: %v\n", err)
		os.Exit(1)
	}

	app := &lockApp{
		host:      host,
		port:      uint16(portNum), //nolint:gosec // validated by ParseUint with bitSize 16
		tlsConfig: tlsConfig,
	}

	if err := app.run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func (a *lockApp) run() error {
	fmt.Println("Starting locksmithctl...")

	a.c = client.New(&client.Opts{
		Host:      a.host,
		Port:      a.port,
		TLSConfig: a.tlsConfig,
		OnAcquired: func(lockTag string) {
			a.addLock(lockTag)
			fmt.Printf("\nAcquired: %s\n> ", lockTag)
		},
		OnDisconnected: func() {
			a.mu.Lock()
			a.connected = false
			a.mu.Unlock()
			fmt.Println("\nDisconnected from server. Use 'reconnect' to reconnect.")
			fmt.Print("> ")
		},
	})

	if err := a.c.Connect(); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", net.JoinHostPort(a.host, strconv.Itoa(int(a.port))), err)
	}

	a.mu.Lock()
	a.connected = true
	a.mu.Unlock()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		a.c.Close()
		fmt.Println("\nExiting.")
		os.Exit(0)
	}()

	fmt.Printf("Connected to %s\n\n", net.JoinHostPort(a.host, strconv.Itoa(int(a.port))))
	fmt.Println(helpText)
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if err := a.handleCommand(strings.Fields(line)); errors.Is(err, errExit) {
			break
		}
	}

	a.c.Close()
	return nil
}

func (a *lockApp) handleCommand(args []string) error {
	switch args[0] {
	case "exit", "quit":
		return errExit

	case "help":
		fmt.Println(helpText)

	case "list":
		a.mu.Lock()
		defer a.mu.Unlock()
		if len(a.locks) == 0 {
			fmt.Println("No locks acquired.")
		} else {
			fmt.Printf("Acquired locks (%d):\n", len(a.locks))
			for i, l := range a.locks {
				fmt.Printf("  %d. %s\n", i+1, l)
			}
		}

	case "acquire":
		if len(args) != 2 {
			fmt.Println("Usage: acquire <lock>")
			return nil
		}
		if !a.isConnected() {
			fmt.Println("Not connected. Use 'reconnect' to reconnect.")
			return nil
		}
		if err := a.c.Acquire(args[1]); err != nil {
			fmt.Printf("Error acquiring lock: %v\n", err)
		}

	case "release":
		if len(args) != 2 {
			fmt.Println("Usage: release <lock>")
			return nil
		}
		if !a.isConnected() {
			fmt.Println("Not connected. Use 'reconnect' to reconnect.")
			return nil
		}
		if err := a.c.Release(args[1]); err != nil {
			fmt.Printf("Error releasing lock: %v\n", err)
			return nil
		}
		a.removeLock(args[1])

	case "reconnect":
		if a.isConnected() {
			fmt.Println("Already connected.")
			return nil
		}
		fmt.Printf("Reconnecting to %s...\n", net.JoinHostPort(a.host, strconv.Itoa(int(a.port))))
		if err := a.c.Connect(); err != nil {
			fmt.Printf("Failed to reconnect: %v\n", err)
		} else {
			a.mu.Lock()
			a.connected = true
			a.mu.Unlock()
			fmt.Printf("Reconnected to %s.\n", net.JoinHostPort(a.host, strconv.Itoa(int(a.port))))
		}

	default:
		fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", args[0])
	}

	return nil
}

func (a *lockApp) isConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connected
}

func (a *lockApp) addLock(lockTag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.locks = append(a.locks, lockTag)
}

func (a *lockApp) removeLock(lockTag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, l := range a.locks {
		if l == lockTag {
			a.locks = append(a.locks[:i], a.locks[i+1:]...)
			return
		}
	}
}

func buildTLSConfig() (*tls.Config, error) {
	if clientCertPath == "" && clientPrivateKeyPath == "" && caCertPath == "" {
		return nil, nil
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}

	if clientCertPath != "" && clientPrivateKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(clientCertPath, clientPrivateKeyPath)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if caCertPath != "" {
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = pool
	}

	return tlsConfig, nil
}
