---
phase: 6
slug: alerts-users-audit-operational-hardening
status: draft
# nyquist_compliant flips false -> true after the executor populates the
# Per-Task Verification Map below during Wave 0 of execute-phase. Until then
# the planner-emitted plans contain MISSING markers for any task whose test
# file does not yet exist; Wave 0 creates those scaffolds. wave_0_complete
# flips true once every MISSING marker is resolved.
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-12
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go: `go test` + testify · Frontend: vitest + playwright |
| **Config file** | `go.mod`, `web/vitest.config.ts`, `web/playwright.config.ts` |
| **Quick run command** | `go test ./internal/... -count=1 -timeout=60s` (backend), `pnpm -C web test --run` (frontend) |
| **Full suite command** | `go test ./... -count=1` + `pnpm -C web test --run` + `pnpm -C web exec playwright test` |
| **Estimated runtime** | ~120s quick · ~600s full |

---

## Sampling Rate

- **After every task commit:** Run quick command for the affected layer
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green AND CI round-trip backup/restore job green (D-45)
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

> Populated by gsd-planner during plan generation. Each plan task contributes a row mapping the task → requirement → threat reference (if any) → automated command. See `## Validation Architecture` in `06-RESEARCH.md` for the per-requirement test design.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

> Planner fills these in based on Validation Architecture in RESEARCH.md. Anticipated:

- [ ] `internal/alerts/alerts_test.go` — table-driven evaluator tests for threshold + offline + anomaly (D-01..D-19)
- [ ] `internal/auth/users_test.go` — admin user-mgmt CRUD + session-revoke + self-edit guards (D-23..D-30)
- [ ] `internal/audit/browse_test.go` — cursor pagination + filter + CSV export + prune function (D-31..D-38, D-51)
- [ ] `internal/cli/backup_test.go`, `internal/cli/restore_test.go`, `internal/cli/doctor_test.go` — CLI exit-code + manifest assertions (D-39..D-50)
- [ ] `.github/workflows/backup-restore-roundtrip.yml` — CI round-trip job (D-45)
- [ ] `web/src/routes/alerts/__tests__/`, `web/src/routes/audit/__tests__/`, `web/src/routes/settings/users/__tests__/` — Vitest specs for alert center, audit browse, users tab
- [ ] `web/e2e/phase6-*.spec.ts` — Playwright golden-path coverage (rule create → fire → ack → snooze; user add → share-once → first-login force-rotate; audit filter → export; backup run → restore via CLI)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Restore-needs-Shifter-down advisory lock UX | OPS-04 | Requires `docker compose stop shifter` orchestration — covered separately by CI job (D-45) | `docker compose stop shifter && docker compose run --rm shifter restore --from <tarball>` |
| Cron-sidecar nightly run | OPS-03 | Requires wall-clock advancement; CI runs `shifter backup` directly. Operator runbook documents manual `docker exec backup-cron ofelia daemon --test` smoke. | Inspect `compose/bundled.yml` for `ofelia` labels + run `docker exec backup-cron ofelia daemon --test` |
| Anomaly cold-start gate ≥21 days | ALERT-04 | Requires time travel; unit tests fake `now()` via `time.Now` injection but operator validates in production by checking the cold-start chip on a real MP | MP detail page → "Anomaly detection: warming up — N days remaining" |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 120s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
