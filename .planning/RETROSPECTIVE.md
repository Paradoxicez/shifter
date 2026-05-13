# Shifter — Project Retrospective

Running log of phase retrospectives. Each entry is written at phase closure and captures what worked, what was inefficient, key lessons, and cost.

---

## Phase 7 — Multi-Vendor Breadth + v1.x Differentiators

**Closed:** 2026-05-13
**Plans:** 07-01 through 07-14 (14 plans, 13 waves)
**Requirements shipped:** V2-VEND-01, V2-VEND-02, V2-VEND-03, ALERT-04 (profile-aware tuning), INST-HARDEN, UX-POWER

### Scope deviations from ROADMAP

> ROADMAP SC#5 reads "ChirpStack version probe on first connect (refuses v3 or unknown)" — this was scoped per locked decisions D-12 and D-13 to manual `shifter doctor` subcommand invocation (no automatic connect-time hook in v1.x). Operators run `shifter doctor` after install and on demand; probes do NOT block `serve` startup. Rationale: install kit reliability beats added connect-path complexity in v1.x.

The three doctor subcommands (`probe-chirpstack`, `probe-timescale`, `probe-region`) deliver the diagnostic capability specified in INST-HARDEN. The connect-time refusal originally described in ROADMAP SC#5 was not implemented — this was an intentional scope reduction, not a missed requirement. D-12 records the ChirpStack version validation scope decision; D-13 records the decision that probes are diagnostic-only (not blocking). These decisions were locked before Phase 7 planning began.

### What worked

- **Wave-based execution:** Splitting Phase 7 into 13 waves with explicit `depends_on` chains prevented integration surprises. Plans 09a/09b and 11a/11b splits (I-1/M-2 deviation pattern) allowed parallel execution within waves.
- **goja sandbox isolation:** Running codec JS inside goja with a 50ms timeout and per-invocation isolate proved to be the correct architecture. The `TestCodecRunner_IsolatePerRun` test caught a shared-state bug early.
- **Embedded catalog pattern:** Using `embed.FS` for catalog JSON means zero runtime I/O for catalog lookups, and drift detection (loader vs DB) gives operators clear visibility into catalog staleness without a separate catalog service.
- **API key redaction via `errorClass(err)`:** The explicit sentinel test (`TestProbeChirpStack_NoAPIKeyInErrorMessage` with `SENTINEL_API_KEY_8f2c93`) enforced the T-07-14-03 threat mitigation at the test layer. This pattern (sentinel + negative assertion) is reusable for other secret-in-error-path risks.
- **Playwright graceful skip pattern:** Using `test.info().annotations.push({ type: 'skip-reason', ... })` for E2E tests that depend on seeded state (catalog update available, vendor profiles present) avoids false failures in CI while preserving intent documentation.

### What was inefficient / lessons

- **Plan 09 split came late:** The I-1 deviation (split 09 → 09a battery curve + 09b profile-aware workers) was identified during execution rather than at planning time. Splitting plans mid-wave disrupts wave numbering; future phases should decompose alert worker plans at design time.
- **Gateway model field naming:** The `ProbeRegion` query initially used `decommissioned_at` (non-existent) instead of `archived_at`. This was caught by a failing test (`TestProbeRegion_Mismatch` returned "error" instead of "warn") but cost one fix iteration. The gateway schema should be documented in a model reference file.
- **`internal/install/probe/` package location:** The API key redaction test was placed in `internal/install/probe/chirpstack_test.go` (per checker B-3 plan directive) in a new directory with no other files. This is slightly awkward but acceptable — the package exists purely to host the black-box sentinel test in a separate package from `internal/doctor`, ensuring no internal access to implementation details.
- **Unused import cleanup:** The initial `probes_test.go` draft imported `"google.golang.org/grpc/credentials/insecure"` without using it. The Go compiler caught this immediately, but it added a round-trip. Template for gRPC test helpers should not include credentials imports unless the test actually dials with TLS.

### Cost

- **Plans executed:** 14 (07-01 through 07-14)
- **Waves:** 13 (Wave 0 scaffolding through Wave 13 phase closure)
- **Auto-fix deviations:** 3 (archived_at field name, unused import, stray function reference in probes.go)
- **Architectural deviations (Rule 4):** 0
- **Scope deviations from ROADMAP:** 1 (SC#5 — connect-time ChirpStack refusal scoped to manual doctor invocation per D-12/D-13)
- **Test count added:** ~47 new unit/integration tests + 3 E2E Playwright specs
- **New packages:** `internal/codec/catalog/`, `internal/catalog/`, `internal/compare/`, `internal/alert/backtest/`, `internal/install/probe/`, `internal/doctor/` (extended), `internal/gateway/` (bulk import added)

---

*Retrospective format: phase name · closed date · scope deviations · what worked · what was inefficient · cost*
*First entry: Phase 7 (2026-05-13)*
