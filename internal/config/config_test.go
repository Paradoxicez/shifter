package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeYAML writes content to a temp file and returns the path.
func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

// setupValidEnv points Load() at a config file and provides the secrets every
// valid Config requires (DB password + session key). Helper so individual tests
// only have to declare the values they care about.
func setupValidEnv(t *testing.T, configPath string) {
	t.Helper()
	t.Setenv("SHIFTER_CONFIG_FILE", configPath)
	t.Setenv("SHIFTER_DB_PASSWORD", "test-db-pw")
	t.Setenv("SHIFTER_SESSION_KEY", "this-is-at-least-thirty-two-bytes-long-key")
}

func TestLoad_DefaultsFromYAML(t *testing.T) {
	p := writeYAML(t, `
env: production
http_port: "9090"
log_level: info
db:
  host: localhost
  port: 5432
  database: shifter
  user: shifter
chirpstack:
  grpc_url: chirpstack:8080
mqtt:
  url: tcp://mosquitto:1883
session:
  idle_timeout: 8h
  lifetime: 24h
tls:
  mode: internal
`)
	setupValidEnv(t, p)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "9090", cfg.HTTPPort)
	require.Equal(t, "localhost", cfg.DB.Host)
	require.Equal(t, "internal", cfg.TLS.Mode)
}

func TestLoad_EnvOverride(t *testing.T) {
	p := writeYAML(t, `
log_level: info
tls:
  mode: internal
session:
  idle_timeout: 8h
  lifetime: 24h
`)
	setupValidEnv(t, p)
	t.Setenv("SHIFTER_LOG_LEVEL", "debug")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_ResolvesSecretsViaFile(t *testing.T) {
	// PITFALL #8 + D-06: secret comes from a file, not direct env.
	dir := t.TempDir()
	pwPath := filepath.Join(dir, "db_password")
	require.NoError(t, os.WriteFile(pwPath, []byte("file-resolved-pw\n"), 0o600))

	keyPath := filepath.Join(dir, "session_key")
	require.NoError(t, os.WriteFile(keyPath, []byte("this-is-at-least-thirty-two-bytes-long-key\r\n"), 0o600))

	p := writeYAML(t, `
log_level: info
tls:
  mode: internal
session:
  idle_timeout: 8h
  lifetime: 24h
`)
	t.Setenv("SHIFTER_CONFIG_FILE", p)
	t.Setenv("SHIFTER_DB_PASSWORD_FILE", pwPath)
	t.Setenv("SHIFTER_SESSION_KEY_FILE", keyPath)
	// Make sure direct env vars are NOT set so the file path is exercised.
	t.Setenv("SHIFTER_DB_PASSWORD", "")
	t.Setenv("SHIFTER_SESSION_KEY", "")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "file-resolved-pw", cfg.DB.Password, "must trim trailing newline")
	require.Equal(t, "this-is-at-least-thirty-two-bytes-long-key", cfg.Session.Key, "PITFALL #8: must trim CRLF")
}

func TestValidate_RejectsTLSNone(t *testing.T) {
	cfg := &Config{
		Env: "production", LogLevel: "info",
		Session: SessionConfig{Key: "this-is-at-least-thirty-two-bytes-long-key", IdleTimeout: 1, Lifetime: 1},
		TLS:     TLSConfig{Mode: "none"},
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "tls.mode")
}

func TestValidate_RequiresAcmeDomain(t *testing.T) {
	cfg := &Config{
		Env: "production", LogLevel: "info",
		Session: SessionConfig{Key: "this-is-at-least-thirty-two-bytes-long-key", IdleTimeout: 1, Lifetime: 1},
		TLS:     TLSConfig{Mode: "acme", Domain: ""},
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "tls.domain")
}

func TestValidate_AcceptsValidConfig(t *testing.T) {
	cfg := &Config{
		Env: "production", LogLevel: "info",
		Session: SessionConfig{Key: "this-is-at-least-thirty-two-bytes-long-key", IdleTimeout: 1, Lifetime: 1},
		TLS:     TLSConfig{Mode: "internal"},
	}
	require.NoError(t, cfg.Validate())
}

func TestValidate_RejectsShortSessionKey(t *testing.T) {
	cfg := &Config{
		Env: "production", LogLevel: "info",
		Session: SessionConfig{Key: "too-short", IdleTimeout: 1, Lifetime: 1},
		TLS:     TLSConfig{Mode: "internal"},
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "session key")
}

func TestValidate_RejectsBadLogLevel(t *testing.T) {
	cfg := &Config{
		Env: "production", LogLevel: "trace",
		Session: SessionConfig{Key: "this-is-at-least-thirty-two-bytes-long-key", IdleTimeout: 1, Lifetime: 1},
		TLS:     TLSConfig{Mode: "internal"},
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "log_level")
}

func TestIsDev(t *testing.T) {
	cfg := &Config{Env: "dev"}
	require.True(t, cfg.IsDev())
	cfg.Env = "production"
	require.False(t, cfg.IsDev())
}

func TestDBConfig_DSN(t *testing.T) {
	d := DBConfig{Host: "h", Port: 5432, Database: "n", User: "u", Password: "p", SSLMode: "require"}
	require.Equal(t, "postgres://u:p@h:5432/n?sslmode=require", d.DSN())
}
