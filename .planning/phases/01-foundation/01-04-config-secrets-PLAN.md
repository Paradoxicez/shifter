---
phase: 01-foundation
plan: 04
type: execute
wave: 4
depends_on: [01, 02]
files_modified:
  - go.mod
  - go.sum
  - internal/config/config.go
  - internal/config/secrets.go
  - internal/config/config_test.go
  - internal/config/secrets_test.go
  - internal/logging/logging.go
  - internal/logging/logging_test.go
  - internal/version/version.go
  - config/config.example.yaml
autonomous: true
requirements: []
must_haves:
  truths:
    - "Loading the example config.yaml + setting SHIFTER_DB_PASSWORD_FILE produces a valid Config struct with all secrets resolved"
    - "Setting `SHIFTER_LOG_LEVEL=debug` overrides config.yaml level (D-25)"
    - "Reading a CRLF-formatted secret file strips trailing \\r\\n (PITFALL #8 prevention)"
    - "config.tls.mode=none is REJECTED by Validate() (D-22)"
    - "Logging produces newline-delimited JSON (PITFALL #9 prevention)"
  artifacts:
    - path: "internal/config/config.go"
      provides: "Config struct + Load() (viper YAML+env layering per D-05) + Validate()"
      contains: "type Config struct"
    - path: "internal/config/secrets.go"
      provides: "ReadSecret(name) reads env var or env_var_FILE per D-06 + RESEARCH §Pattern 12"
      contains: "func ReadSecret"
    - path: "internal/logging/logging.go"
      provides: "slog JSON handler bootstrap with one-line format (D-24, D-25)"
      contains: "slog.NewJSONHandler"
    - path: "internal/version/version.go"
      provides: "Version() returns ldflags-injected build info"
      contains: "Version"
    - path: "config/config.example.yaml"
      provides: "Documented config keys; install kit copies this if /etc/shifter/config.yaml is missing"
      contains: "tls:"
  key_links:
    - from: "config.yaml"
      to: "Go Config struct"
      via: "viper unmarshal with SHIFTER_ prefix env override"
      pattern: "viper"
    - from: "Config.DB.PasswordRef"
      to: "/run/secrets/postgres_password"
      via: "secrets.ReadSecret(\"SHIFTER_DB_PASSWORD\") reads SHIFTER_DB_PASSWORD_FILE"
      pattern: "_FILE"
---

<objective>
Implement the config and secrets layer that every later plan reads from: viper-based YAML+env layering (D-05), file-mounted-secret reader compatible with Compose `secrets:` (D-06), structured JSON logging via stdlib slog (D-24, D-25), build-version package wired by ldflags, and the canonical `config/config.example.yaml` that the install kit copies to `/etc/shifter/config.yaml`.

Purpose: Plans 03/05/07/08/09/12/13/14/15/17/18 all consume the Config struct. Without it, no plan can load DSN, DB pool size, ChirpStack endpoints, session signing key, or TLS mode.

Output: `config.Load()` returns a populated Config, secrets resolved, logging configured. `config-check` (Plan 05) is a thin wrapper.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@01-02-test-harness-PLAN.md

<interfaces>
RESEARCH §Pattern 12 (lines 980-993) — verbatim ReadSecret pattern:
```go
func ReadSecret(name string) (string, error) {
    if v := os.Getenv(name); v != "" { return v, nil }
    if path := os.Getenv(name + "_FILE"); path != "" {
        b, err := os.ReadFile(path)
        if err != nil { return "", fmt.Errorf("read %s_FILE: %w", name, err) }
        return strings.TrimRight(string(b), "\r\n"), nil  // PITFALL #8: handle CRLF
    }
    return "", fmt.Errorf("secret %s not set", name)
}
```

Config.yaml example shape (the full canonical schema — Claude's discretion per D's "config.yaml exact YAML shape"):
```yaml
env: production              # production | dev (D-23: dev disables Cookie.Secure)
http_port: 8080
log_level: info              # info | debug (D-25)

db:
  host: postgres
  port: 5432
  database: shifter
  user: shifter
  # password resolved via SHIFTER_DB_PASSWORD or SHIFTER_DB_PASSWORD_FILE (D-06)
  ssl_mode: prefer           # disable | prefer | require
  max_conns: 10

chirpstack:
  grpc_url: chirpstack:8080
  # api_token resolved via SHIFTER_CHIRPSTACK_API_TOKEN_FILE (D-06)
  insecure: false             # true = no TLS to gRPC (only for in-cluster)

mqtt:
  url: tcp://mosquitto:1883
  user: ""
  # password resolved via SHIFTER_MQTT_PASSWORD_FILE if set (D-06)

session:
  # key resolved via SHIFTER_SESSION_KEY_FILE (D-06) — required
  idle_timeout: 8h
  lifetime: 24h

tls:
  mode: acme                  # acme | byo | internal (D-21); "none" rejected (D-22)
  domain: ""                  # required when mode=acme
  email: ""                   # ACME contact

logo_storage_dir: /var/lib/shifter/logos
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Config struct + viper Load + Validate + secrets.ReadSecret + config.example.yaml</name>
  <files>go.mod, go.sum, internal/config/config.go, internal/config/secrets.go, internal/config/config_test.go, internal/config/secrets_test.go, config/config.example.yaml</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (D-05, D-06, D-07, D-21, D-22, D-23)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 12: Compose secrets idiom" (lines 925-994)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 8: CRLF on Windows" (line 1356)
  </read_first>
  <behavior>
    - TestLoad_DefaultsFromYAML: writes config.example.yaml to a temp dir, points SHIFTER_CONFIG_FILE at it, asserts loaded values match the YAML.
    - TestLoad_EnvOverride: sets SHIFTER_LOG_LEVEL=debug, asserts cfg.LogLevel == "debug" overriding YAML.
    - TestValidate_RejectsTLSNone: cfg.TLS.Mode = "none", Validate() returns error containing "tls.mode".
    - TestValidate_RequiresAcmeDomain: TLS.Mode=acme with empty Domain returns error.
    - TestReadSecret_DirectEnv: SHIFTER_X="value", returns "value".
    - TestReadSecret_FileEnv: SHIFTER_X_FILE points at a file containing "value\n", returns "value".
    - TestReadSecret_CRLF: file contents = "value\r\n", returns "value" (PITFALL #8).
    - TestReadSecret_Missing: neither set, returns error.
  </behavior>
  <action>
1. Install viper:
   ```bash
   go get github.com/spf13/viper@latest
   ```

2. Create `internal/config/config.go`:
   ```go
   package config

   import (
       "fmt"
       "os"
       "strings"
       "time"

       "github.com/spf13/viper"
   )

   type Config struct {
       Env             string        `mapstructure:"env"`
       HTTPPort        string        `mapstructure:"http_port"`
       LogLevel        string        `mapstructure:"log_level"`
       LogoStorageDir  string        `mapstructure:"logo_storage_dir"`
       DB              DBConfig      `mapstructure:"db"`
       ChirpStack      CSConfig      `mapstructure:"chirpstack"`
       MQTT            MQTTConfig    `mapstructure:"mqtt"`
       Session         SessionConfig `mapstructure:"session"`
       TLS             TLSConfig     `mapstructure:"tls"`
   }

   type DBConfig struct {
       Host        string `mapstructure:"host"`
       Port        int    `mapstructure:"port"`
       Database    string `mapstructure:"database"`
       User        string `mapstructure:"user"`
       Password    string // resolved at load time via secrets.ReadSecret
       SSLMode     string `mapstructure:"ssl_mode"`
       MaxConns    int32  `mapstructure:"max_conns"`
   }

   func (d DBConfig) DSN() string {
       sslmode := d.SSLMode
       if sslmode == "" { sslmode = "prefer" }
       return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
           d.User, d.Password, d.Host, d.Port, d.Database, sslmode)
   }

   type CSConfig struct {
       GRPCURL  string `mapstructure:"grpc_url"`
       APIToken string // resolved at load time
       Insecure bool   `mapstructure:"insecure"`
   }

   type MQTTConfig struct {
       URL      string `mapstructure:"url"`
       User     string `mapstructure:"user"`
       Password string // resolved at load time
   }

   type SessionConfig struct {
       Key         string        // resolved at load time (32+ bytes random per OWASP)
       IdleTimeout time.Duration `mapstructure:"idle_timeout"`
       Lifetime    time.Duration `mapstructure:"lifetime"`
   }

   type TLSConfig struct {
       Mode   string `mapstructure:"mode"`   // acme | byo | internal — "none" REJECTED
       Domain string `mapstructure:"domain"`
       Email  string `mapstructure:"email"`
   }

   // Load reads SHIFTER_CONFIG_FILE (default /etc/shifter/config.yaml),
   // applies SHIFTER_* env overrides, resolves all *_FILE secrets,
   // and validates. D-05 + D-06 + D-22.
   func Load() (*Config, error) {
       v := viper.New()
       v.SetEnvPrefix("SHIFTER")
       v.AutomaticEnv()
       v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

       // Defaults
       v.SetDefault("env", "production")
       v.SetDefault("http_port", "8080")
       v.SetDefault("log_level", "info")
       v.SetDefault("logo_storage_dir", "/var/lib/shifter/logos")
       v.SetDefault("db.host", "postgres")
       v.SetDefault("db.port", 5432)
       v.SetDefault("db.database", "shifter")
       v.SetDefault("db.user", "shifter")
       v.SetDefault("db.ssl_mode", "prefer")
       v.SetDefault("db.max_conns", 10)
       v.SetDefault("chirpstack.grpc_url", "chirpstack:8080")
       v.SetDefault("chirpstack.insecure", false)
       v.SetDefault("mqtt.url", "tcp://mosquitto:1883")
       v.SetDefault("session.idle_timeout", "8h")
       v.SetDefault("session.lifetime", "24h")
       v.SetDefault("tls.mode", "acme")

       configPath := os.Getenv("SHIFTER_CONFIG_FILE")
       if configPath == "" {
           configPath = "/etc/shifter/config.yaml"
       }
       v.SetConfigFile(configPath)
       v.SetConfigType("yaml")
       if err := v.ReadInConfig(); err != nil {
           // Missing config file is OK if all required fields are set via env (e.g. CI / dev)
           if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
               // os.PathError is the typical case when SHIFTER_CONFIG_FILE points at non-existent
               if !os.IsNotExist(err) {
                   return nil, fmt.Errorf("read config: %w", err)
               }
           }
       }

       var cfg Config
       if err := v.Unmarshal(&cfg); err != nil {
           return nil, fmt.Errorf("unmarshal config: %w", err)
       }

       // Resolve secrets per D-06
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
       if cfg.Session.Key, err = ReadSecret("SHIFTER_SESSION_KEY"); err != nil {
           return nil, fmt.Errorf("session key: %w", err)
       }

       if err := cfg.Validate(); err != nil {
           return nil, err
       }
       return &cfg, nil
   }

   // Validate enforces D-22 (no plain HTTP in production), D-23 (dev allows insecure cookies),
   // and structural invariants.
   func (c *Config) Validate() error {
       switch c.TLS.Mode {
       case "acme", "byo", "internal":
           // ok
       case "none":
           return fmt.Errorf("config: tls.mode=none is not allowed in production (D-22)")
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

   func (c *Config) IsDev() bool { return c.Env == "dev" }
   ```

3. Create `internal/config/secrets.go` VERBATIM from RESEARCH §Pattern 12 with CRLF fix:
   ```go
   package config

   import (
       "fmt"
       "os"
       "strings"
   )

   // ReadSecret returns a secret value. Resolution order per D-06:
   //   1. Direct env var (e.g. SHIFTER_DB_PASSWORD="...") — dev only.
   //   2. File-backed env var (e.g. SHIFTER_DB_PASSWORD_FILE=/run/secrets/...).
   // Returns an error if neither is set.
   //
   // PITFALL #8: Windows-edited secret files have CRLF line endings. We trim BOTH \r and \n.
   func ReadSecret(name string) (string, error) {
       if v := os.Getenv(name); v != "" {
           return v, nil
       }
       if path := os.Getenv(name + "_FILE"); path != "" {
           b, err := os.ReadFile(path)
           if err != nil {
               return "", fmt.Errorf("read %s_FILE: %w", name, err)
           }
           return strings.TrimRight(string(b), "\r\n"), nil
       }
       return "", fmt.Errorf("secret %s not set (neither %s nor %s_FILE)", name, name, name)
   }

   // ReadSecretOrEmpty is ReadSecret but returns "" instead of an error
   // when neither env var is set. Used for optional secrets (e.g. mqtt password
   // when broker is anonymous).
   func ReadSecretOrEmpty(name string) (string, error) {
       v, err := ReadSecret(name)
       if err != nil {
           // Distinguish "not set" (OK) from real read errors (file exists but unreadable).
           if strings.Contains(err.Error(), "not set") {
               return "", nil
           }
           return "", err
       }
       return v, nil
   }
   ```

4. Create `config/config.example.yaml` — the canonical documented schema. Use exactly the content from `<interfaces>` block above.

5. Replace `internal/config/config_test.go` (and create new `internal/config/secrets_test.go`):

   `internal/config/config_test.go`:
   ```go
   package config

   import (
       "os"
       "path/filepath"
       "testing"

       "github.com/stretchr/testify/require"
   )

   func writeYAML(t *testing.T, content string) string {
       t.Helper()
       dir := t.TempDir()
       p := filepath.Join(dir, "config.yaml")
       require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
       return p
   }

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
   `)
       setupValidEnv(t, p)
       t.Setenv("SHIFTER_LOG_LEVEL", "debug")

       cfg, err := Load()
       require.NoError(t, err)
       require.Equal(t, "debug", cfg.LogLevel)
   }

   func TestValidate_RejectsTLSNone(t *testing.T) {
       cfg := &Config{
           Env: "production", LogLevel: "info",
           Session: SessionConfig{Key: "this-is-at-least-thirty-two-bytes-long-key", IdleTimeout: 1, Lifetime: 1},
           TLS: TLSConfig{Mode: "none"},
       }
       err := cfg.Validate()
       require.Error(t, err)
       require.Contains(t, err.Error(), "tls.mode")
   }

   func TestValidate_RequiresAcmeDomain(t *testing.T) {
       cfg := &Config{
           Env: "production", LogLevel: "info",
           Session: SessionConfig{Key: "this-is-at-least-thirty-two-bytes-long-key", IdleTimeout: 1, Lifetime: 1},
           TLS: TLSConfig{Mode: "acme", Domain: ""},
       }
       err := cfg.Validate()
       require.Error(t, err)
       require.Contains(t, err.Error(), "tls.domain")
   }
   ```

   `internal/config/secrets_test.go`:
   ```go
   package config

   import (
       "os"
       "path/filepath"
       "testing"

       "github.com/stretchr/testify/require"
   )

   func TestReadSecret_DirectEnv(t *testing.T) {
       t.Setenv("SHIFTER_TEST_SECRET", "value")
       v, err := ReadSecret("SHIFTER_TEST_SECRET")
       require.NoError(t, err)
       require.Equal(t, "value", v)
   }

   func TestReadSecret_FileEnv(t *testing.T) {
       p := filepath.Join(t.TempDir(), "secret")
       require.NoError(t, os.WriteFile(p, []byte("value\n"), 0o600))
       t.Setenv("SHIFTER_TEST_SECRET_FILE", p)
       v, err := ReadSecret("SHIFTER_TEST_SECRET")
       require.NoError(t, err)
       require.Equal(t, "value", v)
   }

   func TestReadSecret_CRLF(t *testing.T) {
       p := filepath.Join(t.TempDir(), "secret")
       require.NoError(t, os.WriteFile(p, []byte("value\r\n"), 0o600))
       t.Setenv("SHIFTER_TEST_SECRET_FILE", p)
       v, err := ReadSecret("SHIFTER_TEST_SECRET")
       require.NoError(t, err)
       require.Equal(t, "value", v, "PITFALL #8: must strip \\r\\n")
   }

   func TestReadSecret_Missing(t *testing.T) {
       _, err := ReadSecret("NONEXISTENT_VAR_12345")
       require.Error(t, err)
       require.Contains(t, err.Error(), "not set")
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/config -race -count=1</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/config/config.go` exports `type Config struct` with fields `Env`, `HTTPPort`, `LogLevel`, `DB`, `ChirpStack`, `MQTT`, `Session`, `TLS`
    - File `internal/config/config.go` exports `func Load() (*Config, error)` and method `(c *Config) Validate() error`
    - `Config.Validate()` returns an error when `c.TLS.Mode == "none"` (D-22)
    - File `internal/config/secrets.go` exports `func ReadSecret(name string) (string, error)`
    - `ReadSecret` strips both `\r` and `\n` from file content (verified by `TestReadSecret_CRLF`)
    - File `config/config.example.yaml` exists and contains keys `env:`, `tls:` (with `mode:`), `db:`, `chirpstack:`, `mqtt:`, `session:`, `logo_storage_dir:`
    - Command `go test ./internal/config -race` exits 0 with at least 7 passing tests
    - No test references `t.Skip` (all stubs are implemented)
  </acceptance_criteria>
  <done>
    Config + secrets layer ready. Plans 03/05/07/08/09/12/13 read from `*config.Config`; secrets resolved via `_FILE` env per Compose pattern.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: slog JSON logger + version package</name>
  <files>internal/logging/logging.go, internal/logging/logging_test.go, internal/version/version.go, internal/version/version_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (D-24, D-25)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 9: slog pretty-printed JSON" (lines 1361-1365)
    - 01-02-test-harness-PLAN.md (TestImageTagPinned stub in version_test.go)
  </read_first>
  <behavior>
    - TestNewLogger_OneJSONLinePerEvent: logs 3 events; assert output has 3 lines, each is valid JSON.
    - TestNewLogger_RespectsDebugLevel: cfg.LogLevel="debug", asserts a slog.Debug call gets emitted.
    - TestNewLogger_DefaultLevelInfo: cfg.LogLevel="info", asserts slog.Debug is dropped.
    - TestVersion_DefaultsToZero: linker has not injected; Version() returns "0.0.0-dev".
    - TestImageTagPinned: parse the example compose file (will exist after Plan 20/21) — for now, this stub asserts that the ImageTag constant equals Version().
  </behavior>
  <action>
1. Create `internal/logging/logging.go`:
   ```go
   package logging

   import (
       "io"
       "log/slog"
       "os"
   )

   // New returns a slog.Logger writing one JSON line per event to stdout (D-24, D-25).
   // Level: "info" or "debug". Anything else falls back to info.
   //
   // PITFALL #9: slog.NewJSONHandler is documented to write one JSON object per call to
   // Handle, with no pretty-printing. The default options are correct — we explicitly
   // pass a HandlerOptions to lock the level and to keep AddSource off (no \n in output).
   func New(level string) *slog.Logger {
       return NewWithWriter(os.Stdout, level)
   }

   func NewWithWriter(w io.Writer, level string) *slog.Logger {
       lvl := slog.LevelInfo
       if level == "debug" {
           lvl = slog.LevelDebug
       }
       h := slog.NewJSONHandler(w, &slog.HandlerOptions{
           Level:     lvl,
           AddSource: false,
       })
       return slog.New(h)
   }
   ```

2. Create `internal/version/version.go`:
   ```go
   package version

   // Version is set at build time via:
   //   go build -ldflags "-X github.com/shifter-io/shifter/internal/version.Version=v0.1.0" ./cmd/shifter
   var Version = "0.0.0-dev"

   // Commit is the git SHA, optionally injected.
   var Commit = "unknown"

   // BuildTime is the build timestamp (RFC3339), optionally injected.
   var BuildTime = "unknown"

   // ImageTag is the Docker image tag this binary expects to ship as.
   // OPS-01 verification asserts this matches the tag in compose files.
   var ImageTag = Version

   // Info returns a struct safe to embed in /health responses.
   func Info() Build {
       return Build{Version: Version, Commit: Commit, BuildTime: BuildTime}
   }

   type Build struct {
       Version   string `json:"version"`
       Commit    string `json:"commit"`
       BuildTime string `json:"build_time"`
   }
   ```

3. Replace `internal/logging/logging_test.go` (create new):
   ```go
   package logging

   import (
       "bytes"
       "encoding/json"
       "strings"
       "testing"

       "github.com/stretchr/testify/require"
   )

   func TestNewLogger_OneJSONLinePerEvent(t *testing.T) {
       var buf bytes.Buffer
       lg := NewWithWriter(&buf, "info")
       lg.Info("first")
       lg.Info("second")
       lg.Info("third")

       lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
       require.Len(t, lines, 3, "expect one JSON line per event (PITFALL #9)")
       for i, line := range lines {
           var m map[string]any
           require.NoErrorf(t, json.Unmarshal([]byte(line), &m), "line %d not valid JSON: %q", i, line)
           require.Equal(t, "INFO", m["level"])
       }
   }

   func TestNewLogger_RespectsDebugLevel(t *testing.T) {
       var buf bytes.Buffer
       lg := NewWithWriter(&buf, "debug")
       lg.Debug("debug-line")
       require.Contains(t, buf.String(), "debug-line")
   }

   func TestNewLogger_DefaultLevelInfo(t *testing.T) {
       var buf bytes.Buffer
       lg := NewWithWriter(&buf, "info")
       lg.Debug("debug-line-should-be-dropped")
       require.NotContains(t, buf.String(), "debug-line-should-be-dropped")
   }
   ```

4. Replace `internal/version/version_test.go`:
   ```go
   package version

   import (
       "testing"

       "github.com/stretchr/testify/require"
   )

   func TestVersion_HasDefault(t *testing.T) {
       require.NotEmpty(t, Version)
       require.NotEmpty(t, ImageTag)
   }

   // TestImageTagPinned is the OPS-01 unit test from VALIDATION.md.
   // It asserts the image tag matches the version (so compose tag == binary version).
   // The compose smoke tests (Plans 20, 21) verify the running container reports the same tag.
   func TestImageTagPinned(t *testing.T) {
       require.Equal(t, Version, ImageTag, "OPS-01: container image tag must equal binary version (no :latest)")
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/logging ./internal/version -race -count=1</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/logging/logging.go` exports `func New(level string) *slog.Logger` and `func NewWithWriter(w io.Writer, level string) *slog.Logger`
    - `slog.NewJSONHandler` is the only handler used (no text handler in production code path)
    - File `internal/version/version.go` exports package-level `var Version`, `var Commit`, `var BuildTime`, `var ImageTag` and a `func Info() Build`
    - `TestNewLogger_OneJSONLinePerEvent` passes (PITFALL #9 prevention)
    - `TestImageTagPinned` passes (OPS-01 unit; full check in Plan 20/21)
    - Command `go test ./internal/logging ./internal/version -race` exits 0
  </acceptance_criteria>
  <done>
    Logger + version package wired. `Plans 05+ import logging.New(cfg.LogLevel)`. `/health` (Plan 18) embeds version.Info().
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| operator → config | YAML + env vars; secrets via file mounts (D-06) |
| config → consumers | All later code reads validated `*config.Config` only |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-04-01 | Information Disclosure | secrets in `.env` files committed to git | mitigate | D-06 forbids `.env` for secrets; `_FILE` pattern reads from `/run/secrets/`; ASVS V8. |
| T-04-02 | Tampering | TLS downgrade via `tls.mode: none` | mitigate | `Validate()` rejects "none" with explicit error (D-22); ASVS V9. |
| T-04-03 | Information Disclosure | session key too short (≤16 bytes) → guessable | mitigate | `Validate()` requires `len(Session.Key) >= 32` (OWASP recommends 32+ bytes for session signing); ASVS V6. |
| T-04-04 | Information Disclosure | logging accidentally prints secrets | mitigate | slog with structured `slog.Attr` — secrets are never logged; review at PR (Plans 09, 12, 13 must not log Password / APIToken). ASVS V7. |
| T-04-05 | Tampering | CRLF in secret file silently corrupts password | mitigate | `ReadSecret` trims `\r\n`; tested in `TestReadSecret_CRLF`. PITFALL #8. |
</threat_model>

<verification>
- `internal/config/config.go` exports `Config` struct, `Load()`, `Validate()`
- `internal/config/secrets.go` exports `ReadSecret`, `ReadSecretOrEmpty`
- `internal/logging/logging.go` exports `New(level)` returning `*slog.Logger`
- `internal/version/version.go` exports `Version`, `Info()`
- `config/config.example.yaml` documents every key
- All tests in this plan pass via `go test ./internal/config ./internal/logging ./internal/version`
</verification>

<success_criteria>
- D-05 enforced (config.yaml + SHIFTER_* env)
- D-06 enforced (file-mounted secrets via `_FILE` env)
- D-22 enforced (`tls.mode: none` rejected)
- D-23 enforced (`Env=="dev"` flag exposed via `IsDev()`)
- D-24 enforced (slog JSON, one line per event)
- D-25 enforced (`info`/`debug` only)
- PITFALL #8 prevented (CRLF strip)
- PITFALL #9 prevented (one-line JSON)
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-04-SUMMARY.md` documenting:
- Config struct shape
- Env var → field mapping (so future plans know to set `SHIFTER_DB_PASSWORD_FILE` etc.)
- ReadSecret pattern for any future secret
- ldflags command for injecting `Version` at build time
</output>
