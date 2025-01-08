package env

import (
	"os"
	"testing"
)

func Test_RequiredAndOptionalVariables(t *testing.T) {
	t.Log("Getting mandatory variables that are missing")
	_, err := GetRequiredBool("DOES_NOT_EXIST_B")
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatal("Expected a not found error for the required boolean variable")
	}

	_, err = GetRequiredString("DOES_NOT_EXIST_S")
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatal("Expected a not found error for the required string variable")
	}

	_, err = GetRequiredInteger("DOES_NOT_EXIST_I")
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatal("Expected a not found error for the required integer variable")
	}

	t.Log("Setting some variables")
	os.Setenv("EXISTS_B", "true")
	os.Setenv("EXISTS_S", "string")
	os.Setenv("EXISTS_I", "100000")

	t.Log("Checking that set variables lead to found variables that can be decoded correctly")

	b, err := GetRequiredBool("EXISTS_B")
	if err != nil {
		t.Fatal("Expected boolean variable not found")
	}
	if !b {
		t.Fatal("Gotten boolean was not true")
	}

	s, err := GetRequiredString("EXISTS_S")
	if err != nil {
		t.Fatal("Expected string variable not found")
	}
	if s != "string" {
		t.Fatal("Gotten string did not match expected 'string'")
	}

	i, err := GetRequiredInteger("EXISTS_I")
	if err != nil {
		t.Fatal("Expected integer variable not found")
	}
	if !(i == 100000) {
		t.Fatalf("Integer value expected to be '100000' but was %d", i)
	}

	t.Log("Testing defaults for optional variables")
	b, _ = GetOptionalBool("OPTIONAL_B", true)
	if !b {
		t.Fatal("Boolean default was supposed to be 'true'")
	}

	s, _ = GetOptionalString("OPTIONAL_S", "not-found")
	if s != "not-found" {
		t.Fatal("Expected string to use default of 'not-found'")
	}

	i, _ = GetOptionalInteger("OPTIONAL_I", 8000)
	if !(i == 8000) {
		t.Fatalf("Expected integer to use default of '8000', but was %d", i)
	}

	t.Log("Testing optional variables with defaults but where values are present")
	os.Setenv("PRESENT_OPTIONAL_B", "false")
	os.Setenv("PRESENT_OPTIONAL_S", "optional-string")
	os.Setenv("PRESENT_OPTIONAL_I", "666")

	b, _ = GetOptionalBool("PRESENT_OPTIONAL_B", true)
	if b {
		t.Fatal("Expected optional boolean to be 'false'")
	}

	s, _ = GetOptionalString("PRESENT_OPTIONAL_S", "shound-not-use-default")
	if s == "optional-string" {
		// NOOP
	} else {
		t.Fatalf("Did not match string 'optional-string', was %s", s)
	}

	i, _ = GetOptionalInteger("PRESENT_OPTIONAL_I", 0)
	if i == 666 {
		// NOOP
	} else {
		t.Fatalf("Expected integer to be '666' but was %d", i)
	}
}
