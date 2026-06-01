package config

import (
	"strings"
	"testing"
)

func TestValidateRejectsProductionPlaceholderSecrets(t *testing.T) {
	cfg := Config{
		Environment: "production",
		JWTSecret:   "change-me",
		HMACSecret:  "replace-with-different-long-random-string",
		StaffAccounts: []StaffAccount{{
			Username: "admin",
			Password: "safe-admin-password-value",
			Role:     "admin",
		}},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("Validate() error = %v, want JWT_SECRET production failure", err)
	}
}

func TestValidateRejectsSharedTokenSecrets(t *testing.T) {
	cfg := Config{
		Environment: "development",
		JWTSecret:   "same-secret-value-that-is-long",
		HMACSecret:  "same-secret-value-that-is-long",
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "must be different") {
		t.Fatalf("Validate() error = %v, want shared secret failure", err)
	}
}

func TestValidateAllowsDevelopmentDefaults(t *testing.T) {
	cfg := Config{
		Environment: "development",
		JWTSecret:   "change-me",
		HMACSecret:  "change-me-hmac",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAllowsProductionStrongSecrets(t *testing.T) {
	cfg := Config{
		Environment: "production",
		JWTSecret:   "jwt-secret-with-at-least-24-characters",
		HMACSecret:  "hmac-secret-with-at-least-24-characters",
		StaffAccounts: []StaffAccount{{
			Username: "admin",
			Password: "admin-password-with-at-least-24",
			Role:     "admin",
		}},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
