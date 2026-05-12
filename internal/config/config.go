package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the application configuration consumed by every long-lived
// subsystem. The shape mirrors the canonical config.example.yaml in
// `config/config.example.yaml`. Fields with `mapstructure` tags are populated
// by viper; secret-bearing fields (DB.Password, ChirpStack.APIToken,
// MQTT.Password, Session.Key) are resolved at Load() time via ReadSecret /
// ReadSecretOrEmpty per D-06 and are NOT bound to viper keys (so the YAML
// schema never accidentally exposes raw secrets).
//
// Public surface frozen by Plan 05 (consumers in internal/cli/{migrate,
// serve,configcheck}.go): Load, Config (Env, HTTPPort, LogLevel, DB, TLS),
// DBConfig.DSN, IsDev. Plan 04 only ADDS fields; it never renames or removes.
type Config struct {
	Env            string `mapstructure:"env"`
	HTTPPort       string `mapstructure:"http_port"`
	LogLevel       string `mapstructure:"log_level"`
	LogoStorageDir string `mapstructure:"logo_storage_dir"`
	ReportsRoot    string `mapstructure:"reports_root"`

	DB         DBConfig      `mapstructure:"db"`
	ChirpStack CSConfig      `mapstructure:"chirpstack"`
	MQTT       MQTTConfig    `mapstructure:"mqtt"`
	Session    SessionConfig `mapstructure:"session"`
	TLS        TLSConfig     `mapstructure:"tls"`
}

// DBConfig holds Postgres connection inputs. Password is resolved at Load()
// time from SHIFTER_DB_PASSWORD or SHIFTER_DB_PASSWORD_FILE (D-06).
type DBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string // resolved at load time — NOT mapped from YAML
	SSLMode  string `mapstructure:"ssl_mode"`
	MaxConns int32  `mapstructure:"max_conns"`
}

// DSN returns the libpq-style connection string consumed by pgxpool.ParseConfig.
// Defaults applied here mirror Load()'s viper defaults so a zero-value DBConfig
// (used by tests) still yields a usable string.
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
	GRPCURL  string `mapstructure:"grpc_url"`
	APIToken string // resolved at load time
	Insecure bool   `mapstructure:"insecure"`
}

// MQTTConfig — MQTT broker for the ChirpStack uplink subscriber.
type MQTTConfig struct {
	URL      string `mapstructure:"url"`
	User     string `mapstructure:"user"`
	Password string // resolved at load time
}

// SessionConfig — alexedwards/scs cookie/session manager inputs (D-23).
type SessionConfig struct {
	Key         string        // resolved at load time (>=32 bytes per OWASP)
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`
	Lifetime    time.Duration `mapstructure:"lifetime"`
}

// TLSConfig — D-21 / D-22.
type TLSConfig struct {
	Mode   string `mapstructure:"mode"` // acme | byo | internal — "none" REJECTED (D-22)
	Domain string `mapstructure:"domain"`
	Email  string `mapstructure:"email"`
}

// IsDev reports whether the binary is running in dev mode (D-23: insecure
// cookies allowed when Env == "dev").
func (c *Config) IsDev() bool { return c.Env == "dev" }

// Load builds a *Config from the layered configuration sources defined in
// D-05: viper reads `config.yaml` (default `/etc/shifter/config.yaml`,
// override via `SHIFTER_CONFIG_FILE`), then `SHIFTER_*` environment variables
// override any YAML key (dotted keys → underscore-joined env, e.g.
// `db.host` → `SHIFTER_DB_HOST`). Secret-bearing fields are resolved
// separately via ReadSecret / ReadSecretOrEmpty (D-06) so they are never
// stored in YAML. Validate() then enforces D-22 + structural invariants.
//
// A missing config file is tolerated when all required fields can be
// satisfied by env defaults + secrets; the function still errors if the file
// path is set but unreadable for reasons other than non-existence.
func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("SHIFTER")
	v.AutomaticEnv()
	// Map dotted keys (db.host) → SHIFTER_DB_HOST env var.
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Defaults (also documented in config/config.example.yaml).
	v.SetDefault("env", "production")
	v.SetDefault("http_port", "8080")
	v.SetDefault("log_level", "info")
	v.SetDefault("logo_storage_dir", "/var/lib/shifter/logos")
	v.SetDefault("reports_root", "/var/lib/shifter/reports")
	v.SetDefault("db.host", "postgres")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.database", "shifter")
	v.SetDefault("db.user", "shifter")
	v.SetDefault("db.ssl_mode", "prefer")
	v.SetDefault("db.max_conns", 10)
	v.SetDefault("chirpstack.grpc_url", "chirpstack:8080")
	v.SetDefault("chirpstack.insecure", false)
	v.SetDefault("mqtt.url", "tcp://mosquitto:1883")
	v.SetDefault("mqtt.user", "")
	v.SetDefault("session.idle_timeout", "8h")
	v.SetDefault("session.lifetime", "24h")
	v.SetDefault("tls.mode", "acme")
	v.SetDefault("tls.domain", "")
	v.SetDefault("tls.email", "")

	configPath := os.Getenv("SHIFTER_CONFIG_FILE")
	if configPath == "" {
		configPath = "/etc/shifter/config.yaml"
	}
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		// Missing file is OK — env vars + defaults can still produce a
		// valid Config (CI / dev paths). Surface every other read error.
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config %q: %w", configPath, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Resolve secrets per D-06. Optional secrets default to empty.
	var err error
	if cfg.DB.Password, err = ReadSecretOrEmpty("SHIFTER_DB_PASSWORD"); err != nil {
		return nil, fmt.Errorf("db password: %w", err)
	}
	if cfg.ChirpStack.APIToken, err = ReadSecretOrEmpty("SHIFTER_CHIRPSTACK_API_TOKEN"); err != nil {
		return nil, fmt.Errorf("chirpstack api token: %w", err)
	}
	if cfg.MQTT.Password, err = ReadSecretOrEmpty("SHIFTER_MQTT_PASSWORD"); err != nil {
		return nil, fmt.Errorf("mqtt password: %w", err)
	}
	// Session key is REQUIRED — refuse to start without it (T-04-03).
	if cfg.Session.Key, err = ReadSecret("SHIFTER_SESSION_KEY"); err != nil {
		return nil, fmt.Errorf("session key: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate enforces structural invariants and security defaults: D-22 (no
// plain-HTTP / `tls.mode: none` in production), D-25 (log_level whitelist),
// session key minimum length (T-04-03 / OWASP ASVS V6).
func (c *Config) Validate() error {
	switch c.TLS.Mode {
	case "acme", "byo", "internal":
		// ok
	case "none":
		return fmt.Errorf("config: tls.mode=none is not allowed (D-22)")
	case "":
		return fmt.Errorf("config: tls.mode is required (acme | byo | internal)")
	default:
		return fmt.Errorf("config: tls.mode=%q invalid (acme | byo | internal)", c.TLS.Mode)
	}

	if c.TLS.Mode == "acme" && c.TLS.Domain == "" {
		return fmt.Errorf("config: tls.mode=acme requires tls.domain")
	}

	if c.LogLevel != "info" && c.LogLevel != "debug" {
		return fmt.Errorf("config: log_level=%q invalid (info | debug)", c.LogLevel)
	}

	if c.Env != "production" && c.Env != "dev" {
		return fmt.Errorf("config: env=%q invalid (production | dev)", c.Env)
	}

	if len(c.Session.Key) < 32 {
		return fmt.Errorf("config: session key must be at least 32 bytes (got %d)", len(c.Session.Key))
	}

	if c.Session.IdleTimeout <= 0 {
		return fmt.Errorf("config: session.idle_timeout must be positive")
	}
	if c.Session.Lifetime <= 0 {
		return fmt.Errorf("config: session.lifetime must be positive")
	}
	return nil
}
