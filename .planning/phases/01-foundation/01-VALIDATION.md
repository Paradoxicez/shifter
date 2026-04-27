---
phase: 1
slug: foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-27
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Source: `.planning/phases/01-foundation/01-RESEARCH.md` §Validation Architecture.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Backend framework** | Go stdlib `testing` + `testify/assert` |
| **Backend integration** | `testcontainers-go` for Postgres+TimescaleDB and Mosquitto; `mockgen` for ChirpStack gRPC interface |
| **Frontend framework** | `vitest` (Vite-native) for unit; `@testing-library/react` for component |
| **Backend config file** | `go.mod` (no separate test config); migrations applied per test via `db.RunMigrations` against testcontainer pool |
| **Frontend config file** | `web/vitest.config.ts` (extends `vite.config.ts`) |
| **Quick run command** | `go test ./internal/... -short -race && pnpm --dir web test --run` |
| **Full suite command** | `go test ./... -race -count=1 && pnpm --dir web test --run && pnpm --dir web build && just lint` |
| **Estimated runtime** | ~120 seconds (full suite incl. testcontainers cold start) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./<package> -race && pnpm --dir web test <relevant-files> --run`
- **After every plan wave:** Run `go test ./... -race -count=1 && pnpm --dir web test --run && pnpm --dir web build && just lint`
- **Before `/gsd-verify-work`:** Full suite + `just compose-smoke-bundled` + `just compose-smoke-external` must be green
- **Max feedback latency:** 30 seconds (quick run, hot testcontainers)

---

## Per-Task Verification Map

> Plan IDs and Task IDs are placeholders (`{plan}-{task}`); the planner will populate these. Each REQ-ID below MUST appear in at least one plan task's verification.

| Req ID | Behavior | Test Type | Automated Command | File Exists | Status |
|--------|----------|-----------|-------------------|-------------|--------|
| AUTH-01 | Login with email+password produces a session cookie | integration | `go test ./internal/auth -run TestLogin_Success -race` | ❌ W0 | ⬜ pending |
| AUTH-01 | Login with bad password is rejected (constant-time) | unit + integration | `go test ./internal/auth -run TestVerify_BadPassword_ConstantTime` | ❌ W0 | ⬜ pending |
| AUTH-02 | Session persists across two HTTP requests with the same cookie | integration | `go test ./internal/http -run TestSessionPersistence` | ❌ W0 | ⬜ pending |
| AUTH-02 | Session expires after `IdleTimeout` of inactivity | integration | `go test ./internal/auth -run TestSession_IdleTimeout` | ❌ W0 | ⬜ pending |
| AUTH-03 | Bootstrap admin (created via wizard) does NOT see force-change-password gate on first login | integration | `go test ./internal/auth -run TestWizardAdmin_NoForceChange` | ❌ W0 | ⬜ pending |
| AUTH-04 | 6th failed login from same IP within 1m returns 429 | integration | `go test ./internal/auth -run TestLogin_RateLimit_PerIP` | ❌ W0 | ⬜ pending |
| AUTH-04 | 6th failed login for same username from different IPs returns 429 | integration | `go test ./internal/auth -run TestLogin_RateLimit_PerUsername` | ❌ W0 | ⬜ pending |
| AUTH-05 | Authenticated user can change own password | integration | `go test ./internal/auth -run TestAccount_ChangePassword` | ❌ W0 | ⬜ pending |
| AUTH-05 | Changing password invalidates other sessions (defense-in-depth) | integration | `go test ./internal/auth -run TestAccount_ChangePassword_RevokeOtherSessions` | ❌ W0 | ⬜ pending |
| AUTH-06 | Viewer cannot POST to admin-only endpoints | integration | `go test ./internal/http -run TestRBAC_ViewerForbidden` | ❌ W0 | ⬜ pending |
| AUTH-06 | Admin can POST to admin-only endpoints | integration | `go test ./internal/http -run TestRBAC_AdminAllowed` | ❌ W0 | ⬜ pending |
| AUTH-06 | Frontend hides admin-only UI controls from viewer | component | `pnpm --dir web test components/account-menu.test.tsx` | ❌ W0 | ⬜ pending |
| INST-01 | First-run middleware redirects to /install when no admin user exists | integration | `go test ./internal/install -run TestFirstRun_Gate` | ❌ W0 | ⬜ pending |
| INST-01 | After wizard finish, /install becomes inaccessible (302 to /) | integration | `go test ./internal/install -run TestPostFinish_NoWizardAccess` | ❌ W0 | ⬜ pending |
| INST-02 | Wizard step 4 persists install_identity row | integration | `go test ./internal/install -run TestStep4_PersistsIdentity` | ❌ W0 | ⬜ pending |
| INST-03 | Wizard step 2 captures CS mode + creds and v3 probe | integration | `go test ./internal/install -run TestStep2_CapturesCS` | ❌ W0 | ⬜ pending |
| INST-04 | Wizard step 3 region picker defaults AS923-2 for Thailand | component | `pnpm --dir web test routes/install/region-step.test.tsx` | ❌ W0 | ⬜ pending |
| INST-04 | Saved region's `(name, common_name)` are written to chirpstack_connection | integration | `go test ./internal/install -run TestStep3_PersistsRegion` | ❌ W0 | ⬜ pending |
| INST-05 | Wizard rejects v3 ChirpStack with destructive banner | integration | `go test ./internal/install -run TestStep2_RejectsV3` | ❌ W0 | ⬜ pending |
| INST-05 | `serve` startup refuses to launch against v3 | integration | `go test ./internal/cli -run TestServe_RefusesV3` | ❌ W0 | ⬜ pending |
| INST-06 | `/health` returns `{status, version, uptime_seconds}` without auth | integration | `go test ./internal/http -run TestHealth_Public` | ❌ W0 | ⬜ pending |
| INST-06 | `/health/detailed` requires admin and returns DB ping result | integration | `go test ./internal/http -run TestHealthDetailed_RequiresAdmin` | ❌ W0 | ⬜ pending |
| CHIRP-01 | gRPC client dials and lists devices (uses mock) | integration | `go test ./internal/chirpstack -run TestClient_ListDevices_Mock` | ❌ W0 | ⬜ pending |
| CHIRP-01 | `ProbeVersion` returns version string for v4 mock | integration | `go test ./internal/chirpstack -run TestProbeVersion_v4` | ❌ W0 | ⬜ pending |
| CHIRP-01 | `ProbeVersion` returns ErrChirpStackV3OrUnknown when GetVersion is Unimplemented | integration | `go test ./internal/chirpstack -run TestProbeVersion_v3` | ❌ W0 | ⬜ pending |
| CHIRP-02 | MQTT subscriber connects and re-subscribes on reconnect | integration | `go test ./internal/chirpstack -run TestMQTT_ReconnectResubscribe` | ❌ W0 | ⬜ pending |
| CHIRP-02 | Uplink topic publish lands as a logged event | integration | `go test ./internal/chirpstack -run TestMQTT_UplinkLogged` | ❌ W0 | ⬜ pending |
| CHIRP-03 | Test Connection POST returns gRPC=reachable + MQTT=reachable for v4 + reachable Mosquitto | integration | `go test ./internal/http -run TestTestConn_Happy` | ❌ W0 | ⬜ pending |
| CHIRP-03 | Test Connection POST returns gRPC=unreachable + MQTT=skipped for v3 | integration | `go test ./internal/http -run TestTestConn_V3Refused` | ❌ W0 | ⬜ pending |
| CHIRP-03 | Frontend renders status-row component with green/red dots correctly | component | `pnpm --dir web test components/status-row.test.tsx` | ❌ W0 | ⬜ pending |
| OPS-01 | `docker-compose.bundled.yml` brings up all 6 services healthy | smoke (CI) | `just compose-smoke-bundled` | ❌ W0 | ⬜ pending |
| OPS-01 | `docker-compose.external.yml` brings up only Postgres+Caddy+shifter | smoke (CI) | `just compose-smoke-external` | ❌ W0 | ⬜ pending |
| OPS-01 | Both flavors share the same `shifter:<version>` image | unit | `go test ./internal/version -run TestImageTagPinned` | ❌ W0 | ⬜ pending |
| UX-01 | Every Phase 1 CRUD surface is a `<Dialog>` | manual + lint | Code-review checklist + Biome custom rule | manual | ⬜ pending |
| UX-02 | Login screen renders with shadcn navy primary, Inter font, English copy | component | `pnpm --dir web test routes/login.test.tsx` | ❌ W0 | ⬜ pending |
| UX-02 | Dark mode toggle persists in localStorage and applies `class="dark"` to root | component | `pnpm --dir web test components/theme-provider.test.tsx` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

**Backend test files (stubs to create):**
- [ ] `internal/auth/argon2id_test.go` — Hash + Verify round-trip; constant-time guarantee; PHC parsing edge cases
- [ ] `internal/auth/session_test.go` — IdleTimeout behavior, dev-mode Secure toggle
- [ ] `internal/auth/ratelimit_test.go` — per-IP and per-username buckets, cleanup goroutine
- [ ] `internal/auth/authz_test.go` — `Can()` matrix for admin and viewer
- [ ] `internal/install/state_test.go` — draft persistence, atomic finish, idempotency under concurrent finish
- [ ] `internal/install/middleware_test.go` — first-run redirect; post-install passthrough
- [ ] `internal/chirpstack/version_test.go` — v3 (Unimplemented) and v4 (success) paths
- [ ] `internal/chirpstack/mqtt_test.go` — testcontainer Mosquitto connect, subscribe, reconnect, graceful shutdown
- [ ] `internal/db/migrations_test.go` — clean run from empty schema; idempotent re-run; dirty-state surfacing
- [ ] `internal/http/health_test.go` — public minimum payload; admin-required detailed endpoint
- [ ] `internal/http/testconn_test.go` — happy path, gRPC-fail-skip-MQTT, both-fail
- [ ] `internal/http/spa_test.go` — fallback to index.html for unknown route, no fallback for `/api/*`, 404 for missing static assets
- [ ] `internal/cli/serve_test.go` — startup migration; v3-rejection-on-boot

**Frontend test files (stubs to create):**
- [ ] `web/src/lib/auth.test.ts` — fetch wrapper, redirect-on-401
- [ ] `web/src/routes/install/region-step.test.tsx` — Thailand → AS923-2 default
- [ ] `web/src/components/status-row.test.tsx` — three states (reachable/unreachable/skipped) render
- [ ] `web/src/components/responsive-dialog.test.tsx` — viewport-driven Dialog↔Sheet swap
- [ ] `web/src/components/theme-provider.test.tsx` — light/dark/system + localStorage persistence
- [ ] `web/src/routes/login.test.tsx` — navy primary, Inter font, English copy assertions
- [ ] `web/src/components/account-menu.test.tsx` — admin-only controls hidden from viewer

**Framework install (Wave 0 first task):**
```bash
# Backend test deps
go get github.com/stretchr/testify/assert
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
# Frontend test deps
pnpm --dir web add -D vitest @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom
```

**Config files:**
- [ ] `web/vitest.config.ts` — extends `vite.config.ts` with `test.environment = 'jsdom'`, `test.setupFiles = ['./src/test-setup.ts']`
- [ ] `web/src/test-setup.ts` — jest-dom matcher import

**CI smoke harness:**
- [ ] `just compose-smoke-bundled` recipe — `up -d`, wait for `/health`, then `down`
- [ ] `just compose-smoke-external` recipe — same with `external` compose file

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Every Phase 1 CRUD surface uses `<Dialog>`, no full-page edit screens | UX-01 | Pattern conformance is a design contract, not a discrete behavior; lint can catch the egregious cases (full-page route paired with edit form) but the canonical assertion is human review against UI-SPEC §Dialog Conventions | Reviewer reads each route file under `web/src/routes/`, confirms any mutation surface uses `<Dialog>` (or `<Sheet>` on mobile via `<ResponsiveDialog>`); flags any `<form>` rendered directly on a page route |
| Install wizard "feels conversational" per CONTEXT.md `<specifics>` | UX-02 | Tone is subjective — automated assertion would just lock copy strings, not the writing quality | Reviewer walks the 5-step wizard end-to-end, confirms copy matches "We just need a few details to get Shifter running" register, not "Configure Shifter" |
| Compose smoke tests pass on actual Docker (not just CI mock) | OPS-01 | CI runs Docker-in-Docker; final acceptance is the operator running `docker compose up` on a clean Linux host | `/gsd-verify-work` operator-driven test on a fresh VM |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
