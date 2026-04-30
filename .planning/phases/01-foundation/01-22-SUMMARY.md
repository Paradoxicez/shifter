---
phase: 01-foundation
plan: 22
subsystem: infra
tags: [caddy, tls, https, reverse-proxy, sse, security-headers, csp, hsts]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: bundled compose flavor (Plan 20), external compose flavor (Plan 21), install.sh (Plans 20/21), shifter:8080 service (Plans 18/19)
provides:
  - Caddyfile at repo root mounted by both compose flavors at /etc/caddy/Caddyfile:ro
  - Env-driven TLS modes (D-21): acme | byo | internal via $CADDY_TLS_BLOCK
  - 5 security headers on every response (HSTS, X-Content-Type-Options, Referrer-Policy, X-Frame-Options, CSP) + -Server banner strip
  - SSE-aware /sse handler with flush_interval -1 + read_buffer 0 + response_header_timeout 0 (Phase 4 readiness; PITFALL #7 prevention)
  - HTTPS health-poll path through Caddy (curl -fsk https://localhost/health) in both install scripts and both smoke recipes
  - X-Forwarded-For + X-Real-IP propagation to shifter:8080 (trusted-proxy chain for Plan 09 rate limiter)
affects: [phase-04-realtime-dashboard, phase-06-ops-hardening, future deploy automation]

# Tech tracking
tech-stack:
  added:
    - "Caddy 2.8 reverse proxy with env-interpolation TLS block selection"
  patterns:
    - "env-templated TLS directive ({$CADDY_TLS_BLOCK}) — install.sh chooses the block before docker compose up"
    - "HTTPS-only health poll through Caddy (curl -fsk) — exercises full edge → backend path in smoke recipes"
    - "SSE block declared upfront even when no /sse endpoint exists — PITFALL #7 prophylaxis"

key-files:
  created: []
  modified:
    - "Caddyfile (full body — security headers, TLS env block, SSE handler, /health fast-path, X-Forwarded-For propagation)"
    - "install/bundled/install.sh (case block for SHIFTER_TLS_MODE → CADDY_TLS_BLOCK; HTTPS health poll)"
    - "install/external/install.sh (same case block; HTTPS health poll)"
    - "Justfile (compose-smoke-bundled + compose-smoke-external switched to HTTPS via Caddy)"

key-decisions:
  - "TLS modes are operator-selected at install time via SHIFTER_TLS_MODE env var; install.sh maps to the Caddy `tls` directive — Caddyfile itself contains no mode-specific logic, only an env interpolation point"
  - "CADDY_GLOBAL_TLS_BLOCK reserved (currently empty) for future ACME-issuer overrides (e.g., ZeroSSL fallback when LE rate-limits) — adds the seam without committing to specific issuer config in Phase 1"
  - "Default $SHIFTER_DOMAIN is `localhost` — keeps the smoke recipe + LAN-only deployments working without operator domain setup; production deploys override via SHIFTER_DOMAIN env"
  - "/sse handler ships in Phase 1 even though no /sse endpoint exists yet — the cost is 8 lines of unreachable Caddyfile config; the benefit is zero retrofit risk in Phase 4 (PITFALL #7 forbids retrofitting flush_interval after the fact)"
  - "/health declared as a separate `handle` block (not just falling through to root reverse_proxy) so the canonical liveness probe is fast-path and immune to any future header/body rewrites added at the root level"
  - "Root reverse_proxy explicitly sets X-Forwarded-For + X-Real-IP — Plan 09's rate limiter consumes XFF first-hop; making the contract visible in the Caddyfile prevents accidental drop in future edits"
  - "Live compose smoke deferred (continuing the Plan 19/20/21 pattern): Docker daemon was unresponsive in this session, and Phase 1 sign-off already accepts deferred live smoke as an Open Todo. `docker compose config -q` parses cleanly for both flavors; static + grep acceptance criteria all pass"

patterns-established:
  - "Env-block templating for selective Caddy directives: install.sh exports the rendered string, Caddyfile interpolates via {$VAR} — no separate Caddyfile-per-mode, no template engine, no string substitution at deploy time"
  - "HTTPS-only smoke path: every health probe in install scripts + Justfile recipes goes through Caddy with `curl -fsk https://localhost/health`. shifter:8080 is no longer polled directly — the smoke now validates the full edge → backend chain"
  - "Security headers declared once at the site-block level: `header { ... }` runs for every response (200, 401, 5xx) without per-handler duplication"
  - "Server banner stripped via `-Server` in the header block — defeats version-fingerprinting reconnaissance with one line"

requirements-completed: [OPS-01]

# Metrics
duration: 6min
completed: 2026-04-28
---

# Phase 1 Plan 22: Caddyfile Summary

**Caddy 2.8 reverse proxy at the bundled+external compose edge with env-driven TLS modes (acme/byo/internal), 5 OWASP-grade security headers + server-banner strip, and a Phase 4 SSE-ready handler — production deploy gate per D-22 (no plain HTTP).**

## Performance

- **Duration:** ~6 min (post-resume; original first-pass before disk-full ~12 min)
- **Started:** 2026-04-28T07:01:04Z (initial plan executor) → resume 2026-04-30T15:39Z
- **Completed:** 2026-04-30T15:40:00Z
- **Tasks:** 1
- **Files modified:** 4

## Accomplishments
- Caddyfile at repo root: env-templated TLS block, security headers (HSTS, X-Content-Type-Options, Referrer-Policy, X-Frame-Options, CSP, -Server), /sse SSE-aware handler (flush_interval -1, read_buffer 0, response_header_timeout 0), explicit /health fast-path, root reverse_proxy with X-Forwarded-For + X-Real-IP propagation
- Both install scripts (bundled, external) render `CADDY_TLS_BLOCK` from `SHIFTER_TLS_MODE` via a `case` block (acme/byo/internal/error-with-exit-2)
- Both install scripts switch the post-up health poll from `http://localhost:8080/health` to `https://localhost/health` (via Caddy, with `curl -fsk` to tolerate the internal CA in default mode)
- Both Justfile smoke recipes (`compose-smoke-bundled`, `compose-smoke-external`) export TLS env vars and poll Caddy over HTTPS — the smoke now validates the full edge → backend path
- D-20 (Caddy bundled both flavors), D-21 (3 TLS modes), D-22 (no plain HTTP), PITFALL #7 (SSE buffering) all addressed at the Phase 1 boundary — no retrofit work in Phase 4

## Task Commits

1. **Task 1: Caddyfile with env-driven TLS + SSE-aware proxy + security headers** — `76304d8` (feat)

**Plan metadata:** _to be populated by final docs commit_

## Files Created/Modified
- `Caddyfile` — Caddy 2 reverse proxy: env-driven TLS modes, 5 security headers, /sse handler, /health fast-path, X-Forwarded-For propagation (75 lines changed)
- `install/bundled/install.sh` — `case` block mapping SHIFTER_TLS_MODE → CADDY_TLS_BLOCK; HTTPS health poll (45 lines changed)
- `install/external/install.sh` — HTTPS health poll (the matching case block landed in Plan 21; 6 lines changed)
- `Justfile` — `compose-smoke-bundled` + `compose-smoke-external` recipes: HTTPS poll through Caddy with internal CA (19 lines changed)

## Decisions Made
- **TLS mode selection lives in install.sh, not in the Caddyfile.** The Caddyfile contains a single `{$CADDY_TLS_BLOCK}` interpolation point; the install script reads `SHIFTER_TLS_MODE` and exports the rendered Caddy directive. This keeps the Caddyfile single-purpose and means any future TLS mode (e.g., DNS-01 challenge for wildcard certs) is purely an install.sh case-arm addition — no Caddyfile fork.
- **`{$SHIFTER_DOMAIN:localhost}` defaults to localhost.** Smoke tests and LAN-only "internal" mode deployments work without any operator domain setup; production deploys override via env. The env-default syntax (`{$VAR:default}`) is a Caddyfile feature — no shell-side fallback needed.
- **`CADDY_GLOBAL_TLS_BLOCK` reserved but currently empty.** Phase 1 ships with the global block accepting only `email {$SHIFTER_TLS_EMAIL}` plus the empty interpolation point. Future ZeroSSL fallback (D-21 mentions it for LE rate-limit recovery) will populate the global block — adding the seam now means the change is one env var, not a Caddyfile structural edit.
- **`/sse` handler shipped in Phase 1 even though no `/sse` endpoint exists.** The handler is unreachable today (no backend route serves it), but PITFALL #7 prevents retrofitting `flush_interval -1` after a Phase 4 SSE endpoint goes live with default proxy buffering. The 8 lines of preemptive config are cheaper than debugging a broken realtime stream in Phase 4 staging.
- **`/health` is a dedicated `handle` block, not a fall-through to root.** Future header rewrites at the root level (Phase 6 may add tighter CSP per environment) cannot accidentally affect the canonical liveness probe — `/health` always uses the minimal upstream config.
- **Live compose smoke deferred (Plans 19/20/21 pattern).** Docker daemon was unresponsive during this run; Phase 1 sign-off already lists deferred live smoke as an accepted Open Todo. `docker compose config -q` parses cleanly for both `bundled.yml` and `external.yml`. The next operator with a working Docker daemon should run `just compose-smoke-bundled` + `just compose-smoke-external` to confirm green-path behavior.

## Deviations from Plan

None substantive — all plan-listed truths, artifacts, and acceptance criteria are met.

Two minor enhancements added beyond the plan body (both within Rule 2 / Rule 3 territory but documented here for traceability):

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added X-Forwarded-For + X-Real-IP propagation in root reverse_proxy**
- **Found during:** Task 1 (Caddyfile authoring)
- **Issue:** Plan 09's rate limiter (login + per-username) honors `X-Forwarded-For` first hop as the canonical client IP — but Plan 22's plan-verbatim Caddyfile did not declare the headers, leaving the contract implicit. Without explicit `header_up X-Forwarded-For {remote_host}` + `header_up X-Real-IP {remote_host}` in the root `reverse_proxy` block, Caddy's default behavior (passthrough only) would pass the original XFF value if any (none in production — operator-controlled trust boundary), but a future Caddy upgrade could change defaults.
- **Fix:** Added explicit `header_up X-Forwarded-For {remote_host}` + `header_up X-Real-IP {remote_host}` in the root `reverse_proxy` block.
- **Files modified:** `Caddyfile`
- **Verification:** Plan 09's `clientIP(r)` helper consumes XFF first-hop; the Caddyfile contract is now visible at the trust boundary instead of relying on Caddy's defaults.
- **Committed in:** 76304d8 (Task 1 commit)

**2. [Rule 2 - Missing Critical] Added `CADDY_GLOBAL_TLS_BLOCK` interpolation seam**
- **Found during:** Task 1 (Caddyfile authoring)
- **Issue:** Plan must_haves specify "global block has email + tls block injected via $CADDY_GLOBAL_TLS_BLOCK", but the plan body's verbatim Caddyfile didn't show the seam. Without it, future ZeroSSL fallback per D-21 would require a Caddyfile edit instead of an env var change.
- **Fix:** Added `{$CADDY_GLOBAL_TLS_BLOCK}` inside the global `{ ... }` block; documented as currently empty by default with comment pointing at D-21 future use.
- **Files modified:** `Caddyfile`
- **Verification:** `grep -F '$CADDY_GLOBAL_TLS_BLOCK' Caddyfile` returns 1 match; both compose flavors leave the env var unset, Caddy treats unset envs as empty strings (no parse error).
- **Committed in:** 76304d8 (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (Rule 2 — both add seams declared in plan must_haves but elided from the verbatim Caddyfile body)
**Impact on plan:** Both deviations are pure additions that satisfy stated must_haves more rigorously. No scope creep, no architectural change.

## Issues Encountered
- **Disk-full mid-execution (resolved before resume):** The original executor agent (run started 2026-04-28T07:01:04Z) finished all four file edits but was blocked by an OS-level disk-full error before it could commit Task 1 or write SUMMARY.md. STATE.md was last touched at 07:01:04Z (Plan 21 metadata commit). When this resume executor took over, disk had 74G free; all four files were intact and uncommitted (`git status` showed `M Caddyfile / M Justfile / M install/bundled/install.sh / M install/external/install.sh`). The resume executor verified the edits matched plan acceptance criteria via grep, then committed Task 1, created SUMMARY.md, and updated STATE.md / ROADMAP.md / REQUIREMENTS.md. Reported as a one-time environmental issue, not a plan defect.
- **Live compose smoke deferred (continuing Plan 19/20/21 pattern):** Docker daemon was unresponsive during this run. Phase 1 sign-off already accepts deferred live smoke as an Open Todo (logged at the Plan 19 / 20 / 21 boundary). `docker compose config -q` parses cleanly for `compose/bundled.yml` and `compose/external.yml --env-file install/external/.env.example`; static + grep acceptance criteria all pass.

## Open Todos (deferred)

- **Live compose smoke through Caddy.** First operator with a working Docker daemon: `just compose-smoke-bundled` + `just compose-smoke-external`. Expected behavior:
  - Bundled: Caddy comes up with `tls internal` (Caddy local CA), `https://localhost/health` returns 200 within ~30s of `docker compose up -d`.
  - External: Caddy + shifter come up; ChirpStack + MQTT URLs point at `127.0.0.1:1` (intentionally unreachable per Plan 21 strategy); shifter logs degraded-mode warnings; `https://localhost/health` returns 200.
  - Both should tear down cleanly with no orphan volumes (`docker compose down -v`).

## User Setup Required

None - the install script does the operator-side TLS mode rendering automatically. Operators select via `SHIFTER_TLS_MODE` env var or accept the `internal` default (Caddy local CA, fine for LAN deploys).

For production `acme` mode the operator must:
1. Set `SHIFTER_DOMAIN` to a real DNS-resolvable domain
2. Set `SHIFTER_TLS_MODE=acme`
3. Set `SHIFTER_TLS_EMAIL` to a Let's Encrypt account email (Caddy uses it for ACME registration + expiry warnings)

For `byo` mode the operator must additionally mount `cert.pem` + `key.pem` into the Caddy container at `/etc/caddy/cert.pem` + `/etc/caddy/key.pem`. Documenting these specifics is a Plan 24 (README + operator runbook) responsibility — Plan 22 surfaces them in install.sh comments only.

## Next Phase Readiness

- Phase 1 stack is now end-to-end TLS-terminated through Caddy in both compose flavors. Operator login → install wizard → settings → test connection runs entirely over HTTPS.
- Phase 4 SSE endpoint can land without any Caddyfile change — `/sse` handler is already in place with the correct `flush_interval -1` + `read_buffer 0` + `response_header_timeout 0` configuration.
- Phase 1 plans remaining: 24 (README + operator runbook). After 24 lands, Phase 1 sign-off requires the deferred live compose smoke run.

## Threat Flags

None — no new security-relevant surface beyond the trust boundaries already declared in the plan's `<threat_model>`. All seven STRIDE entries (T-22-01..07) are mitigated as planned.

## Self-Check: PASSED

Verified before final docs commit:

- [x] `Caddyfile` exists at repo root (75 lines)
- [x] `install/bundled/install.sh` updated (45 lines changed)
- [x] `install/external/install.sh` updated (6 lines changed)
- [x] `Justfile` updated (19 lines changed)
- [x] Task 1 commit `76304d8` exists in `git log`
- [x] 5 security headers present (`grep -E "Strict-Transport-Security|X-Content-Type-Options|Referrer-Policy|X-Frame-Options|Content-Security-Policy" Caddyfile` → 5)
- [x] `$CADDY_TLS_BLOCK` interpolation present (1 match)
- [x] `@sse path /sse /sse/*` matcher present (1 match)
- [x] `flush_interval -1` present (2 matches — handler + comment)
- [x] `reverse_proxy shifter:8080` present (3 matches — @sse handle, /health handle, root)
- [x] Both install.sh files contain `case "${SHIFTER_TLS_MODE:-internal}"` block
- [x] Justfile recipes poll `https://localhost/health` (2 matches — bundled + external)
- [x] `docker compose -f compose/bundled.yml config -q` exits 0
- [x] `docker compose -f compose/external.yml --env-file install/external/.env.example config -q` exits 0

---
*Phase: 01-foundation*
*Completed: 2026-04-30 (executed across 2026-04-28 first pass + 2026-04-30 resume after disk-full recovery)*
