---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "08"
subsystem: codec-test-runner-ui
tags: [codec, test-runner, ui, goja, surface-4, device-profiles]
dependency_graph:
  requires:
    - phase: 07-multi-vendor-breadth-v1-x-differentiators
      plan: "07"
      provides: "POST /api/device-profiles/{id}/test-codec backend endpoint"
    - phase: 04-realtime-dashboard
      plan: "09"
      provides: "JsonTree component for decoded JSON rendering"
  provides:
    - web/src/components/codec-test-runner/CodecTestRunner.tsx: Surface 4 panel (collapsed by default)
    - web/src/lib/codecTest.ts: runCodecTest() API helper + CodecTestResponse type
  affects:
    - web/src/routes/profiles/mapping-editor.tsx: CodecTestRunner mounted below codec_js textarea
tech_stack:
  added: []
  patterns:
    - "Collapsible panel via button+state toggle (no @radix-ui/react-accordion dep)"
    - "useMutation for POST test-codec — result stored in local state (not query cache)"
    - "Stack trace sub-toggle expanded by default when error_stack present"
    - "JsonTree reused from Phase 4 for decoded JSON + canonical mapping rendering"
key_files:
  created:
    - web/src/lib/codecTest.ts
    - web/src/components/codec-test-runner/CodecTestRunner.tsx
  modified:
    - web/src/components/codec-test-runner/CodecTestRunner.test.tsx
    - web/src/routes/profiles/mapping-editor.tsx
key_decisions:
  - "Route URL uses /api/device-profiles/{id}/test-codec (not /api/profiles/) — consistent with plan 07-07 deviation and existing profile route surface"
  - "Accordion replaced with simple button+state toggle — @radix-ui/react-accordion not installed in project; avoids adding dependency for single use case"
  - "Stack trace expanded by default (stackExpanded=true) — plan acceptance requires Copy Stack Trace visible without extra click; better UX for debugging"
requirements-completed: [V2-VEND-02]
metrics:
  duration_minutes: 5
  completed_date: "2026-05-13"
  tasks_completed: 2
  tasks_total: 3
  files_changed: 4
---

# Phase 07 Plan 08: Codec Test Runner UI Summary

**One-liner:** Collapsible codec test-runner panel (Surface 4) embedded in the device-profile editor — hex + fPort input, dual-tab output (Decoded JSON / Canonical Mapping) with JsonTree rendering and red error panel with line/col + stack trace; wired to POST /api/device-profiles/{id}/test-codec via useMutation.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add API client + implement CodecTestRunner component | 1a8728b | codecTest.ts, CodecTestRunner.tsx, CodecTestRunner.test.tsx |
| 2 | Mount CodecTestRunner inside Edit Profile dialog | e113246 | mapping-editor.tsx |
| 3 | Visual end-to-end verification (checkpoint) | — | awaiting operator |

## What Was Built

### API Client (`web/src/lib/codecTest.ts`)

- `runCodecTest(profileId, hex, fPort): Promise<CodecTestResponse>` — POSTs to `/api/device-profiles/{encodeURIComponent(profileId)}/test-codec`
- `CodecTestResponse` type with all 6 fields: `decoded_json`, `canonical_mapping`, `error_message`, `error_line`, `error_col`, `error_stack`
- Uses project-standard `apiFetch<T>` wrapper (handles auth, CSRF header, 401 redirect)

### CodecTestRunner Component (`web/src/components/codec-test-runner/CodecTestRunner.tsx`)

Surface 4 per UI-SPEC:

**Collapsed state (default):**
- Header button labelled "Test Codec" with chevron icon
- No input/output rendered until expanded

**Expanded state:**
- Left pane (full width on mobile, 1/2 width md+):
  - `fPort (optional)` number input (0–255)
  - `Payload (hex bytes)` monospace textarea with placeholder `e.g. 6F 01 23 45 AB CD`
  - `Run Test` button (disabled while no hex or pending)
  - Scratch-pad notice: "Results are not saved. Clear the form to reset."
- Right pane (flex-1):
  - Empty: "Run a test to see output here."
  - Success: shadcn `<Tabs>` with "Decoded JSON" (default) + "Canonical Mapping" tabs
    - Each tab: `<JsonTree>` component + `<Button aria-label="Copy ... to clipboard" title="Copy as JSON">`
  - Error: `role="alert"` red panel with error_message, `at line N, column M`, stack trace toggle (expanded by default) + `Copy Stack Trace` button

**Accessibility:**
- `aria-label="Copy decoded JSON to clipboard"` on decoded JSON copy button
- `aria-label="Copy canonical mapping to clipboard"` on canonical mapping copy button
- `aria-label="Copy error stack trace to clipboard"` on stack trace copy button
- `role="alert"` on error panel
- `aria-expanded` on collapsible header and stack trace toggle

### Profile Editor Integration (`web/src/routes/profiles/mapping-editor.tsx`)

`{profileId && <CodecTestRunner profileId={profileId} />}` added below the `codec_js` Textarea in the right pane of the two-pane body section. Hidden for new (unsaved) profiles — no profileId to test against.

### Tests (3/3 pass)

- `renders Test Codec panel collapsed by default` — panel header visible, input fields not rendered
- `shows decoded JSON tree on successful test` — mocks `{decoded_json: {cumulative: 123.456}}`, asserts "cumulative" key visible in Decoded JSON tab
- `renders error panel with line/col on goja exception` — mocks error response, asserts `role="alert"`, `at line 5, column 12`, "Copy Stack Trace"

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Accordion component not installed**
- **Found during:** Task 1 implementation
- **Issue:** Plan specified using `<Accordion>/<AccordionItem>/<AccordionTrigger>/<AccordionContent>` from shadcn for the collapsible panel and stack trace sub-section. `@radix-ui/react-accordion` is not installed in the project (not in package.json, no `web/src/components/ui/accordion.tsx`).
- **Fix:** Implemented collapsible behaviour with a simple `useState(expanded)` + `<button aria-expanded={...}>` toggle. Same UX, no new dependency. Stack trace sub-section uses the same pattern with `useState(true)` (expanded by default) so "Copy Stack Trace" is immediately visible.
- **Files modified:** `web/src/components/codec-test-runner/CodecTestRunner.tsx`
- **Commit:** 1a8728b

**2. [Rule 1 - Bug] Stack trace expanded by default**
- **Found during:** Task 1 tests (GREEN phase) — test `renders error panel with line/col` asserted `getByText('Copy Stack Trace')` but button was behind a collapsed toggle.
- **Issue:** Plan's acceptance criterion says "Copy Stack Trace button is visible" without requiring extra user interaction. Having it collapsed by default violated the test expectation and the UX intent (error debugging should be immediate).
- **Fix:** Changed `useState(false)` → `useState(true)` for `stackExpanded`. Operator can collapse the stack trace after reading it.
- **Files modified:** `web/src/components/codec-test-runner/CodecTestRunner.tsx`
- **Commit:** 1a8728b

## Known Stubs

None — Task 3 is a human-verify checkpoint (visual end-to-end). No code stubs.

## Threat Surface Scan

No new network endpoints, auth paths, or trust boundaries introduced by this plan. The frontend calls `POST /api/device-profiles/{id}/test-codec` which is admin-only with per-user 30/min rate limit (plan 07-07). All sandbox safety is server-side.

## Self-Check: PARTIAL (Task 3 pending operator verification)

Files verified on disk:
- web/src/lib/codecTest.ts: FOUND
- web/src/components/codec-test-runner/CodecTestRunner.tsx: FOUND
- web/src/components/codec-test-runner/CodecTestRunner.test.tsx: FOUND (3 tests pass)
- web/src/routes/profiles/mapping-editor.tsx (CodecTestRunner import + mount): FOUND

Commits verified:
- 1a8728b: feat(07-08): implement CodecTestRunner Surface 4 + runCodecTest API client
- e113246: feat(07-08): mount CodecTestRunner in profile editor below codec_js textarea

Test results:
- `pnpm --dir web test:run -- src/components/codec-test-runner`: 3/3 PASS
- `pnpm --dir web typecheck`: PASS (exit 0)
- `pnpm --dir web build`: PASS (4.63s)

UI-SPEC acceptance criteria (all 15 grep checks):
- "Test Codec": FOUND
- "fPort (optional)": FOUND
- "Payload (hex bytes)": FOUND
- "Run Test": FOUND
- "Results are not saved. Clear the form to reset.": FOUND
- "Decoded JSON": FOUND
- "Canonical Mapping": FOUND
- "Run a test to see output here.": FOUND
- aria-label="Copy decoded JSON to clipboard": FOUND
- aria-label="Copy canonical mapping to clipboard": FOUND
- aria-label="Copy error stack trace to clipboard": FOUND
- "Copy Stack Trace": FOUND
- "at line": FOUND
- role="alert": FOUND
- "JsonTree": FOUND
