// Package main implements an interactive CLI for Locksmith.
//
// nolint
package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/trebent/zerologr"
)

var (
	address              string
	clientCertPath       string
	clientPrivateKeyPath string
	caCertPath           string
)

func main() {
	zerologr.Set(logr.Discard())

	rootCmd := &cobra.Command{
		Use:           "locksmithctl",
		Short:         "Interactive CLI for Locksmith lock management",
		Long:          "locksmithctl is an interactive CLI for acquiring and releasing Locksmith locks.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return startApp()
		},
	}

	rootCmd.Flags().StringVar(&address, "address", "localhost:9090", "Locksmith server address (host:port).")
	rootCmd.Flags().StringVar(&clientCertPath, "cert", "", "Absolute path to a PEM encoded certificate.")
	rootCmd.Flags().StringVar(&clientPrivateKeyPath, "private-key", "", "Absolute path to a PEM encoded private key.")
	rootCmd.Flags().StringVar(
		&caCertPath,
		"ca-cert",
		"",
		"Absolute path to a PEM encoded CA certificate which signed the server certificate.",
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Render("Error: ")+err.Error())
		os.Exit(1)
	}
}

// startApp parses the connection address, builds a TLS config if needed,
// constructs the lockApp, and launches the interactive REPL.
func startApp() error {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", address, err)
	}

	portNum, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid port in address %q: %w", address, err)
	}

	tlsConfig, err := buildTLSConfig()
	if err != nil {
		return fmt.Errorf("TLS config error: %w", err)
	}

	app := newLockApp(host, uint16(portNum), tlsConfig) //nolint:gosec // validated by ParseUint with bitSize 16
	return app.run()
}

// buildTLSConfig constructs a *tls.Config from the TLS flags. It returns nil
// when no TLS flags are provided (plain TCP connection).
func buildTLSConfig() (*tls.Config, error) {
	if clientCertPath == "" && clientPrivateKeyPath == "" && caCertPath == "" {
		return nil, nil
	}

	cfg := &tls.Config{MinVersion: tls.VersionTLS13}

	if clientCertPath != "" && clientPrivateKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(clientCertPath, clientPrivateKeyPath)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	if caCertPath != "" {
		ca, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(ca)
		cfg.RootCAs = pool
	}

	return cfg, nil
}
