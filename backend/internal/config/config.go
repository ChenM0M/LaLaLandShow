package config

import (
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
	HTTPAddr      string
	DatabaseURL   string
	RedisURL      string
	MigrationsDir string
	RunMigrations bool
	SeedSeats     bool
	JWTSecret     string
	HMACSecret    string
	JWTTTL        time.Duration
	RateLimit     RateLimitConfig
	StaffAccounts []StaffAccount
}

type RateLimitConfig struct {
	Enabled    bool
	Window     time.Duration
	GenericMax int
	BookingMax int
}

func Load() Config {
	return Config{
		HTTPAddr:      getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisURL:      os.Getenv("REDIS_URL"),
		MigrationsDir: getEnv("MIGRATIONS_DIR", "backend/migrations"),
		RunMigrations: getBoolEnv("RUN_MIGRATIONS", true),
		SeedSeats:     getBoolEnv("SEED_SEATS", true),
		JWTSecret:     getEnv("JWT_SECRET", "change-me"),
		HMACSecret:    getEnv("HMAC_SECRET", "change-me"),
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
