---
status: partial
phase: 04-realtime-dashboard
source: [04-VERIFICATION.md]
started: 2026-05-11T15:45:00Z
updated: 2026-05-11T15:45:00Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. SSE live update — inject a real measurement via the backend and confirm the KPI instant tile updates in the browser without a page reload
expected: The [data-kpi='instant'] tile value changes within 5 seconds of injection without a URL change
why_human: The backend /internal/test/inject-measurement endpoint referenced by the E2E fixture helper does not exist in any non-test binary build. The Playwright dashboard-live-update.spec.ts gracefully no-ops on injection failure and skips the live-update assertion — so the E2E spec passes structurally but does not actually prove DASH-02/DASH-03 live-update behavior end-to-end.
result: [pending]

### 2. Playwright E2E suite execution against a running server
expected: All 6 Phase 4 specs pass — dashboard-loads, dashboard-live-update, dashboard-capability-filter, dashboard-date-range, metering-point-detail, dashboard-mobile
why_human: The storageState pre-auth fixture (web/playwright/fixtures/admin-session.json) contains a stub placeholder cookie value 'stub-admin-session-replace-via-playwright-save-storage'. Every Phase 4 spec that uses test.use({ storageState }) will receive a 401 against a real server. The specs need to be run after regenerating admin-session.json per the fixture README instructions.
result: [pending]

### 3. Caddy reverse proxy SSE non-buffering in production
expected: curl -N https://<host>/api/events streams events without buffering; SSE reconnects survive Caddy restarts
why_human: The @sse matcher and flush_interval -1 are correctly configured in Caddyfile, but integration testing requires a production-equivalent reverse proxy. Cannot verify programmatically without a deployed instance.
result: [pending]

### 4. Mobile device real-browser SSE reconnect after tab backgrounding
expected: Opening dashboard on iPhone Safari and Android Chrome, backgrounding the tab for 60s, then foregrounding resumes KPI updates within 30s
why_human: Playwright viewport emulation covers layout (DASH-06 mobile grid confirmed by spec), but real mobile Safari tab-background/foreground SSE reconnect behavior requires a physical device test.
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
