package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the application configuration consumed by every long-lived
// subsystem. The shape mirrors the canonical config.example.yaml documented in
// Plan 04. Plan 05 only reads .Env, .HTTPPort, .LogLevel, .DB, and .TLS.Mode;
// later plans (07/08/09/12/13/14/15/17/18) read the rest.
type Config struct {
	Env            string
	HTTPPort       string
	LogLevel       string
	LogoStorageDir string

	DB         DBConfig
	ChirpStack CSConfig
	MQTT       MQTTConfig
	Session    SessionConfig
	TLS        TLSConfig
}

// DBConfig holds Postgres connection inputs. Password is resolved at Load()
// time from SHIFTER_DB_PASSWORD or SHIFTER_DB_PASSWORD_FILE (D-06).
type DBConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
	MaxConns int32
}

// DSN returns the libpq-style connection string consumed by pgxpool.ParseConfig.
func (d DBConfig) DSN() string {
	sslmode := d.SSLMode
	if sslmode == "" {
		sslmode = "prefer"
	}
	port := d.Port
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, port, d.Database, sslmode)
}

// CSConfig — ChirpStack gRPC connection.
type CSConfig struct {
	GRPCURL  string
	APIToken string
	Insecure bool
}

// MQTTConfig — MQTT broker for the ChirpStack uplink subscriber.
type MQTTConfig struct {
	URL      string
	User     string
	Password string
}

// SessionConfig — alexedwards/scs cookie/session manager inputs.
type SessionConfig struct {
	Key         string
	IdleTimeout time.Duration
	Lifetime    time.Duration
}

// TLSConfig — D-21 / D-22.
type TLSConfig struct {
	Mode   string // acme | byo | internal — "none" is rejected
	Domain string
	Email  string
}

// IsDev reports whether the binary is running in dev mode (D-23: insecure
// cookies allowed when Env == "dev").
func (c *Config) IsDev() bool { return c.Env == "dev" }

// Load builds a *Config from environment variables. This is a temporary
// scaffold for Plan 05 — Plan 04 replaces it with a viper-driven YAML+env
// layered loader, secrets file reader, and Validate(). The minimal env-only
// path here lets `shifter version`, `shifter healthcheck`, and `shifter
// migrate` work today while preserving the public function signature later
// plans depend on.
//
// Recognized env vars (subset of Plan 04's full spec):
//   - SHIFTER_ENV (default "production")
//   - SHIFTER_HTTP_PORT (default "8080")
//   - SHIFTER_LOG_LEVEL (default "info")
//   - SHIFTER_DB_HOST / SHIFTER_DB_PORT / SHIFTER_DB_USER /
//     SHIFTER_DB_PASSWORD / SHIFTER_DB_NAME / SHIFTER_DB_SSL_MODE /
//     SHIFTER_DB_MAX_CONNS
//   - SHIFTER_TLS_MODE (default "internal" so dev paths don't error today;
//     Plan 04's Validate() will tighten this).
func Load() (*Config, error) {
	cfg := &Config{
		Env:      envOr("SHIFTER_ENV", "production"),
		HTTPPort: envOr("SHIFTER_HTTP_PORT", "8080"),
		LogLevel: envOr("SHIFTER_LOG_LEVEL", "info"),
		DB: DBConfig{
			Host:     envOr("SHIFTER_DB_HOST", "postgres"),
			Port:     envIntOr("SHIFTER_DB_PORT", 5432),
			Database: envOr("SHIFTER_DB_NAME", "shifter"),
			User:     envOr("SHIFTER_DB_USER", "shifter"),
			Password: os.Getenv("SHIFTER_DB_PASSWORD"),
			SSLMode:  envOr("SHIFTER_DB_SSL_MODE", "prefer"),
			MaxConns: int32(envIntOr("SHIFTER_DB_MAX_CONNS", 10)),
		},
		TLS: TLSConfig{
			Mode: envOr("SHIFTER_TLS_MODE", "internal"),
		},
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
