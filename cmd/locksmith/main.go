package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/maansthoernvik/locksmith/pkg/env"
	locksmith "github.com/maansthoernvik/locksmith/pkg/locksmith"
	"github.com/maansthoernvik/locksmith/pkg/vault"
	"github.com/maansthoernvik/locksmith/pkg/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Set global log level
	logLevel, _ := env.GetOptionalString(env.LOCKSMITH_LOG_LEVEL, env.LOCKSMITH_LOG_LEVEL_DEFAULT)
	zerolog.SetGlobalLevel(translateToZerologLevel(logLevel))

	logOutputEnv, err := env.GetOptionalString(env.LOCKSMITH_LOG_OUTPUT, env.LOCKSMITH_LOG_OUTPUT_DEFAULT)
	checkError(err)

	logOutput := os.Stderr
	if logOutputEnv == "stdout" {
		logOutput = os.Stdout
	}

	console, err := env.GetOptionalBool(env.LOCKSMITH_LOG_OUTPUT_CONSOLE, env.LOCKSMITH_LOG_OUTPUT_CONSOLE_DEFAULT)
	checkError(err)

	if console {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: logOutput})
	} else {
		log.Logger = log.Output(logOutput)
	}

	// Print to bypass loglevel settings and write to stdout
	// Check if '?' since the version info can only be set for container builds, not via 'go install'
	if version.Version != "?" {
		log.Info().
			Str("version", version.Version).
			Str("commit", version.Commit).
			Str("built", version.Built).
			Msg("starting Locksmith")
	} else {
		log.Info().Msg("starting locksmith")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Check if Prometheus metrics are enabled, start the metrics server if so.
	var metricsServer *http.Server
	metrics, err := env.GetOptionalBool(env.LOCKSMITH_METRICS, env.LOCKSMITH_METRICS_DEFAULT)
	checkError(err)

	if metrics {
		http.Handle("/metrics", promhttp.Handler())
		metricsServer = &http.Server{Addr: ":20000"}
		go func() {
			log.Info().Str("address", metricsServer.Addr).Msg("starting metrics server")
			if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
				log.Error().Err(err).Msg("metrics server failure")
			} else {
				log.Info().Msg("stopped metrics server")
			}
		}()
	}

	go func() {
		signal_ch := make(chan os.Signal, 1)
		signal.Notify(signal_ch, syscall.SIGINT, syscall.SIGTERM)
		signal := <-signal_ch
		log.Info().Any("signal", signal).Msg("captured stop signal")
		if metrics {
			if err := metricsServer.Shutdown(ctx); err != nil {
				log.Error().Err(err).Msg("error shutting down metrics server")
			}
		}
		cancel()
	}()

	port, err := env.GetOptionalUint16(env.LOCKSMITH_PORT, env.LOCKSMITH_PORT_DEFAULT)
	checkError(err)

	queueType, err := env.GetOptionalString(env.LOCKSMITH_Q_TYPE, env.LOCKSMITH_Q_TYPE_DEFAULT)
	checkError(err)

	concurrency, err := env.GetOptionalInteger(env.LOCKSMITH_Q_CONCURRENCY, env.LOCKSMITH_Q_CONCURRENCY_DEFAULT)
	checkError(err)

	capacity, err := env.GetOptionalInteger(env.LOCKSMITH_Q_CAPACITY, env.LOCKSMITH_Q_CAPACITY_DEFAULT)
	checkError(err)

	locksmithOptions := &locksmith.LocksmithOptions{
		Port:             port,
		QueueType:        vault.QueueType(queueType),
		QueueConcurrency: concurrency,
		QueueCapacity:    capacity,
	}

	tls, err := env.GetOptionalBool(env.LOCKSMITH_TLS, env.LOCKSMITH_TLS_DEFAULT)
	checkError(err)

	if tls {
		locksmithOptions.TlsConfig = getTlsConfig()
	}

	if err := locksmith.New(locksmithOptions).Start(ctx); err != nil {
		log.Error().Err(err).Msg("server start error")
		os.Exit(1)
	}

	log.Info().Msg("server stopped")
}

func translateToZerologLevel(level string) zerolog.Level {
	switch level {
	case "DEBUG":
		return zerolog.DebugLevel
	case "INFO":
		return zerolog.InfoLevel
	case "WARNING":
		return zerolog.WarnLevel
	case "ERROR":
		return zerolog.ErrorLevel
	case "FATAL":
		return zerolog.FatalLevel
	case "PANIC":
		return zerolog.PanicLevel
	}

	log.Warn().Msg("unable to decode log level")
	return zerolog.NoLevel
}

// Fetch TLS config to supply the TCP listener.
func getTlsConfig() *tls.Config {
	tlsConfig := &tls.Config{}

	serverCertPath, err := env.GetOptionalString(env.LOCKSMITH_TLS_CERT_PATH, env.LOCKSMITH_TLS_CERT_PATH_DEFAULT)
	checkError(err)

	serverKeyPath, err := env.GetOptionalString(env.LOCKSMITH_TLS_KEY_PATH, env.LOCKSMITH_TLS_KEY_PATH_DEFAULT)
	checkError(err)

	cert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	checkError(err)

	tlsConfig.Certificates = []tls.Certificate{cert}

	requireClientVerify, err := env.GetOptionalBool(env.LOCKSMITH_TLS_REQUIRE_CLIENT_CERT, env.LOCKSMITH_TLS_REQUIRE_CLIENT_CERT_DEFAULT)
	checkError(err)

	if requireClientVerify {
		clientCaCertPath, err := env.GetOptionalString(env.LOCKSMITH_TLS_CLIENT_CA_CERT_PATH, env.LOCKSMITH_TLS_CLIENT_CA_CERT_PATH_DEFAULT)
		checkError(err)

		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		caCert, err := os.ReadFile(clientCaCertPath)
		checkError(err)

		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)
		tlsConfig.ClientCAs = pool
	}

	return tlsConfig
}

// Panics if the error is not nil.
func checkError(err error) {
	if err != nil {
		panic(err)
	}
}
