---
phase: 260513-qcf
plan: 01
subsystem: install/compose
tags: [bugfix, install, chirpstack, security]
key-decisions:
  - ChirpStack v4.10 TOML does not support ${VAR} env substitution; use sh entrypoint with sed-replace of POSTGRES_PASSWORD_PLACEHOLDER
  - ChirpStack v4 migrations require pg_trgm extension; added to postgres-init SQL
  - stateWireResponse wire type keeps DB layer untouched; only the HTTP response is sanitized
key-files:
  modified:
    - install/bundled/install.sh
    - compose/bundled.yml
    - internal/install/handlers.go
    - internal/install/state_test.go
    - compose/postgres-init/01-chirpstack.sql
    - compose/chirpstack/chirpstack.toml
  created:
    - compose/chirpstack/chirpstack.toml
    - compose/chirpstack/region_as923.toml
    - compose/chirpstack/region_as923_2.toml
    - compose/chirpstack/region_as923_3.toml
    - compose/chirpstack/region_as923_4.toml
    - compose/chirpstack/region_eu868.toml
    - compose/chirpstack/region_us915_0.toml
    - compose/chirpstack/region_au915_0.toml
    - compose/chirpstack/region_in865.toml
    - compose/postgres-init/01-chirpstack.sql
metrics:
  duration: ~35min
  completed: 2026-05-13
  tasks: 3
  files: 13
---

# Phase 260513-qcf Plan 01: Fix 3 Install Rehearsal Bugs — SUMMARY

**One-liner:** Removed dead host-path mkdir, shipped ChirpStack TOML configs with sed-based password injection and pg_trgm init, and added a stateWireResponse redactor to strip password_hash from the install state API.

## Tasks Completed

| # | Task | Commit | Status |
|---|------|--------|--------|
| 1 | Remove dead /var/lib/shifter/backups host-path step from install.sh | `7fb473e` | Done |
| 2 | ChirpStack config bootstrap — host-bind config dir + postgres init DB | `2762d95` | Done |
| 3 | Redact password_hash from GET /api/install/state wire response | `f9c5dbe` | Done |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] ChirpStack v4 TOML does not support ${VAR} env substitution**
- **Found during:** Task 2, live verification
- **Issue:** The plan specified `dsn="postgres://shifter:${POSTGRESQL_PASSWORD}@..."` with `export POSTGRESQL_PASSWORD=$(cat /run/secrets/postgres_password)` in the entrypoint. ChirpStack v4.10 reads TOML files literally — no `${VAR}` expansion. `configfile` output confirmed the literal string was passed as the password, causing `password authentication failed`.
- **Fix:** Replaced `${POSTGRESQL_PASSWORD}` in chirpstack.toml with the placeholder string `POSTGRES_PASSWORD_PLACEHOLDER`. The sh entrypoint now uses `sed "s/POSTGRES_PASSWORD_PLACEHOLDER/$PW/g"` to copy all TOMLs to `/tmp/chirpstack/` with the real password substituted, then points chirpstack at `/tmp/chirpstack`.
- **Files modified:** `compose/chirpstack/chirpstack.toml`, `compose/bundled.yml`
- **Commit:** `2762d95`

**2. [Rule 1 - Bug] command: vs entrypoint: — chirpstack image has its own ENTRYPOINT**
- **Found during:** Task 2, first restart cycle
- **Issue:** Using `command:` to set `["sh", "-c", "..."]` passes those as arguments to the chirpstack binary (which is the image's ENTRYPOINT), so chirpstack saw `sh` as an unrecognized subcommand.
- **Fix:** Changed to `entrypoint:` to override the image's ENTRYPOINT with the shell wrapper.
- **Files modified:** `compose/bundled.yml`
- **Commit:** `2762d95`

**3. [Rule 2 - Missing functionality] ChirpStack schema migrations require pg_trgm**
- **Found during:** Task 2, after password fix — chirpstack connected but migrations failed
- **Issue:** ChirpStack v4's initial schema migration creates a `gin_trgm_ops` index, which requires the `pg_trgm` extension. The `chirpstack` database was created without it.
- **Fix:** Added `\connect chirpstack` + `CREATE EXTENSION IF NOT EXISTS pg_trgm;` to `compose/postgres-init/01-chirpstack.sql`.
- **Files modified:** `compose/postgres-init/01-chirpstack.sql`
- **Commit:** `2762d95`

## Live Verification Results

**Task 1:**
- `bash -n install/bundled/install.sh` exits 0
- `grep -n "var/lib/shifter/backups" install/bundled/install.sh` returns empty
- Diff shows exactly 3 lines removed (echo + mkdir + chown)

**Task 2:**
- `docker ps --filter "name=compose-chirpstack-1"` → `Up About a minute` (NOT Restarting)
- ChirpStack logs show successful MQTT connections to all 8 regions + API gRPC serving
- Both `shifter` and `chirpstack` databases confirmed via `\l`
- `compose/chirpstack/` contains 9 TOML files (chirpstack.toml + 8 regions)
- `docker compose -f compose/bundled.yml config --quiet` exits 0

**Task 3:**
- `go test ./internal/install/... -run TestRedactStep1Admin -v` → 2 passed
- `go build ./internal/install/...` → success
- `redactStep1Admin` strips password_hash, preserves email/name, returns nil for nil input

## ChirpStack Region TOMLs — Source Notes

| File | Source |
|------|--------|
| region_as923.toml | Verbatim from official chirpstack-docker |
| region_as923_2.toml | Verbatim from official chirpstack-docker |
| region_as923_3.toml | Modeled on as923 — AS923-3 plan, 917.1–918.9 MHz sub-band |
| region_as923_4.toml | Modeled on as923 — AS923-4 plan, 917.3–920.9 MHz sub-band |
| region_eu868.toml | Verbatim from official chirpstack-docker |
| region_us915_0.toml | Verbatim from official chirpstack-docker |
| region_au915_0.toml | Modeled on official AU915 structure |
| region_in865.toml | Verbatim from official chirpstack-docker |

## Known Stubs

None. All changes are functional fixes with no placeholder data flowing to UI.

## Threat Flags

None. No new network endpoints, auth paths, or schema changes at trust boundaries introduced. The password redaction (Task 3) closes an existing information-disclosure risk.

## Self-Check: PASSED

- `install/bundled/install.sh` — exists, passes `bash -n`, no backups reference
- `compose/chirpstack/chirpstack.toml` — exists
- `compose/chirpstack/region_as923.toml` — exists
- `compose/chirpstack/region_as923_2.toml` — exists
- `compose/chirpstack/region_as923_3.toml` — exists
- `compose/chirpstack/region_as923_4.toml` — exists
- `compose/chirpstack/region_eu868.toml` — exists
- `compose/chirpstack/region_us915_0.toml` — exists
- `compose/chirpstack/region_au915_0.toml` — exists
- `compose/chirpstack/region_in865.toml` — exists
- `compose/postgres-init/01-chirpstack.sql` — exists
- `internal/install/handlers.go` — builds, contains redactStep1Admin
- `internal/install/state_test.go` — 2 new tests pass
- Commits `7fb473e`, `2762d95`, `f9c5dbe` — all present in git log
