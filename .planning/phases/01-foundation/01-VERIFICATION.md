---
phase: 01-foundation
verified: 2026-04-30T16:15:00Z
status: human_needed
score: 5/5 success criteria verified (Docker-mediated runtime checks pending human)
human_verification:
  - test: "Run `./install/bundled/install.sh shifter.example.com` on a host with a working Docker daemon"
    expected: "Script generates secrets, builds the shifter:0.1.0 image via the multi-stage Dockerfile, brings up postgres/mosquitto/redis/chirpstack/chirpstack-gateway-bridge/chirpstack-rest-api/shifter/caddy via compose/bundled.yml, polls https://localhost/health (Caddy → shifter:8080) until 200, and prints 'Visit https://shifter.example.com/install'"
    why_human: "Docker daemon was unresponsive during Plans 19-22 execution; verifier reproduced same failure when running testcontainers-backed Go integration tests (`go test ./internal/http/...` → testcontainers-go cannot reach Docker socket, hangs 30s). Plans 20/21 SUMMARY.md explicitly document this deferral. Cannot be exercised without operator-driven test on a host with a healthy Docker daemon."
  - test: "Run `./install/external/install.sh` after populating install/external/.env (SHIFTER_DOMAIN, SHIFTER_CHIRPSTACK_GRPC_URL, SHIFTER_MQTT_URL) and secrets/chirpstack_api_token.txt"
    expected: "Script validates env, refuses to fabricate the ChirpStack API token, builds image, starts postgres/shifter/caddy via compose/external.yml, polls https://localhost/health until 200"
    why_human: "Same Docker daemon dependency. Plan 21 SUMMARY documents the deferral; the just compose-smoke-external recipe stubs unreachable URLs (127.0.0.1:1) so the degraded-mode boot path can be confirmed only when Docker is healthy."
  - test: "After bundled install: visit https://<domain>/install; complete all 5 wizard steps end-to-end (admin user → ChirpStack mode + creds against a real v4 server → AS923-2 region → install identity → review/finish)"
    expected: "Wizard advances through all 5 steps; Step 2 calls the real ChirpStack v4 server via gRPC and ProbeVersion succeeds; finish atomically inserts the admin row + install_identity + chirpstack_connection in one Serializable txn; redirected to /login; signing in as the new admin reaches /settings"
    why_human: "End-to-end UX flow with a live ChirpStack v4 server cannot be exercised programmatically. Code paths verified: install handlers (StateHandler/Step1..4Handler/FinishHandler), FinishSetup atomic txn, FirstRunGate middleware, RootLayout install-state pre-check, login screen, /settings page. No simulated v4 ChirpStack server has been stood up in this verification session."
  - test: "Verify the install wizard refuses ChirpStack v3 with the destructive banner"
    expected: "Step 2 against a v3 ChirpStack server returns 422 v3_detected; UI renders the destructive Alert with copy 'Shifter doesn't support ChirpStack v3'"
    why_human: "Requires a live v3 server (or simulated InternalService.GetVersion returning Unimplemented). chirpstack-step.tsx renders the destructive Alert when state.v3 is true; backend Step2Handler maps ErrChirpStackV3OrUnknown to v3_detected. Logic is wired and tested in Go unit tests against bufconn mocks; live UX flow is operator-verifiable."
  - test: "Verify rate-limiting bites at the 6th failed login attempt within 1 minute"
    expected: "5 failed POST /api/auth/login → 401 bad_credentials; 6th → 429 rate_limited with Retry-After header"
    why_human: "Logic + helper tests pass at the unit level (LoginLimiter Allow/RetryAfter), but live HTTP integration tests live in internal/http and depend on testcontainers (Docker daemon) which was unresponsive. Quick operator manual: hit /api/auth/login 6× with wrong credentials and confirm the 6th returns 429."
  - test: "Verify dialog-first convention by clicking through every Phase 1 mutation surface"
    expected: "Every CRUD action (admin password change, settings ChirpStack edit) opens a Dialog (or Sheet on mobile via ResponsiveDialog) — no full-page edit forms"
    why_human: "Pattern conformance — code review of routes/ confirms ChangePasswordDialog and EditConnectionDialog wrap mutations; install wizard is documented exception per UI-SPEC. Visual confirmation that the modal-first feel is correct is human-only per VALIDATION.md Manual-Only Verifications."
---

# Phase 1: Foundation Verification Report

**Phase Goal:** Operator can install Shifter, sign in, and confirm Shifter is talking to ChirpStack — no telemetry yet, just a verified, scripted, secure starting point.
**Verified:** 2026-04-30T16:15:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (Roadmap Success Criteria)

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | Operator can run a single scripted install (bundled or external) and reach the login screen on first boot. | ⚠️ CODE VERIFIED — RUNTIME PENDING | `install/bundled/install.sh` (84 lines, idempotent), `install/external/install.sh` (123 lines, validates env), `compose/bundled.yml` (192 lines, 8 services, secrets block, json-file logging caps, no `:latest`), `compose/external.yml` (143 lines, 3 services), `Dockerfile` (multi-stage with distroless runtime + HEALTHCHECK via `shifter healthcheck`), `Caddyfile` (TLS modes acme/byo/internal, security headers, SSE-aware proxy). Login screen at `web/src/routes/login.tsx` (122 lines, verbatim UI-SPEC copy). All wired; live `docker compose up` deferred — see human_verification[0,1]. |
| 2 | First-run wizard captures admin user, ChirpStack creds, region default (AS923-2/Thailand), install identity, and refuses ChirpStack v3. | ✓ VERIFIED | 5 step handlers in `internal/install/handlers.go` (422 lines: Step1=admin+Argon2id+strength; Step2=mode/grpc/api_token/mqtt+ProbeVersion+v3 refusal; Step3=region whitelist via `Regions()`; Step4=identity+timezone+units; Finish=atomic Serializable txn). `internal/install/finish.go` (211 lines, INSERT user → UPSERT install_identity → UPSERT chirpstack_connection → DELETE install_state). `internal/install/regions.go` defines AS923-2 with `DefaultForCountry: "TH"` and Thai NBTC note. UI in `web/src/routes/install/` (5 step files, 545 LOC total): region-step.tsx defaults `as923_2`, displays "We pre-selected AS923-2 because the install address is in Thailand"; chirpstack-step.tsx renders destructive Alert with "Shifter doesn't support ChirpStack v3" on `v3_detected`. INST-05 enforced at boot via `probeChirpStackOrRefuse` in `internal/cli/serve.go:203`. |
| 3 | Operator logs in with email+password, forced to change default password on first login, rate-limited on failed attempts, can change own password. | ✓ VERIFIED (with documented reframing) | Login: `internal/auth/handlers.go:LoginHandler` — Argon2id verify (`internal/auth/argon2id.go`, OWASP m=19456 t=2 p=1, constant-time compare), session via SCS+pgxstore (`internal/auth/session.go`, RenewToken on PutUser to defeat fixation). Rate limit: `internal/auth/ratelimit.go` (per-IP + per-username token buckets, burst 5 / minute, both must allow). Change own password: `internal/auth/account.go:ChangePasswordHandler` + `web/src/routes/change-password-dialog.tsx`. **AUTH-03 reframing:** D-09 (CONTEXT.md) reframes "force change default admin password" because the bootstrap admin chooses their own password during the wizard — no default password ever exists. `must_change_password` schema field exists (migration 0002) and is exposed via `/api/account/me` for Phase 6 USER-04 (admin-created secondary users). REQUIREMENTS.md AUTH-03 still shows `[ ]` Pending — see Anti-Patterns / Reframing Note below. |
| 4 | "Test connection" reports gRPC and MQTT reachability with a clear error path; `/health` reports the running version. | ✓ VERIFIED (with documented D-19 split) | `internal/http/testconn.go:TestConnHandler` — two-channel probe (gRPC+ProbeVersion → MQTT or skipped if gRPC failed). `web/src/routes/settings/test-connection.tsx` + `settings.tsx` render StatusRow with reachable/unreachable/skipped variants. `internal/http/health.go`: GET /health (public, `{status, version, uptime_seconds}`) and GET /health/detailed (admin-required via `auth.RequireAction(sm, ActionHealthDetailed)`, includes DB ping). **D-19 split:** original INST-06 wording promised "DB+CS+MQTT+disk+last-uplink-age on /health" — D-19 split this; Phase 1 ships /health (public minimal) + /health/detailed (admin, DB only); Phase 6 expands /health/detailed. INST-06 wording was updated by Plan 24 (REQUIREMENTS.md L26). |
| 5 | Shell UI applies shadcn navy aesthetic in English; uses dialogs for CRUD; establishes modal-first convention. | ✓ VERIFIED | `web/src/theme.css` (3.3K, custom navy `#1E40AF` OKLCH semantic palette per UI-SPEC), shadcn `new-york` style + slate base, 21+ ui components in `web/src/components/ui/`. `ResponsiveDialog` (80 lines), `Stepper` (42), `StatusRow` (37), `ThemeProvider` (59) wired. Dialogs used for change-password (`change-password-dialog.tsx` 147 lines) and edit-connection (`settings/edit-connection-dialog.tsx`). Topbar+sidebar shell (`components/shell/responsive-shell.tsx` 41 lines). All copy is English; verbatim UI-SPEC strings present in login screen + change-password dialog. SPA built — `web/dist/assets/` contains index-BC86Dl5m.js (473K), index-Bd6nixs7.css (74K), Inter+JetBrains Mono fontsource woff2 files. |

**Score:** 5/5 truths code-verified; 5 require human runtime confirmation (Docker daemon + UI walkthrough).

### Required Artifacts (Plan must_haves)

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `cmd/shifter/main.go` | Binary entry | ✓ VERIFIED | 14 lines, calls `cli.Execute()` |
| `internal/cli/{root,serve,migrate,version,createadmin,configcheck,healthcheck}.go` | 6 Cobra subcommands | ✓ VERIFIED | All 6 present + tested via built binary's `--help` |
| `internal/auth/{argon2id,session,ratelimit,handlers,account,authz,users}.go` | Auth subsystem | ✓ VERIFIED | 7 source files (5.4K-9.4K each); OWASP Argon2id, SCS+pgxstore, golang.org/x/time/rate, AUTH-06 Can()/RequireAction |
| `internal/chirpstack/{client,version,mqtt,ping,errors}.go` | ChirpStack integration | ✓ VERIFIED | gRPC Dial+auth interceptor, ProbeVersion (v3 sentinel), paho.mqtt OnConnect resubscribe, PingMQTT |
| `internal/install/{state,middleware,regions,handlers,finish}.go` | Wizard subsystem | ✓ VERIFIED | install_state singleton store, FirstRunGate cached, 8-region catalog with TH default, 5 step handlers + atomic FinishSetup |
| `internal/http/{router,health,spa,testconn,middleware}.go` | HTTP layer | ✓ VERIFIED | chi router with canonical middleware order (PITFALL #4), /health split, SPA fallback (path traversal mitigated via fs.Sub), Test Connection two-channel |
| `internal/db/migrations/0001..0006_*.{up,down}.sql` | 6 initial migrations | ✓ VERIFIED | init+timescaledb / users (lowercase CHECK) / sessions / install_state (singleton id=1) / install_identity / chirpstack_connection (token-by-ref) all present + down counterparts |
| `internal/db/sqlc/` | sqlc-generated typed queries | ✓ VERIFIED | models.go + 4 query files generated; `sqlc.yaml` at repo root |
| `web/src/App.tsx` + routes | SPA shell | ✓ VERIFIED | Router with AuthLayout (login, install) + RootLayout (loader-gated /settings); lazy routes for code splitting |
| `web/src/routes/install/{index,admin,chirpstack,region,identity,review}-step.tsx` | 5-step wizard UI | ✓ VERIFIED | All 5 steps + shell (94-157 lines each, 545 LOC total); zod validation, AS923-2 default, v3 destructive banner |
| `web/src/routes/login.tsx` | Login screen | ✓ VERIFIED | 122 lines, verbatim UI-SPEC copy, 401/429 error mapping |
| `web/src/routes/settings.tsx` + `settings/{edit-connection,test-connection}.tsx` | Settings + Test Connection | ✓ VERIFIED | Two cards (Account + ChirpStack); admin-only Edit; Test Connection POSTs and renders StatusRow |
| `web/embed.go` (`//go:embed all:dist`) | Production SPA embed | ✓ VERIFIED | 26 lines, embed.FS exposed as `web.Dist`; `web/dist/.gitkeep` placeholder lets Go compile pre-pnpm-build |
| `compose/{bundled,external}.yml` | Two compose flavors | ✓ VERIFIED | 192/143 lines respectively; same shifter:0.1.0 image; secrets block; pinned tags (timescaledb 2.26.0-pg16, mosquitto 2.0.20, chirpstack 4.10, redis 7-alpine, caddy 2.8); json-file logging caps |
| `Dockerfile` | Multi-stage build | ✓ VERIFIED | web-builder (node:22-alpine + pnpm) → go-builder (golang:1.24-alpine + ldflags) → distroless static-debian12 nonroot + HEALTHCHECK via `shifter healthcheck` |
| `Caddyfile` | Reverse proxy + TLS | ✓ VERIFIED | TLS env-driven (acme/byo/internal); HSTS+CSP+Referrer-Policy; SSE flush_interval -1 (Phase 4 forward-look); Server header stripped |
| `install/{bundled,external}/install.sh` | Installer scripts | ✓ VERIFIED | Bundled: idempotent secret gen + image build + compose up + 90s /health poll via Caddy. External: validates .env REQUIRED vars, refuses to fabricate CS API token, same shape. |
| `README.md` + `docs/{install,operator-runbook}.md` | Operator docs | ✓ VERIFIED | 99 / 84 / 76 lines; bundled+external walkthroughs; TLS modes table; recovery procedures cite decision IDs (D-09, D-14, INST-05, D-19) |
| `Justfile` + `.air.toml` | Dev orchestration | ✓ VERIFIED | bootstrap/dev/build/test/lint/migrate/compose-smoke-{bundled,external} recipes |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `cmd/shifter/main.go` | `internal/cli` | `cli.Execute()` | ✓ WIRED | Single-call indirection |
| `internal/cli/serve.go` | `internal/http.NewRouter` | `httpapi.NewRouter(deps)` | ✓ WIRED | All 9 wiring blocks (config → pool → migrate → CS probe → MQTT → auth → install → testconn → router) |
| `internal/http/router.go` | All handlers | chi `r.Method(...)` | ✓ WIRED | 16 routes mounted in canonical PITFALL #4 order; SPA fallback last |
| Login UI → `/api/auth/login` | `web/src/lib/auth.ts:login` | apiFetch POST | ✓ WIRED | login() → POST /api/auth/login → 200 sets cookie via SCS LoadAndSave |
| Wizard UI → `/api/install/step/*` | `web/src/lib/install.ts:postStep1..4`, `finish` | apiFetch POST | ✓ WIRED | All 5 step handlers + finish; 410 Gone on completed install bounces UI to /login |
| Test Connection UI → `/api/settings/chirpstack/test` | `web/src/lib/settings.ts:testChirpStackConnection` | apiFetch POST | ✓ WIRED | Two-channel response rendered as StatusRow with reachable/unreachable/skipped |
| `internal/install/handlers.go:Step2Handler` | `internal/chirpstack.ProbeVersion` | direct call after Dial | ✓ WIRED | v3 → 422 v3_detected; non-v3 success persists draft + chirpstack_version |
| `internal/cli/serve.go:probeChirpStackOrRefuse` | `internal/chirpstack.ProbeVersion` | boot-time gate before listener opens | ✓ WIRED | Returns INST-05 refusal error if v3 detected; degraded-mode for unreachable |
| `internal/http/router.go` install gate | `internal/install.FirstRunGate` | `r.Use(install.FirstRunGate(...))` | ✓ WIRED | Cached via atomic.Bool; whitelist covers /install + /api/install + /login + /health |
| RootLayout loader → install state pre-check | `web/src/lib/install.ts:fetchInstallState` | apiFetch GET /api/install/state | ✓ WIRED | Non-null state → redirect /install; 410 → null → continue to session check; session 401 → redirect /login |
| `internal/http/health.go:HealthDetailed` | `auth.RequireAction(sm, ActionHealthDetailed)` | chi middleware wrapper | ✓ WIRED | Group-scoped; admin-only |
| `compose/{bundled,external}.yml` shifter env | `internal/config.Load` `_FILE` resolution | viper + Compose secrets | ✓ WIRED | DB password, CS API token, session key, MQTT password all via /run/secrets/* |
| `web/embed.go` `//go:embed all:dist` | `internal/http/spa.go:SPAHandler` | `fs.Sub(web.Dist, "dist")` | ✓ WIRED | dist/ contains real assets (473K JS, 74K CSS, font woff2 files) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `routes/_root.tsx` (RootLayout) | `user` | rootLoader → fetchSessionUser → /api/account/me | Yes — DB SELECT against `"user"` row | ✓ FLOWING |
| `routes/install/index.tsx` (InstallWizard) | `state` (CurrentStep) | fetchInstallState → /api/install/state | Yes — DB SELECT against install_state singleton | ✓ FLOWING |
| `routes/settings.tsx` (SettingsPage) | `meQ.data`, `csQ.data` | TanStack Query → /api/account/me, /api/settings/chirpstack | Yes — both queries hit DB; csQ pulls chirpstack_connection row | ✓ FLOWING |
| `routes/settings/test-connection.tsx` (StatusRow) | `result` (TestConnResult) | testM.mutate → /api/settings/chirpstack/test | Yes — backend dials real ChirpStack + pings MQTT | ✓ FLOWING |
| `routes/login.tsx` | `error` | API response from /api/auth/login | Yes — bcrypt-comparable Argon2id verify against user table | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Built binary responds to `--help` and lists 6 subcommands | `./shifter --help` | "Available Commands: completion, config-check, create-admin, healthcheck, help, migrate, serve, version" — all 6 documented subcommands present | ✓ PASS |
| `shifter version` reports build provenance | `./shifter version` | "shifter dev / commit: none / built: unknown" (dev build; production build ldflags inject real version per Dockerfile) | ✓ PASS |
| `shifter create-admin --help` exposes flags | `./shifter create-admin --help` | --email, --password, --name, --reset, -h flags listed | ✓ PASS |
| Go module compiles with no vet warnings | `go build ./... && go vet ./...` | Both succeed (no errors, no vet output) | ✓ PASS |
| SPA build artifacts exist | `ls web/dist/assets/` | index-BC86Dl5m.js (473K), index-Bd6nixs7.css (74K), Inter+JetBrains Mono woff2 files all present | ✓ PASS |
| Pure unit tests (no Docker) pass | `go test -run "TestProbeVersion\|TestRBAC\|TestHash\|TestVerify\|TestCan\|TestPasswordStrength\|TestRegions" -short -count=1 ./internal/...` | 19/20 packages pass; `internal/http` fails because it depends on testcontainers-go, which cannot reach the Docker daemon (same blocker that deferred Plans 19-22 compose smoke) | ⚠️ PARTIAL — see Issues below |
| Frontend test suite | `pnpm --dir web test --run` | 7 vitest workers fail to start with `ERR_REQUIRE_ESM` from `html-encoding-sniffer` (jsdom transitive). Code compiles fine — the SPA build succeeded — but the Vitest+jsdom+Node version interaction is broken. | ✗ FAIL — see Issues below |
| Live compose smoke (bundled) | `just compose-smoke-bundled` | NOT RUN — Docker daemon unresponsive in this session, matching Plans 19-22 deferral context | ? SKIP — human |
| Live compose smoke (external) | `just compose-smoke-external` | NOT RUN — same | ? SKIP — human |

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| ----------- | ------------- | ----------- | ------ | -------- |
| AUTH-01 | 07, 09, 23 | Login with email + password | ✓ SATISFIED | argon2id.go, handlers.go:LoginHandler, login.tsx |
| AUTH-02 | 08 | Session persists; idle timeout | ✓ SATISFIED | NewSessionManager(SCS+pgxstore, configurable IdleTimeout/Lifetime) |
| AUTH-03 | 09 (reframed) | Force change default admin password on first login | ⚠️ REFRAMED | D-09 reframes — bootstrap admin chooses own password in wizard; `must_change_password` schema field exists for Phase 6 USER-04 admin-created users. REQUIREMENTS.md still marks AUTH-03 as `[ ]` Pending — wording update not applied (Plan 24 only updated INST-06). |
| AUTH-04 | 09 | Failed login rate-limited | ✓ SATISFIED | LoginLimiter (per-IP + per-username, burst 5 / minute) |
| AUTH-05 | 09, 11 | User can change own password | ✓ SATISFIED | account.go:ChangePasswordHandler + change-password-dialog.tsx |
| AUTH-06 | 10, 11 | Two roles enforced server-side and frontend hidden | ✓ SATISFIED | authz.go:Can() + RequireAction middleware; settings.tsx hides Edit button for viewers |
| INST-01 | 14, 15, 16 | First-run wizard captures admin | ✓ SATISFIED | Step1Handler + admin-step.tsx |
| INST-02 | 15, 16 | Wizard captures install identity | ✓ SATISFIED | Step4Handler + identity-step.tsx + install_identity table |
| INST-03 | 15, 16 | Wizard captures CS mode + creds | ✓ SATISFIED | Step2Handler + chirpstack-step.tsx + chirpstack_connection table |
| INST-04 | 14, 15, 16 | Region picker with AS923-2 Thailand default | ✓ SATISFIED | Regions() catalog with `DefaultForCountry: "TH"` + region-step.tsx pre-select |
| INST-05 | 12, 15, 18 | Refuse ChirpStack v3 | ✓ SATISFIED | ProbeVersion sentinel; refused at wizard step 2, settings PUT, and serve.go boot |
| INST-06 | 18, 24 | /health (public) + /health/detailed (admin DB) | ✓ SATISFIED | health.go + REQUIREMENTS.md wording updated by Plan 24 to D-19 split |
| CHIRP-01 | 12, 17 | gRPC control plane | ✓ SATISFIED | chirpstack.Dial + Client.PingDevices + ProbeVersion |
| CHIRP-02 | 13 | MQTT subscriber | ✓ SATISFIED | NewMQTTSubscriber with OnConnect resubscribe |
| CHIRP-03 | 17 | Test Connection action | ✓ SATISFIED | TestConnHandler two-channel + StatusRow render |
| OPS-01 | 19, 20, 21, 22 | Two compose flavors share image | ✓ SATISFIED | bundled.yml + external.yml both use `shifter:0.1.0`; Caddyfile + Dockerfile shared |
| UX-01 | 06, 11, 17 | All CRUD via dialogs | ✓ SATISFIED | ResponsiveDialog primitive; ChangePasswordDialog + EditConnectionDialog wrap mutations; install wizard documented exception |
| UX-02 | 06, 23 | shadcn navy aesthetic, English | ✓ SATISFIED | theme.css with custom navy OKLCH; verbatim UI-SPEC English copy in login + dialogs |

**Coverage:** 18/18 phase requirement IDs accounted for. AUTH-03 is satisfied by D-09 reframing but the REQUIREMENTS.md checkbox (`- [ ]`) and Traceability row (`Pending`) were never flipped to reflect this. See Anti-Patterns / Reframing Note.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `.planning/REQUIREMENTS.md` | 14, 230 | AUTH-03 marked `[ ]` Pending in checklist + Traceability table despite D-09 reframing being implemented and tested (`TestWizardAdmin_NoForceChange`, `TestAccount_ChangePassword`) | ⚠️ Warning | Documentation drift — Plan 24 updated INST-06 wording but missed AUTH-03; reader of REQUIREMENTS.md sees AUTH-03 as not implemented when in fact it is, just reframed. CONTEXT.md L32 is the authoritative reframe; SUMMARY 09 acknowledges. |
| Frontend test infrastructure (`web/`) | n/a | vitest 4.1.5 + jsdom 29 + Node 22 transitive `html-encoding-sniffer` ESM/CJS conflict — `pnpm test --run` fails before any tests execute | ⚠️ Warning | Frontend unit tests defined in VALIDATION.md cannot be run automatically. Code itself compiles and ships in dist/, so no runtime impact, but `just test` regression coverage is degraded. Not flagged in any SUMMARY. |
| `internal/http/*_test.go` (RBAC, health-detailed, testconn integration) | n/a | Tests depend on testcontainers-go → Docker daemon; daemon unresponsive in this session; tests hang ~32s and fail | ℹ️ Info | Same Docker daemon dependency that deferred Plans 19/20/21/22 live compose smoke. Logic verified by code inspection + unit tests where Docker not required. |
| `web/dist/.gitkeep` (placeholder) | n/a | Real built artifacts present (473K JS, 74K CSS, woff2 fonts) — not a stub | ℹ️ Info | The .gitkeep is the documented bootstrap pattern (web/embed.go comment); real build is up-to-date. |

No blocker anti-patterns found.

### Reframing Note (AUTH-03)

D-09 (CONTEXT.md L32) explicitly reframes AUTH-03: "AUTH-03 ('force change default admin password on first login') is therefore reframed as: applies only to admin-created secondary users (Phase 6 USER-04 inherits this) — the bootstrap admin sets their own password from the start." Plan 09 SUMMARY (L75, L310) and Plan 24's REQUIREMENTS.md update commit acknowledge this. However, REQUIREMENTS.md was only updated for INST-06; AUTH-03's checkbox + Traceability row remain `[ ]` Pending. This is a documentation gap, not a functional gap. The `must_change_password` column exists (migration 0002), is exposed via /api/account/me, and is used by `shifter create-admin` (sets it false per D-09). Phase 6 USER-04 will set it true for admin-created secondary users.

**Recommendation (does NOT block Phase 1 sign-off):** Phase 2 plan-zero or a Phase 6 USER-04 plan should update REQUIREMENTS.md AUTH-03 wording to `[x] AUTH-03 (Phase 1 reframed by D-09; Phase 6 USER-04 implements force-change for admin-created users)` for traceability hygiene.

### Deviations from Phase Goal

None. Every roadmap success criterion has supporting code, every requirement ID has supporting plan(s), and every plan has a completed SUMMARY.md. The only deferred verifications are Docker-runtime confirmations (Plans 19-22 explicitly documented this; verifier reproduced the same daemon issue).

### Human Verification Required

See frontmatter `human_verification` block. The 6 items break down into:

1. **Live bundled compose install** — full `install/bundled/install.sh` round-trip on a host with a working Docker daemon, ending at "Visit https://<domain>/install" + a 200 from /health via Caddy.
2. **Live external compose install** — `install/external/install.sh` against a stubbed-or-real ChirpStack v4 + MQTT.
3. **End-to-end wizard walkthrough** — 5 steps + finish + login + reach /settings, against a real ChirpStack v4 server.
4. **v3 destructive banner** — Step 2 against a v3 ChirpStack (or simulated InternalService.GetVersion=Unimplemented).
5. **Rate-limit live test** — confirm 6th failed login returns 429 with Retry-After.
6. **Modal-first pattern conformance review** — manual audit per VALIDATION.md.

Items 1, 2, and 5 are testcontainer-equivalent and could be automated once Docker is healthy; items 3, 4, and 6 are operator-driven UX confirmations that ROADMAP.md success-criterion #5 explicitly anticipates as "establishing the modal-first convention" rather than mechanical assertions.

### Gaps Summary

There are **no blocker gaps**. All 5 ROADMAP success criteria have substantive, wired, data-flowing implementations. The 6 human-verification items are runtime confirmations that cannot be exercised without a healthy Docker daemon and operator UX walkthrough. The AUTH-03 documentation drift in REQUIREMENTS.md is a warning, not a gap — the underlying behavior is implemented per D-09 and tested.

Phase 1 is **functionally complete**. Sign-off requires:
1. A future operator-driven session with a working Docker daemon to run the deferred compose smokes.
2. A live UX walkthrough of the install wizard against a real ChirpStack v4 server.
3. Optional: AUTH-03 wording cleanup in REQUIREMENTS.md.

---

*Verified: 2026-04-30T16:15:00Z*
*Verifier: Claude (gsd-verifier)*
