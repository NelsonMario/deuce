// Package config loads typed application configuration from environment
// variables. No secrets are hardcoded.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv            string
	Port              string
	LogLevel          string
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	JoinCodeLength    int
	PlayerTokenSecret string
	CORSOrigins       []string
	// RedisHost is optional. When unset, device-identity linking (v2) is
	// disabled and the app behaves exactly as v1 (every join creates a new
	// player identity).
	RedisHost     string
	RedisPort     string
	RedisUsername string
	RedisPassword string
	RedisTLS      bool
}

func Load() (*Config, error) {
	cfg := &Config{
		AppEnv:            getEnv("APP_ENV", "development"),
		Port:              getEnv("PORT", "8080"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "5432"),
		DBUser:            os.Getenv("DB_USER"),
		DBPassword:        os.Getenv("DB_PASSWORD"),
		DBName:            os.Getenv("DB_NAME"),
		DBSSLMode:         getEnv("DB_SSLMODE", "disable"),
		PlayerTokenSecret: os.Getenv("PLAYER_TOKEN_SECRET"),
		RedisHost:         os.Getenv("REDIS_HOST"),
		RedisPort:         getEnv("REDIS_PORT", "6379"),
		RedisUsername:     getEnv("REDIS_USERNAME", "default"),
		RedisPassword:     os.Getenv("REDIS_PASSWORD"),
	}

	redisTLS, err := strconv.ParseBool(getEnv("REDIS_TLS", "false"))
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_TLS: %w", err)
	}
	cfg.RedisTLS = redisTLS

	joinCodeLength, err := strconv.Atoi(getEnv("JOIN_CODE_LENGTH", "6"))
	if err != nil {
		return nil, fmt.Errorf("invalid JOIN_CODE_LENGTH: %w", err)
	}
	cfg.JoinCodeLength = joinCodeLength

	origins := getEnv("CORS_ORIGINS", "http://localhost:5173")
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, o)
		}
	}

	if cfg.DBUser == "" {
		return nil, fmt.Errorf("DB_USER is required")
	}
	if cfg.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD is required")
	}
	if cfg.DBName == "" {
		return nil, fmt.Errorf("DB_NAME is required")
	}
	if cfg.PlayerTokenSecret == "" {
		return nil, fmt.Errorf("PLAYER_TOKEN_SECRET is required")
	}
	if len(cfg.PlayerTokenSecret) < 16 {
		return nil, fmt.Errorf("PLAYER_TOKEN_SECRET must be at least 16 characters")
	}

	return cfg, nil
}

// DatabaseURL builds the Postgres DSN from the individual DB_* fields,
// escaping user/password so special characters (e.g. "@", ":") can't break
// the connection string.
func (c *Config) DatabaseURL() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     fmt.Sprintf("%s:%s", c.DBHost, c.DBPort),
		Path:     "/" + c.DBName,
		RawQuery: "sslmode=" + url.QueryEscape(c.DBSSLMode),
	}
	return u.String()
}

// RedisURL builds a Redis connection URL from the individual REDIS_* fields.
// It returns "" when REDIS_HOST isn't set, signaling that device-identity
// linking (v2) should be disabled.
func (c *Config) RedisURL() string {
	if c.RedisHost == "" {
		return ""
	}
	scheme := "redis"
	if c.RedisTLS {
		scheme = "rediss"
	}
	u := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%s", c.RedisHost, c.RedisPort),
	}
	if c.RedisPassword != "" {
		u.User = url.UserPassword(c.RedisUsername, c.RedisPassword)
	}
	return u.String()
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
