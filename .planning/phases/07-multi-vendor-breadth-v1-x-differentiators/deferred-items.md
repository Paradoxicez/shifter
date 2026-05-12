# Deferred Items

## Pre-existing test failures (out of scope for 07-11b)

**ConsumptionChart.test.tsx — 3 failing tests**
- `renders without crashing with data` — expects Recharts SVG in DOM; Recharts may not render in jsdom
- `liveMode=true → pulse marker element present in DOM`
- `liveMode=true → pulse marker has motion-reduce:animate-none class`

These failures existed before plan 07-11b and are unrelated to the report templates UI work.
Discovered during: Task 1 GREEN phase (running test suite).
