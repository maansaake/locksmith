package env_test

import (
	"errors"
	"testing"

	"github.com/maansaake/locksmith/pkg/env"
)

func Test_RequiredAndOptionalVariables(t *testing.T) {
	t.Log("Getting mandatory variables that are missing")
	_, err := env.GetRequiredBool("DOES_NOT_EXIST_B")
	var notFoundErr *env.NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Fatal("Expected a not found error for the required boolean variable, got: ", err)
	}

	_, err = env.GetRequiredString("DOES_NOT_EXIST_S")
	if !errors.As(err, &notFoundErr) {
		t.Fatal("Expected a not found error for the required string variable, got: ", err)
	}

	_, err = env.GetRequiredInteger("DOES_NOT_EXIST_I")
	if !errors.As(err, &notFoundErr) {
		t.Fatal("Expected a not found error for the required integer variable, got: ", err)
	}

	t.Log("Setting some variables")
	t.Setenv("EXISTS_B", "true")
	t.Setenv("EXISTS_S", "string")
	t.Setenv("EXISTS_I", "100000")

	t.Log("Checking that set variables lead to found variables that can be decoded correctly")

	b, err := env.GetRequiredBool("EXISTS_B")
	if err != nil {
		t.Fatal("Expected boolean variable not found")
	}
	if !b {
		t.Fatal("Gotten boolean was not true")
	}

	s, err := env.GetRequiredString("EXISTS_S")
	if err != nil {
		t.Fatal("Expected string variable not found")
	}
	if s != "string" {
		t.Fatal("Gotten string did not match expected 'string'")
	}

	i, err := env.GetRequiredInteger("EXISTS_I")
	if err != nil {
		t.Fatal("Expected integer variable not found")
	}
	if i != 100000 {
		t.Fatalf("Integer value expected to be '100000' but was %d", i)
	}

	t.Log("Testing defaults for optional variables")
	b, _ = env.GetOptionalBool("OPTIONAL_B", true)
	if !b {
		t.Fatal("Boolean default was supposed to be 'true'")
	}

	s, _ = env.GetOptionalString("OPTIONAL_S", "not-found")
	if s != "not-found" {
		t.Fatal("Expected string to use default of 'not-found'")
	}

	i, _ = env.GetOptionalInteger("OPTIONAL_I", 8000)
	if i != 8000 {
		t.Fatalf("Expected integer to use default of '8000', but was %d", i)
	}

	t.Log("Testing optional variables with defaults but where values are present")
	t.Setenv("PRESENT_OPTIONAL_B", "false")
	t.Setenv("PRESENT_OPTIONAL_S", "optional-string")
	t.Setenv("PRESENT_OPTIONAL_I", "666")

	b, _ = env.GetOptionalBool("PRESENT_OPTIONAL_B", true)
	if b {
		t.Fatal("Expected optional boolean to be 'false'")
	}

	s, _ = env.GetOptionalString("PRESENT_OPTIONAL_S", "shound-not-use-default")
	if s != "optional-string" {
		t.Fatalf("Did not match string 'optional-string', was %s", s)
	}

	i, _ = env.GetOptionalInteger("PRESENT_OPTIONAL_I", 0)
	if i != 666 {
		t.Fatalf("Expected integer to be '666' but was %d", i)
	}
}
