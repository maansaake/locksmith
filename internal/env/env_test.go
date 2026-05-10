package env_test

import (
	"errors"
	"os"
	"testing"

	"github.com/maansaake/locksmith/internal/env"
	"github.com/trebent/envparser"
)

func TestParse_NoEnvVars(t *testing.T) {
	if err := env.Parse(); err != nil {
		t.Fatalf("expected no error when no env vars are set, got: %v", err)
	}
}

func TestParse_InvalidEnvVar(t *testing.T) {
	t.Setenv("LOCKSMITH_LOG_VERBOSITY", "-1")

	err := env.Parse()
	if err == nil {
		t.Fatal("expected an error when LOG_VERBOSITY is negative, got nil")
	}

	if !errors.Is(err, envparser.ErrValidate) {
		t.Errorf("expected error to wrap envparser.ErrValidate, got: %v", err)
	}
}

func TestParse_ValidEnvVars(t *testing.T) {
	t.Setenv("LOCKSMITH_PORT", "8080")
	t.Setenv("LOCKSMITH_LOG_VERBOSITY", "5")

	if err := env.Parse(); err != nil {
		t.Fatalf("expected no error with valid env vars, got: %v", err)
	}

	if got := env.Port.Value(); got != 8080 {
		t.Errorf("expected Port to be 8080, got %d", got)
	}

	if got := env.LogVerbosity.Value(); got != 5 {
		t.Errorf("expected LogVerbosity to be 5, got %d", got)
	}
}

func TestParse_TLSCertPath_NonExistentFile(t *testing.T) {
	t.Setenv("LOCKSMITH_TLS_CERT_PATH", "/nonexistent/path/cert.pem")

	err := env.Parse()
	if err == nil {
		t.Fatal("expected an error when TLS_CERT_PATH points to a non-existent file, got nil")
	}

	if !errors.Is(err, envparser.ErrValidate) {
		t.Errorf("expected error to wrap envparser.ErrValidate, got: %v", err)
	}
}

func TestParse_TLSCertPath_ValidFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "cert-*.pem")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	_ = f.Close()

	t.Setenv("LOCKSMITH_TLS_CERT_PATH", f.Name())

	if err := env.Parse(); err != nil {
		t.Fatalf("expected no error when TLS_CERT_PATH points to an existing file, got: %v", err)
	}

	if got := env.TLSCertPath.Value(); got != f.Name() {
		t.Errorf("expected TLSCertPath to be %q, got %q", f.Name(), got)
	}
}
