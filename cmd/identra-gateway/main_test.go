package main

import "testing"

func TestValidateCORSConfigRejectsWildcardWithCredentials(t *testing.T) {
	err := validateCORSConfig([]string{"https://app.example.com", "*"}, true)
	if err == nil {
		t.Fatal("expected wildcard origin with credentials to be rejected")
	}
}

func TestValidateCORSConfigAllowsWildcardWithoutCredentials(t *testing.T) {
	if err := validateCORSConfig([]string{"*"}, false); err != nil {
		t.Fatalf("expected wildcard without credentials to be valid: %v", err)
	}
}
