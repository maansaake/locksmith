// Package env provides some rudimentary environment variable parsing.
//
//nolint:gochecknoglobals // intended for simplicity of use
package env

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/trebent/envparser"
)

const (
	defaultLogVerbosity = 0
	defaultLogOutput    = "console"

	defaultVersion = "unknown"
	defaultCommit  = "unknown"

	defaultMetrics = false

	defaultPort        = 9000
	defaultMetricsPort = 20000

	defaultQueueType        = "multi"
	defaultQueueConcurrency = 10
	defaultQueueCapacity    = 100

	defaultTLS                  bool   = false
	defaultTLSCertParth         string = "/etc/cert/locksmith.pem"
	defaultTLSKeyPath           string = "/etc/cert/locksmith.key"
	defaultTLSRequireClientCert bool   = false
	defaultTLSClientCACertPath  string = "/etc/cert/client_ca.cert"
)

var (
	LogVerbosity = envparser.Register(&envparser.Opts[int]{
		Name:  "LOG_VERBOSITY",
		Desc:  "The verbosity level for logging. Higher values indicate more verbose logging.",
		Value: defaultLogVerbosity,
		Validate: func(v int) error {
			if v < 0 {
				return fmt.Errorf("verbosity level must be non-negative, got %d", v)
			}
			return nil
		},
	})
	LogOutput = envparser.Register(&envparser.Opts[string]{
		Name:  "LOG_OUTPUT",
		Desc:  "The output for logging. Can be 'json', or 'console'.",
		Value: defaultLogOutput,
		Validate: func(v string) error {
			if !slices.Contains([]string{"json", "console"}, v) {
				return fmt.Errorf("invalid log output '%s', must be 'json' or 'console'", v)
			}
			return nil
		},
	})
	Version = envparser.Register(&envparser.Opts[string]{
		Name:  "VERSION",
		Desc:  "The version of the application. This is used for informational purposes and does not affect application behavior.",
		Value: defaultVersion,
	})
	Commit = envparser.Register(&envparser.Opts[string]{
		Name:  "COMMIT",
		Desc:  "The commit hash of the application. This is used for informational purposes and does not affect application behavior.",
		Value: defaultCommit,
	})

	ObservabilityEnabled = envparser.Register(&envparser.Opts[bool]{
		Name:  "OBSERVABILITY",
		Desc:  "Whether to enable Prometheus metrics. If true, a /metrics endpoint will be exposed on the server.",
		Value: defaultMetrics,
	})
	RuntimeMetrics = envparser.Register(&envparser.Opts[bool]{
		Name:  "RUNTIME_METRICS",
		Desc:  "Whether to enable Go runtime metrics. This is only applicable if OBSERVABILITY is set to true.",
		Value: defaultMetrics,
	})

	Port = envparser.Register(&envparser.Opts[int]{
		Name:  "PORT",
		Desc:  "The port for the server to listen on.",
		Value: defaultPort,
		Validate: func(v int) error {
			if v <= 0 || v > 65535 {
				return fmt.Errorf("port must be between 1 and 65535, got %d", v)
			}
			return nil
		},
	})

	QueueType = envparser.Register(&envparser.Opts[string]{
		Name:  "Q_TYPE",
		Desc:  "The type of queue to use. Can be 'multi' or 'single'.",
		Value: defaultQueueType,
		Validate: func(v string) error {
			if !slices.Contains([]string{"multi", "single"}, v) {
				return fmt.Errorf("invalid queue type '%s', must be 'multi' or 'single'", v)
			}
			return nil
		},
	})
	QueueConcurrency = envparser.Register(&envparser.Opts[int]{
		Name:  "Q_CONCURRENCY",
		Desc:  "The concurrency level for the multi queue. This is only applicable if Q_TYPE is set to 'multi'.",
		Value: defaultQueueConcurrency,
		Validate: func(v int) error {
			if v <= 0 {
				return fmt.Errorf("queue concurrency must be greater than 0, got %d", v)
			}
			return nil
		},
	})
	QueueCapacity = envparser.Register(&envparser.Opts[int]{
		Name:  "Q_CAPACITY",
		Desc:  "The capacity for the queue. This is only applicable if Q_TYPE is set to 'multi'.",
		Value: defaultQueueCapacity,
		Validate: func(v int) error {
			if v <= 0 {
				return fmt.Errorf("queue capacity must be greater than 0, got %d", v)
			}
			return nil
		},
	})

	TLS = envparser.Register(&envparser.Opts[bool]{
		Name:  "TLS",
		Desc:  "Whether to enable TLS for the server. If true, TLS_CERT_PATH and TLS_KEY_PATH must be set to valid file paths for the TLS certificate and key, respectively.",
		Value: defaultTLS,
	})
	TLSCertPath = envparser.Register(&envparser.Opts[string]{
		Name:     "TLS_CERT_PATH",
		Desc:     "The file path for the TLS certificate. This is only applicable if TLS is set to true.",
		Value:    defaultTLSCertParth,
		Validate: validateFilePath,
	})
	TLSKeyPath = envparser.Register(&envparser.Opts[string]{
		Name:     "TLS_KEY_PATH",
		Desc:     "The file path for the TLS key. This is only applicable if TLS is set to true.",
		Value:    defaultTLSKeyPath,
		Validate: validateFilePath,
	})
	TLSRequireClientCert = envparser.Register(&envparser.Opts[bool]{
		Name:  "TLS_REQUIRE_CLIENT_CERT",
		Desc:  "Whether to require client certificates for TLS connections. If true, TLS_CLIENT_CA_CERT_PATH must be set.",
		Value: defaultTLSRequireClientCert,
	})
	TLSClientCACertPath = envparser.Register(&envparser.Opts[string]{
		Name:     "TLS_CLIENT_CA_CERT_PATH",
		Desc:     "The file path for the CA certificate used to verify client certificates. This is only applicable if TLS is set to true and TLS_REQUIRE_CLIENT_CERT is set to true.",
		Value:    defaultTLSClientCACertPath,
		Validate: validateFilePath,
	})
)

//nolint:gochecknoinits // intended
func init() {
	envparser.ExitOnError = false  //nolint:reassign // intended
	envparser.Prefix = "LOCKSMITH" //nolint:reassign // intended
}

func Parse() error {
	return envparser.Parse()
}

func validateFilePath(path string) error {
	if path == "" {
		return errors.New("file path cannot be empty")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist at path '%s'", path)
	}

	return nil
}
