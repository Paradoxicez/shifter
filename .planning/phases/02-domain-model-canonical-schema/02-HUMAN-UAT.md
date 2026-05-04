---
status: partial
phase: 02-domain-model-canonical-schema
source: [02-VERIFICATION.md]
started: 2026-05-04T11:32:00Z
updated: 2026-05-04T11:32:00Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. End-to-end add-device atomic flow against real bundled-compose ChirpStack v4
expected: Operator runs single 'Add device' dialog action; ChirpStack tenant + application + profile + device + keys all created; Shifter device row appears; one audit_log row of action='create' / entity_type='device' written.
result: [pending]

### 2. Swap-then-cumulative-continuity visual check against the per-meter detail chart
expected: After swap commit on a populated MP, the per-meter detail chart (Phase 4 deliverable) shows the cumulative line crossing the swap timestamp without a vertical step.
result: [pending]
note: Cannot fully execute today — the per-meter cumulative chart ships in Phase 4. Defer.

### 3. Mapping editor "feels conversational" UX walk-through
expected: Reviewer walks through mapping a new profile end-to-end via tree-click without ever opening the manual JSON-pointer field; the experience does not feel overwhelming.
result: [pending]

### 4. shifter test-harness clean_swap --vendor axioma_w1 against a deployed install
expected: Operator runs the CLI subcommand against a freshly-installed Shifter binary connected to a real ChirpStack v4 + Mosquitto; cumulative chart shows the swap continuity per the runbook claim; W3 sync barrier reaches the success path within 10s.
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
