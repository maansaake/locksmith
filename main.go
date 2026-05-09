package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/maansaake/locksmith/internal/env"
	"github.com/maansaake/locksmith/internal/otel"
	locksmith "github.com/maansaake/locksmith/pkg/locksmith"
	"github.com/maansaake/locksmith/pkg/vault"

	"github.com/trebent/zerologr"
)

func main() {
	if err := env.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing environment variables: %v\n", err)
		os.Exit(1)
	}

	logger := zerologr.New(&zerologr.Opts{
		Console: env.LogOutput.Value() == "console",
		Caller:  true,
		V:       env.LogVerbosity.Value(),
	})
	zerologr.Set(logger)

	signalCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cleanup, err := setupMetrics(signalCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error setting up metrics: %v\n", err)
		os.Exit(1) //nolint:gocritic // intended to exit on metrics setup failure
	}
	defer cleanup(context.Background()) //nolint:errcheck // impossible to handle

	locksmithOptions := &locksmith.Opts{
		Version:          env.Version.Value(),
		Port:             uint16(env.Port.Value()), //nolint:gosec // validated on parse
		QueueType:        vault.QueueType(env.QueueType.Value()),
		QueueConcurrency: env.QueueConcurrency.Value(),
		QueueCapacity:    env.QueueCapacity.Value(),
	}

	if env.TLS.Value() {
		tlsConfig, err := getTLSConfig() //nolint:govet // shad
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading TLS config: %v\n", err)
			os.Exit(1)
		}

		locksmithOptions.TLSConfig = tlsConfig
	}

	zerologr.Info(
		"Starting locksmith",
		"port", locksmithOptions.Port,
		"tls_enabled", locksmithOptions.TLSConfig != nil,
		"queue_type", locksmithOptions.QueueType,
		"queue_concurrency", locksmithOptions.QueueConcurrency,
		"queue_capacity", locksmithOptions.QueueCapacity,
	)

	ls, err := locksmith.New(locksmithOptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Locksmith instance: %v\n", err)
		os.Exit(1)
	}

	if err := ls.Start(signalCtx); err != nil { //nolint:govet // shad
		fmt.Fprintf(os.Stderr, "Locksmith start error: %v\n", err)
		os.Exit(1)
	}
}

// setupMetrics initializes OpenTelemetry instrumentation if enabled via environment variables.
// It returns a shutdown function that should be deferred for proper cleanup.
func setupMetrics(ctx context.Context) (func(context.Context) error, error) {
	if env.ObservabilityEnabled.Value() {
		zerologr.Info("Observability enabled, instrumenting using OTEL", "runtime_metrics", env.RuntimeMetrics.Value())
		return otel.Instrument(ctx, "locksmith", env.Version.Value(), env.RuntimeMetrics.Value())
	}

	return func(context.Context) error { return nil }, nil
}

// getTLSConfig loads TLS configuration based on environment variables. It returns a
// tls.Config struct or an error if the configuration is invalid.
func getTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}

	cert, err := tls.LoadX509KeyPair(env.TLSCertPath.Value(), env.TLSKeyPath.Value())
	if err != nil {
		return nil, err
	}

	tlsConfig.Certificates = []tls.Certificate{cert}

	if env.TLSRequireClientCert.Value() {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		caCert, err := os.ReadFile(env.TLSClientCACertPath.Value()) //nolint:govet // shad
		if err != nil {
			return nil, err
		}

		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)
		tlsConfig.ClientCAs = pool
	}

	return tlsConfig, nil
}
