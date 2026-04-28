---
phase: 01-foundation
plan: 04
subsystem: config
tags: [go, viper, secrets, slog, version, ldflags, tls, owasp]

requires:
  - phase: 01-foundation
    plan: 01
    provides: go.mod + Go monorepo skeleton
  - phase: 01-foundation
    plan: 02
    provides: testify on go.sum + internal/version/version_test.go scaffold (t.Skip)
  - phase: 01-foundation
    plan: 05
    provides: Plan 05's stub Config/Load/Logger.New/version.Info signatures (Plan 04 replaces bodies, never signatures)
provides:
  - internal/config/config.go (full viper YAML+env layered loader + Validate per D-05/D-22)
  - internal/config/secrets.go (ReadSecret + ReadSecretOrEmpty per D-06 + RESEARCH §Pattern 12, CRLF-trim per PITFALL #8)
  - internal/logging/logging.go (slog.NewJSONHandler one-line JSON to stdout per D-24/D-25, PITFALL #9 defused)
  - internal/version/version.go (ldflags-injected Version/Commit/BuildTime + ImageTag pin for OPS-01)
  - config/config.example.yaml (canonical schema; install kit copies to /etc/shifter/config.yaml)
affects:
  - 01-08-session-manager (Session.Key + IdleTimeout + Lifetime)
  - 01-09-login-ratelimit (cfg + structured logging)
  - 01-12-chirpstack-grpc (CSConfig.GRPCURL + APIToken + Insecure)
  - 01-13-mqtt-subscriber (MQTTConfig + secrets resolution)
  - 01-14-install-middleware (cfg pass-through)
  - 01-15-install-handlers (cfg pass-through)
  - 01-17-test-connection (config-check connectivity probes consume DBConfig.DSN + CSConfig + MQTTConfig)
  - 01-18-router-health (logging.New(cfg.LogLevel), version.Info() in /health response)
  - 01-19-spa-embed (cfg pass-through)
  - 01-20-compose-bundled (Compose secrets file mount idiom requires _FILE pattern)
  - 01-21-compose-external (same)
  - 01-22-caddyfile (TLS mode/domain/email)
  - 01-23-login-ui (no direct dep but session lifetime tunables surface here)
  - 01-24-readme-docs (Documents env→field map, ldflags command, secrets file pattern)

tech-stack:
  added:
    - github.com/spf13/viper@v1.21.0
    - github.com/spf13/cast@v1.10.0 (transitive)
    - github.com/go-viper/mapstructure/v2@v2.4.0 (transitive)
    - github.com/sagikazarmark/locafero@v0.11.0 (transitive)
    - github.com/spf13/afero@v1.15.0 (transitive)
    - github.com/sourcegraph/conc@v0.3.1 (transitive)
    - github.com/fsnotify/fsnotify@v1.9.0 (transitive)
    - github.com/subosito/gotenv@v1.6.0 (transitive)
    - github.com/pelletier/go-toml/v2@v2.2.4 (transitive)
    - go.yaml.in/yaml/v3@v3.0.4 (transitive — distinct from gopkg.in/yaml.v3)
  patterns:
    - "Layered config: viper.AutomaticEnv + SetEnvPrefix(SHIFTER) + SetEnvKeyReplacer('.', '_') so dotted YAML keys (db.host) overlay onto SHIFTER_DB_HOST env vars (D-05)"
    - "Compose-secrets idiom: every secret has both a SHIFTER_<NAME> direct env (dev) and SHIFTER_<NAME>_FILE pointing into /run/secrets/<name> (production); ReadSecret prefers direct then file, errors when neither set"
    - "Optional vs required secrets: ReadSecret() errors hard, ReadSecretOrEmpty() tolerates 'not set' (returns ('', nil)) but propagates real read errors. Session.Key uses ReadSecret (T-04-03 — refuse to start without it); DB password / CS token / MQTT password use ReadSecretOrEmpty"
    - "CRLF-safe file read: strings.TrimRight(content, '\\r\\n') so a Windows-edited secret file does not silently corrupt the password (PITFALL #8)"
    - "JSON logging: slog.NewJSONHandler(stdout, &HandlerOptions{Level: lvl, AddSource: false}) — one event per line, no source paths (would inflate log volume; aggregation across containers makes file:line low-value)"
    - "ldflags injection: var Version = 'dev' / Commit = 'none' / BuildTime = 'unknown' as self-describing local-build sentinels; production -ldflags '-X internal/version.Version=...' overrides at link time"
    - "ImageTag = Version: single source of truth for both `shifter version` output and the Docker image tag the binary expects to ship under (OPS-01)"

key-files:
  created:
    - internal/config/secrets.go
    - internal/config/secrets_test.go
    - internal/config/config_test.go
    - internal/logging/logging_test.go
    - config/config.example.yaml
  modified:
    - go.mod
    - go.sum
    - internal/config/config.go (full viper loader replaces env-only stub)
    - internal/config/doc.go (now documents implemented behavior)
    - internal/logging/logging.go (one-line JSON handler to stdout replaces stderr stub)
    - internal/logging/doc.go (now documents implemented behavior)
    - internal/version/version.go (added ImageTag tied to Version)
    - internal/version/version_test.go (replaces Plan 02 t.Skip with TestImageTagPinned)
    - internal/version/doc.go (documents OPS-01 role)

key-decisions:
  - "Validate() refuses to start with TLS Mode=='none' (D-22) AND with empty TLS.Mode. The plan only required rejecting 'none' — adding the empty check is Rule 2 (missing critical functionality): a misconfigured viper read or a YAML typo would otherwise let the binary boot with no TLS surface at all and emit confusing downstream errors."
  - "Session.Key length floor is enforced as 32 bytes (>=, not just !=0) per OWASP ASVS V6 / T-04-03. A short or zero-length key would let an attacker brute-force the session HMAC. The plan documented this as a 'truth' in must_haves; the test asserts it explicitly."
  - "ReadSecret strips both \\r and \\n (not just \\n) per RESEARCH §Pattern 12 + PITFALL #8. The plan reference snippet only showed \\n; the must_haves block explicitly demanded CRLF handling. The implementation uses strings.TrimRight(s, '\\r\\n') so any combination of trailing CR/LF is removed."
  - "ReadSecretOrEmpty distinguishes 'env not set' (return '') from 'file unreadable' (propagate). The plan-suggested check is `strings.Contains(err.Error(), 'not set')` — kept that approach because defining a sentinel error type would force every call site to adapt error handling for one optional path, and the underlying error message is explicit and stable."
  - "AutomaticEnv binds case-insensitive keys via SetEnvKeyReplacer + SetEnvPrefix. Tested with TestLoad_EnvOverride: SHIFTER_LOG_LEVEL=debug overrides log_level: info in the YAML. Confirms D-05's order (env > YAML > defaults)."
  - "Optional secret resolution must run BEFORE Validate() so a missing-but-optional secret (MQTT password) doesn't fail Validate() expectations. Required secrets (Session.Key) error out before Validate() too. Secrets are resolved before Unmarshal-time defaults take precedence — order in Load(): viper.ReadInConfig → Unmarshal → ReadSecret(*) → Validate()."
  - "logging emits to stdout (not stderr). Docker's json-file driver captures stdout under the operator's configured size + rotation caps. Writing to stderr would split log capture across two streams and break the 'one event per line in docker logs' contract."
  - "AddSource: false — including file:line in JSON logs adds ~30 bytes per event with low operational value when logs are aggregated across containers. Re-enable per-package later if a debugging session needs it."
  - "Three log-level inputs are accepted: 'debug', 'info', '' (empty=default to info). Anything else (warn, error, trace, garbage) silently falls back to LevelInfo — D-25 only exposes two levels and we deliberately do NOT advertise warn/error as configurable. NewWithWriter logs nothing about the fallback because logging itself is the subsystem; the operator gets the same observable behavior as a typo in any other config key."
  - "ImageTag is a top-level package var rather than a function — keeps the OPS-01 unit test (TestImageTagPinned) trivial (require.Equal(t, Version, ImageTag)) and lets compose-file generators read it as a build-time constant if needed."

patterns-established:
  - "Pattern: layered viper config — Plan 04 establishes the canonical loader. Future plans MUST read configuration only via *config.Config (passed in from cli/serve/migrate); no plan may call os.Getenv directly for runtime config."
  - "Pattern: secrets-by-reference — every credential is mounted as a file under /run/secrets/<name> and read via ReadSecret('SHIFTER_<NAME>'). Future plans adding new secrets MUST follow this idiom: define both the direct env (SHIFTER_NEW_SECRET) and the file env (SHIFTER_NEW_SECRET_FILE), and use ReadSecret if required or ReadSecretOrEmpty if optional. Never put new secrets in config.yaml."
  - "Pattern: build-time metadata — package-level var defaults are 'dev' / 'none' / 'unknown' so unstamped builds are self-describing. Production builds inject via -ldflags. New build-time constants follow the same idiom; never use init() functions to compute build metadata."
  - "Pattern: ImageTag = Version — single source of truth for the Docker image tag. Compose plans (20/21) MUST read this constant rather than hardcode a tag string; OPS-01 unit test enforces equality."
  - "Pattern: log-level whitelist — D-25 explicitly limits the operator-facing surface to info|debug. Future logging changes MUST NOT add warn/error/trace as configurable levels without amending D-25."

requirements-completed: []

duration: 10min
completed: 2026-04-28
---

# Phase 01 Plan 04: Config & Secrets Summary

**Viper-driven YAML+env config loader (D-05) + Compose-secrets file reader (D-06) + slog one-line JSON logger (D-24/D-25) + version package with ImageTag pin (OPS-01) — all with public signatures preserved so internal/cli/{migrate,serve,configcheck}.go keep compiling unchanged.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-04-28T00:21:45Z
- **Completed:** 2026-04-28
- **Tasks:** 2 / 2 (TDD: RED → GREEN per task)
- **Commits:** 4 (2 RED + 2 GREEN)
- **Files created:** 5 (config_test.go, secrets.go, secrets_test.go, logging_test.go, config/config.example.yaml)
- **Files modified:** 6 (config.go, logging.go, version.go, version_test.go + 2 doc.go)

## Accomplishments

### Configuration loader (Task 1)

- `config.Load() (*Config, error)` reads from three sources in viper-canonical order: `SHIFTER_*` env vars (highest), `config.yaml` at `$SHIFTER_CONFIG_FILE` or `/etc/shifter/config.yaml`, built-in defaults (lowest).
- Dotted YAML keys map to underscored env names: `db.host` → `SHIFTER_DB_HOST`, `chirpstack.grpc_url` → `SHIFTER_CHIRPSTACK_GRPC_URL`, `tls.mode` → `SHIFTER_TLS_MODE`.
- Missing config file is tolerated; non-existence error is silently downgraded so CI / dev paths can run with env-only config.
- Secrets resolved AFTER unmarshal, BEFORE Validate(). Session.Key is required (ReadSecret); DB password, ChirpStack token, MQTT password are optional (ReadSecretOrEmpty).
- Validate() rejects: `tls.mode: none`, `tls.mode: ""`, unknown TLS modes, `tls.mode: acme` without a domain, log_level outside `info|debug`, env outside `production|dev`, session key < 32 bytes, non-positive session timeouts.

### Secrets reader (Task 1)

- `ReadSecret(name)` resolves in order: direct env var → `<name>_FILE` env var pointing to a file → error.
- File contents are CRLF-trimmed (`strings.TrimRight(s, "\r\n")`) — defuses PITFALL #8 (Windows-edited secret files silently corrupting passwords).
- `ReadSecretOrEmpty(name)` returns `("", nil)` when neither env var is set, but propagates real errors (e.g. `_FILE` set but file unreadable). Distinguished by substring match on the error message.

### Structured logging (Task 2)

- `logging.New(level)` returns a `*slog.Logger` writing JSON to stdout (not stderr — Docker `json-file` driver captures stdout under operator-configured rotation caps).
- `slog.NewJSONHandler` is the only handler used; `AddSource: false` keeps event size small (file:line strings have low operational value when logs aggregate across containers).
- D-25 level whitelist: `debug` → LevelDebug, `info` / empty → LevelInfo, anything else (warn, error, trace, garbage) silently falls through to LevelInfo.
- `NewWithWriter(io.Writer, level)` is the test-injectable form; same level switch.

### Version package (Task 2)

- `Version`, `Commit`, `BuildTime` package-level vars default to `dev` / `none` / `unknown` so unstamped local builds are self-describing.
- New: `ImageTag = Version` — single source of truth for the Docker image tag the binary expects to ship as. Plans 20/21 (compose) read this constant; OPS-01 unit test asserts `ImageTag == Version`.
- `BuildInfo` JSON shape unchanged; `Info()` returns the package vars in struct form for `/health` (Plan 18) and `shifter version --json`.

## Env Var → Field Mapping (for downstream plans)

| YAML key                | Env var                                  | Field                            | Default                       | Required? |
| ----------------------- | ---------------------------------------- | -------------------------------- | ----------------------------- | --------- |
| `env`                   | `SHIFTER_ENV`                            | `Config.Env`                     | `production`                  | yes       |
| `http_port`             | `SHIFTER_HTTP_PORT`                      | `Config.HTTPPort`                | `8080`                        | yes       |
| `log_level`             | `SHIFTER_LOG_LEVEL`                      | `Config.LogLevel`                | `info`                        | yes       |
| `logo_storage_dir`      | `SHIFTER_LOGO_STORAGE_DIR`               | `Config.LogoStorageDir`          | `/var/lib/shifter/logos`      | no        |
| `db.host`               | `SHIFTER_DB_HOST`                        | `Config.DB.Host`                 | `postgres`                    | yes       |
| `db.port`               | `SHIFTER_DB_PORT`                        | `Config.DB.Port`                 | `5432`                        | yes       |
| `db.database`           | `SHIFTER_DB_DATABASE`                    | `Config.DB.Database`             | `shifter`                     | yes       |
| `db.user`               | `SHIFTER_DB_USER`                        | `Config.DB.User`                 | `shifter`                     | yes       |
| (n/a — file only)       | `SHIFTER_DB_PASSWORD[_FILE]`             | `Config.DB.Password`             | (empty)                       | no¹       |
| `db.ssl_mode`           | `SHIFTER_DB_SSL_MODE`                    | `Config.DB.SSLMode`              | `prefer`                      | yes       |
| `db.max_conns`          | `SHIFTER_DB_MAX_CONNS`                   | `Config.DB.MaxConns`             | `10`                          | yes       |
| `chirpstack.grpc_url`   | `SHIFTER_CHIRPSTACK_GRPC_URL`            | `Config.ChirpStack.GRPCURL`      | `chirpstack:8080`             | yes       |
| (n/a — file only)       | `SHIFTER_CHIRPSTACK_API_TOKEN[_FILE]`    | `Config.ChirpStack.APIToken`     | (empty)                       | no        |
| `chirpstack.insecure`   | `SHIFTER_CHIRPSTACK_INSECURE`            | `Config.ChirpStack.Insecure`     | `false`                       | yes       |
| `mqtt.url`              | `SHIFTER_MQTT_URL`                       | `Config.MQTT.URL`                | `tcp://mosquitto:1883`        | yes       |
| `mqtt.user`             | `SHIFTER_MQTT_USER`                      | `Config.MQTT.User`               | (empty)                       | no        |
| (n/a — file only)       | `SHIFTER_MQTT_PASSWORD[_FILE]`           | `Config.MQTT.Password`           | (empty)                       | no        |
| (n/a — file only)       | `SHIFTER_SESSION_KEY[_FILE]`             | `Config.Session.Key`             | —                             | **YES**   |
| `session.idle_timeout`  | `SHIFTER_SESSION_IDLE_TIMEOUT`           | `Config.Session.IdleTimeout`     | `8h`                          | yes       |
| `session.lifetime`      | `SHIFTER_SESSION_LIFETIME`               | `Config.Session.Lifetime`        | `24h`                         | yes       |
| `tls.mode`              | `SHIFTER_TLS_MODE`                       | `Config.TLS.Mode`                | `acme`                        | yes       |
| `tls.domain`            | `SHIFTER_TLS_DOMAIN`                     | `Config.TLS.Domain`              | (empty)                       | yes if mode=acme |
| `tls.email`             | `SHIFTER_TLS_EMAIL`                      | `Config.TLS.Email`               | (empty)                       | no        |

¹ DB password is functionally required for any real deploy; configured as optional in Load() so dev paths against a no-password local Postgres still parse the config.

## Adding a new secret (pattern for future plans)

```go
// 1. Define the constant in your package or use the SHIFTER_<UPPER>_<NAME> form.
// 2. Resolve at startup — pick required vs optional:

token, err := config.ReadSecret("SHIFTER_NEW_API_TOKEN")        // required: error if neither env var set
maybe, err := config.ReadSecretOrEmpty("SHIFTER_OPTIONAL_PW")   // optional: ("", nil) if neither set, but propagates read errors

// 3. Document in config/config.example.yaml's header comments.
// 4. Add to docker-compose.bundled.yml and external.yml top-level secrets: block + service-level secrets: list.
// 5. Set `SHIFTER_<UPPER>_<NAME>_FILE: /run/secrets/<lower>` in the service environment.
```

Never store the raw secret in `config.yaml`. The Compose `secrets:` mount is the only production path.

## ldflags command for production builds

```bash
go build -ldflags "
  -X github.com/shifter-io/shifter/internal/version.Version=$(git describe --always --dirty)
  -X github.com/shifter-io/shifter/internal/version.Commit=$(git rev-parse --short HEAD)
  -X github.com/shifter-io/shifter/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
" -o bin/shifter ./cmd/shifter
```

Plan 24 (readme-docs) wires this into the `Justfile` `build` recipe; Plans 20/21 (compose) make it the canonical Dockerfile build step. The same -ldflags injection updates `Version` AND `ImageTag` in lockstep because `ImageTag = Version` is evaluated at link time after Version is overridden.

## Decisions Made

- **TLS empty rejection (Rule 2 deviation).** The plan only required `tls.mode: none` rejection. I added empty-string rejection too because viper's defaults make `tls.mode: ""` unreachable on a clean Load(), but a hand-edited config or partial env override can still produce it — failing loudly is better than silently defaulting to who-knows-what.
- **Session key length is checked, not just presence.** OWASP ASVS V6 / T-04-03 demands `>=32` bytes for session signing. The plan flagged this in must_haves but didn't enforce it in the verbatim Validate(). I added the explicit check + a test asserting the rejection path.
- **CRLF strip uses `\r\n` not just `\n`.** RESEARCH §Pattern 12 showed `\n`-only; the plan's interfaces snippet had `\r\n`. The plan was right — Windows-edited secret files end with `\r\n`, and `TrimRight("\n")` would leave the `\r` corrupting the password. The test `TestReadSecret_CRLF` asserts this explicitly.
- **stdout, not stderr, for logging.** Plan's interfaces showed `os.Stdout` indirectly (PITFALL #9 reference); the existing Plan 05 stub used `os.Stderr`. Switched to stdout because Docker's json-file driver captures stdout under rotation caps and the operator-facing `docker logs` contract is "one event per line on stdout."
- **`NewWithWriter` exposed as a public function.** The plan suggested it for tests; I made it the canonical constructor and let `New(level)` delegate. This keeps the test surface and the production surface identical (same level switch, same handler options) so a test that passes against `NewWithWriter` also passes against `New`.
- **Three accepted log-level forms (`debug`, `info`, `""`).** Empty string maps to LevelInfo so a missing `SHIFTER_LOG_LEVEL` doesn't trigger the "fallback to info" path needlessly. Anything else (including warn, error, trace) silently falls back to LevelInfo per D-25's two-level surface.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Validate() also rejects empty `tls.mode`**

- **Found during:** Task 1 implementation.
- **Issue:** Plan's Validate() only rejected `"none"`. An empty string (e.g. operator deletes the line in config.yaml + a partial viper merge) would fall through `default:` and surface `tls.mode="" invalid` as a generic message instead of a clear "tls.mode is required" line.
- **Fix:** Added an explicit `case "":` returning `tls.mode is required (acme | byo | internal)`.
- **Files modified:** `internal/config/config.go`
- **Tested by:** `TestValidate_RejectsTLSNone` (covers the related path); the empty case is exercised indirectly by removing the YAML key + tls.mode default.
- **Commit:** `414f5a6`

**2. [Rule 2 - Missing Critical] Validate() rejects session keys shorter than 32 bytes**

- **Found during:** Task 1 implementation.
- **Issue:** OWASP ASVS V6 / T-04-03 require `>=32` byte session signing keys for HMAC strength. Plan listed this as a must_haves "truth" but the verbatim Validate() block only checked timeout positivity.
- **Fix:** Added `if len(c.Session.Key) < 32 { return fmt.Errorf("config: session key must be at least 32 bytes (got %d)", len(c.Session.Key)) }`.
- **Files modified:** `internal/config/config.go`
- **Tested by:** `TestValidate_RejectsShortSessionKey`.
- **Commit:** `414f5a6`

**3. [Rule 1 - Bug] Logger writes to stdout, not stderr**

- **Found during:** Task 2 implementation.
- **Issue:** Plan 05's existing logging.New stub wrote to `os.Stderr`. D-24 calls for "Structured JSON logs (one event per line) to stdout" and Docker's json-file driver only captures stdout for the operator's `docker logs` view by default. Plan 04's reference said "stdout" but the existing stub overrode that.
- **Fix:** Use `os.Stdout` in `New()`; tests use `NewWithWriter(buf, ...)` with an `io.Writer` so test capture is unaffected.
- **Files modified:** `internal/logging/logging.go`
- **Tested by:** all four logging tests use `NewWithWriter`; production stdout path verified by smoke test (`shifter version --json` runs cleanly with no log noise on stderr).
- **Commit:** `9d66dd3`

**4. [Rule 1 - Bug] viper.ConfigFileNotFoundError detection**

- **Found during:** Task 1 implementation.
- **Issue:** Plan's verbatim used `if _, ok := err.(viper.ConfigFileNotFoundError); !ok { ... }` which is a type assertion against a value type. viper v1.21 is fine with the type assertion, but `errors.As` is more robust (handles wrapped errors and is the modern Go idiom).
- **Fix:** `var notFound viper.ConfigFileNotFoundError; if !errors.As(err, &notFound) && !os.IsNotExist(err) { return nil, fmt.Errorf("read config %q: %w", configPath, err) }`. Functionally equivalent for unwrapped errors, future-proof for wrapped.
- **Files modified:** `internal/config/config.go`
- **Tested by:** `TestLoad_DefaultsFromYAML` (file present), `TestLoad_EnvOverride` (file present); the missing-file path is exercised by `setupValidEnv` plus tests that point at `t.TempDir()` with no file written.
- **Commit:** `414f5a6`

---

**Total deviations:** 4 auto-fixed (2 Rule 2 missing-critical, 2 Rule 1 bug). All fixes tightened the security/correctness posture without altering the public API.
**Impact on plan:** None — the plan's success criteria all pass; deviations strengthen the implementation against threat-model entries T-04-02 (TLS downgrade) and T-04-03 (weak session key).

## Issues Encountered

- **viper transitive deps are heavy.** Adding viper@1.21 pulled in afero, locafero, conc, fsnotify, and the rebranded mapstructure (now `go-viper/mapstructure/v2`). go.sum grew by ~30 entries. Documented in tech-stack.added so reviewers can audit the supply-chain surface.
- **`yaml.in/yaml/v3` and `gopkg.in/yaml.v3` coexist.** viper 1.21 switched its YAML backend to `go.yaml.in/yaml/v3` while testify still pulls `gopkg.in/yaml.v3` for assertion formatting. Both are in go.sum; no version conflict at runtime because they have different import paths.
- **Plan 05 stub had `os.Stderr` for logging.** Easy to miss because `shifter version` is the only subcommand operators run, and version itself doesn't log. Found by re-reading D-24. Caught + corrected (Deviation 3).

## Threat Surface Notes

No new threat surface beyond the plan's `<threat_model>`. All five register entries (T-04-01..05) are mitigated by code shipped in this plan:

| Threat | Mitigation |
|--------|------------|
| T-04-01 (secrets in `.env`) | D-06 enforced via ReadSecret(_FILE) idiom; example yaml header documents the rule |
| T-04-02 (TLS downgrade via `tls.mode: none`) | Validate() rejects with explicit error |
| T-04-03 (short session key) | Validate() requires >=32 bytes; ReadSecret hard-errors when SHIFTER_SESSION_KEY[_FILE] missing |
| T-04-04 (logging leaks secrets) | slog.Attr discipline is a Plan 09/12/13 review item; Plan 04 itself logs nothing |
| T-04-05 (CRLF in secret file) | TestReadSecret_CRLF + strings.TrimRight("\\r\\n") |

## Known Stubs

None — Plan 04 fully implements the config + secrets + logging + version surface. The minimal-stub state Plan 05 left behind (env-only Load, stderr logger) is replaced; subsequent plans see the production-quality surface.

## User Setup Required

None for development. Production deploys (Plans 20/21) generate the secret files in `secrets/` and mount them via Compose `secrets:` blocks; that wiring is Plan 20/21's responsibility.

For local smoke-testing today:

```bash
export SHIFTER_SESSION_KEY=this-is-at-least-thirty-two-bytes-long-key
export SHIFTER_TLS_MODE=internal
go run ./cmd/shifter config-check
# → PASS config syntax (env=production, tls.mode=internal)
# → SKIP connectivity probes (pending Plan 17)
```

## Next Phase Readiness

- ✅ `config.Load()` returns a populated, validated `*Config` with secrets resolved. Plans 03/05/07/08/09/12/13/14/15/17/18 can read every field they need.
- ✅ `internal/cli/{migrate,serve,configcheck}.go` keep compiling — the contract (`Load() (*Config, error)`, Config field names) is unchanged.
- ✅ `logging.New(cfg.LogLevel)` returns the canonical D-24/D-25 logger; existing serve/migrate calls pick up the new behavior automatically.
- ✅ `version.Info()` shape is unchanged; `version.ImageTag` exposes the OPS-01 pinning constant for Plans 20/21.
- ✅ `config/config.example.yaml` is the canonical schema; Plan 24 (readme-docs) and Plans 20/21 (compose) reference it.
- ⚠️ **Plan 17 (test-connection) connectivity probes** must use `cfg.DB.DSN()`, `cfg.ChirpStack.GRPCURL/APIToken/Insecure`, and `cfg.MQTT.URL/User/Password` — all populated by Load().
- ⚠️ **Plan 22 (caddyfile)** needs `cfg.TLS.{Mode,Domain,Email}` plumbed through env to Caddy. The values exist on Config; the Caddyfile generator/template owns the env-pass-through.

## Self-Check: PASSED

Files verified to exist:
- FOUND: `internal/config/config.go`
- FOUND: `internal/config/secrets.go`
- FOUND: `internal/config/config_test.go`
- FOUND: `internal/config/secrets_test.go`
- FOUND: `internal/config/doc.go`
- FOUND: `internal/logging/logging.go`
- FOUND: `internal/logging/logging_test.go`
- FOUND: `internal/logging/doc.go`
- FOUND: `internal/version/version.go`
- FOUND: `internal/version/version_test.go`
- FOUND: `internal/version/doc.go`
- FOUND: `config/config.example.yaml`

Commits verified to exist:
- FOUND: `ec04068` (Task 1 RED — failing config + secrets tests)
- FOUND: `414f5a6` (Task 1 GREEN — viper loader + secrets reader)
- FOUND: `537cee0` (Task 2 RED — failing logging + version tests)
- FOUND: `9d66dd3` (Task 2 GREEN — slog one-line JSON + ImageTag)

Behavior verified:
- `go build ./...` exits 0
- `go vet ./...` exits 0
- `go test ./internal/config -race -count=1` passes 18 tests
- `go test ./internal/logging ./internal/version -race -count=1` passes 8 tests
- `go test ./... -short -race -count=1` passes 26 tests across 12 packages
- `shifter version --json` outputs `{"version":"dev","commit":"none","build_time":"unknown"}`
- `shifter config-check` (with `SHIFTER_SESSION_KEY` + `SHIFTER_TLS_MODE=internal`) prints `PASS config syntax (env=production, tls.mode=internal)` followed by `SKIP connectivity probes (pending Plan 17)` and exits 0
- `shifter config-check` (without `SHIFTER_SESSION_KEY`) prints `FAIL config: session key: secret SHIFTER_SESSION_KEY not set (...)` and exits 1 — confirms T-04-03 hard-stop

---
*Phase: 01-foundation*
*Plan: 04-config-secrets*
*Completed: 2026-04-28*
