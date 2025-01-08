// Package env provides some rudimentary environment variable parsing.
package env

import (
	"fmt"
	"os"
	"strconv"
)

const (
	LOCKSMITH_LOG_LEVEL                  string = "LOCKSMITH_LOG_LEVEL"
	LOCKSMITH_LOG_LEVEL_DEFAULT          string = "WARNING"
	LOCKSMITH_LOG_OUTPUT                 string = "LOCKSMITH_LOG_OUTPUT"
	LOCKSMITH_LOG_OUTPUT_DEFAULT         string = "stderr"
	LOCKSMITH_LOG_OUTPUT_CONSOLE         string = "LOCKSMITH_LOG_OUTPUT_CONSOLE"
	LOCKSMITH_LOG_OUTPUT_CONSOLE_DEFAULT bool   = false
)

const (
	LOCKSMITH_METRICS         string = "LOCKSMITH_METRICS"
	LOCKSMITH_METRICS_DEFAULT bool   = false
)

const (
	LOCKSMITH_PORT         string = "LOCKSMITH_PORT"
	LOCKSMITH_PORT_DEFAULT uint16 = 9000
)

const (
	LOCKSMITH_Q_TYPE                string = "LOCKSMITH_Q_TYPE"
	LOCKSMITH_Q_TYPE_DEFAULT        string = "multi"
	LOCKSMITH_Q_CONCURRENCY         string = "LOCKSMITH_Q_CONCURRENCY"
	LOCKSMITH_Q_CONCURRENCY_DEFAULT int    = 10
	LOCKSMITH_Q_CAPACITY            string = "LOCKSMITH_Q_CAPACITY"
	LOCKSMITH_Q_CAPACITY_DEFAULT    int    = 100
)

const (
	LOCKSMITH_TLS                             string = "LOCKSMITH_TLS"
	LOCKSMITH_TLS_DEFAULT                     bool   = false
	LOCKSMITH_TLS_CERT_PATH                   string = "LOCKSMITH_TLS_CERT_PATH"
	LOCKSMITH_TLS_CERT_PATH_DEFAULT           string = "/etc/cert/locksmith.pem"
	LOCKSMITH_TLS_KEY_PATH                    string = "LOCKSMITH_TLS_KEY_PATH"
	LOCKSMITH_TLS_KEY_PATH_DEFAULT            string = "/etc/cert/locksmith.key"
	LOCKSMITH_TLS_REQUIRE_CLIENT_CERT         string = "LOCKSMITH_TLS_REQUIRE_CLIENT_CERT"
	LOCKSMITH_TLS_REQUIRE_CLIENT_CERT_DEFAULT bool   = false
	LOCKSMITH_TLS_CLIENT_CA_CERT_PATH         string = "LOCKSMITH_TLS_CLIENT_CA_CERT_PATH"
	LOCKSMITH_TLS_CLIENT_CA_CERT_PATH_DEFAULT string = "/etc/cert/client_ca.cert"
)

type NotFoundError struct {
	name string
}

func newErrorNotFound(name string) error {
	return &NotFoundError{name: name}
}

func (err *NotFoundError) Error() string {
	return fmt.Sprintf("Did not find variable '%s'", err.name)
}

func GetOptionalBool(name string, def bool) (bool, error) {
	if v, e := os.LookupEnv(name); e {
		return strconv.ParseBool(v)
	}
	return def, nil
}

func GetRequiredBool(name string) (bool, error) {
	if v, e := os.LookupEnv(name); e {
		return strconv.ParseBool(v)
	}
	return false, newErrorNotFound(name)
}

func GetOptionalString(name string, def string) (string, error) {
	if v, e := os.LookupEnv(name); e {
		return v, nil
	}
	return def, nil
}

func GetRequiredString(name string) (string, error) {
	if v, e := os.LookupEnv(name); e {
		return v, nil
	}
	return "", newErrorNotFound(name)
}

func GetOptionalInteger(name string, def int) (int, error) {
	if v, e := os.LookupEnv(name); e {
		// by setting a base of 0, the base is implied by the string's format
		i64, err := strconv.ParseInt(v, 0, 0)
		return int(i64), err
	}
	return def, nil
}

func GetRequiredInteger(name string) (int, error) {
	if v, e := os.LookupEnv(name); e {
		// by setting a base of 0, the base is implied by the string's format
		i64, err := strconv.ParseInt(v, 0, 0)
		return int(i64), err
	}
	return 0, newErrorNotFound(name)
}

func GetOptionalUint16(name string, def uint16) (uint16, error) {
	if v, e := os.LookupEnv(name); e {
		// by setting a base of 0, the base is implied by the string's format
		i64, err := strconv.ParseUint(v, 0, 16)
		return uint16(i64), err
	}
	return def, nil
}
