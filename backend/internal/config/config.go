package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type StaffAccount struct {
	Username string
	Password string
	Role     string
}

type Config struct {
	Environment   string
	HTTPAddr      string
	DatabaseURL   string
	RedisURL      string
	Database      DatabaseConfig
	MigrationsDir string
	StaticDir     string
	RunMigrations bool
	SeedSeats     bool
	JWTSecret     string
	HMACSecret    string
	JWTTTL        time.Duration
	RateLimit     RateLimitConfig
	StaffAccounts []StaffAccount
}

type DatabaseConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type RateLimitConfig struct {
	Enabled    bool
	Window     time.Duration
	GenericMax int
	BookingMax int
}

func Load() Config {
	dbMaxConns := getIntEnv("DB_MAX_CONNS", 12)
	dbMinConns := getIntEnv("DB_MIN_CONNS", 1)
	if dbMinConns > dbMaxConns {
		dbMinConns = dbMaxConns
	}

	return Config{
		Environment: strings.ToLower(strings.TrimSpace(getEnv("APP_ENV", "development"))),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		Database: DatabaseConfig{
			MaxConns:        int32(dbMaxConns),
			MinConns:        int32(dbMinConns),
			MaxConnLifetime: getDurationEnv("DB_MAX_CONN_LIFETIME", time.Hour),
			MaxConnIdleTime: getDurationEnv("DB_MAX_CONN_IDLE_TIME", 5*time.Minute),
		},
		MigrationsDir: getEnv("MIGRATIONS_DIR", "backend/migrations"),
		StaticDir:     os.Getenv("STATIC_DIR"),
		RunMigrations: getBoolEnv("RUN_MIGRATIONS", true),
		SeedSeats:     getBoolEnv("SEED_SEATS", true),
		JWTSecret:     getEnv("JWT_SECRET", "change-me"),
		HMACSecret:    getEnv("HMAC_SECRET", "change-me-hmac"),
		JWTTTL:        getDurationEnv("JWT_TTL", 12*time.Hour),
		RateLimit: RateLimitConfig{
			Enabled:    getBoolEnv("RATE_LIMIT_ENABLED", true),
			Window:     getDurationEnv("RATE_LIMIT_WINDOW", time.Minute),
			GenericMax: getIntEnv("RATE_LIMIT_GENERIC_MAX", 300),
			BookingMax: getIntEnv("RATE_LIMIT_BOOKING_MAX", 1200),
		},
		StaffAccounts: loadStaffAccounts(),
	}
}

func (c Config) Validate() error {
	if c.Environment == "" {
		return errors.New("APP_ENV cannot be empty")
	}
	if c.JWTSecret == "" {
		return errors.New("JWT_SECRET cannot be empty")
	}
	if c.HMACSecret == "" {
		return errors.New("HMAC_SECRET cannot be empty")
	}
	if c.JWTSecret == c.HMACSecret {
		return errors.New("JWT_SECRET and HMAC_SECRET must be different")
	}
	if c.Environment == "production" {
		if isUnsafeSecret(c.JWTSecret) {
			return errors.New("JWT_SECRET must be replaced for production")
		}
		if isUnsafeSecret(c.HMACSecret) {
			return errors.New("HMAC_SECRET must be replaced for production")
		}
		for _, account := range c.StaffAccounts {
			if isUnsafeSecret(account.Password) {
				return fmt.Errorf("%s password must be replaced for production", account.Username)
			}
		}
	}
	return nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getBoolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getIntEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func loadStaffAccounts() []StaffAccount {
	accounts := []StaffAccount{}

	adminUsers := splitCSV(getEnv("ADMIN_USERS", "admin"))
	adminPasswords := splitCSV(getEnv("ADMIN_PASSWORDS", "change-me-admin"))
	for i, username := range adminUsers {
		password := ""
		if i < len(adminPasswords) {
			password = adminPasswords[i]
		}
		if username != "" && password != "" {
			accounts = append(accounts, StaffAccount{
				Username: username,
				Password: password,
				Role:     "admin",
			})
		}
	}

	checkerUser := strings.TrimSpace(getEnv("CHECKER_USER", "checker"))
	checkerPassword := strings.TrimSpace(getEnv("CHECKER_PASSWORD", "change-me-checker"))
	if checkerUser != "" && checkerPassword != "" {
		accounts = append(accounts, StaffAccount{
			Username: checkerUser,
			Password: checkerPassword,
			Role:     "checker",
		})
	}

	return accounts
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func isUnsafeSecret(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 24 {
		return true
	}
	return strings.Contains(strings.ToLower(value), "change-me") ||
		strings.Contains(strings.ToLower(value), "replace-with")
}
