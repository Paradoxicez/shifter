---
phase: 03-provisioning-gateways-devices-bulk-import
plan: 03
subsystem: chirpstack
tags: [chirpstack, grpc, gateway, device, singleflight, otaa, abp, metrics-cache]

# Dependency graph
requires:
  - phase: 03-provisioning-gateways-devices-bulk-import-01
    provides: testsupport ChirpStack Phase 3 mock (Gateway service + Device reveal methods)
  - phase: 02-foundation-auth-tenancy-and-chirpstack-bootstrap
    provides: Client struct, CreateDevice / CreateDeviceKeys / DeleteDevice wrappers, EnsureTenant + EnsureApplication bootstrap
provides:
  - ChirpStack v4.17 GatewayService 6-RPC wrapper surface (Create/Get/Update/Delete/List/GetMetrics)
  - ErrNotFound sentinel + wrapCSErr translation helper for codes.NotFound
  - MetricsCache (1-minute TTL + singleflight deduplication) for the gateway list page metrics fan-out
  - DeviceService Activate (ABP), GetKeys (OTAA reveal), GetActivation (ABP reveal) wrappers
  - LoRaWAN 1.0.x 3-key copy invariant enforced inside ActivateDevice (NwkSEncKey = SNwkSIntKey = FNwkSIntKey = NwkSKey)
affects: [03-04, 03-05, 03-06, 03-07, 03-08, 03-09, 03-10]

# Tech tracking
tech-stack:
  added:
    - "golang.org/x/sync/singleflight (promoted from indirect to direct)"
  patterns:
    - "wrapper-with-sentinel: every RPC returns wrapCSErr(err) so codes.NotFound translates to a typed ErrNotFound usable by handlers via errors.Is"
    - "domain-shape projection: handlers see *Gateway / *DeviceKeys / *DeviceActivation structs, never api.* protos — keeps chirpstack package the sole importer of api/go/v4"
    - "in-memory cache with singleflight: per-key map + sync.RWMutex + singleflight.Group; clock injectable via cache.now for deterministic TTL tests"

key-files:
  created:
    - internal/chirpstack/gateway.go
    - internal/chirpstack/gateway_metrics_cache.go
  modified:
    - internal/chirpstack/errors.go (added ErrNotFound + wrapCSErr)
    - internal/chirpstack/device.go (appended ActivateDevice + GetDeviceKeys + GetDeviceActivation)
    - internal/chirpstack/device_test.go (appended 4 new tests pinning the 1.0.x 3-key copy invariant)
    - internal/chirpstack/gateway_test.go (replaced Wave 0 t.Skip skeletons)
    - internal/chirpstack/gateway_metrics_cache_test.go (replaced Wave 0 t.Skip skeletons)
    - internal/testsupport/chirpstack_mock_phase3.go (renamed GetDeviceKeys/GetDeviceActivation to proto names GetKeys/GetActivation; added Activate handler)
    - go.mod (golang.org/x/sync promoted to direct)

key-decisions:
  - "ChirpStack v4.17 RPC method is GetMetrics, NOT the obsolete pre-v4 name CONTEXT D-02 still uses. Wrapper method matches proto; cache strategy matches D-02."
  - "ErrNotFound sentinel emitted by wrapCSErr — handlers branch via errors.Is; other gRPC codes flow through wrapped for 5xx mapping."
  - "Location.Source = LocationSource_CONFIG is stamped unconditionally inside CreateGateway/UpdateGateway because Shifter always uses operator-typed lat/lng (D-02)."
  - "MetricsCache key is gatewayID; singleflight key matches. Verified per-gateway-key isolation in TestMetricsCache_PerGatewayKey so a thundering herd for gateway A doesn't block fetches for gateway B."
  - "GetDeviceActivation translates BOTH codes.NotFound and an empty DeviceActivation payload (DevAddr=='') into ErrNotFound — CS v4 can surface either depending on whether the device row exists."

patterns-established:
  - "Wrapper pattern: typed input struct + named return + wrapCSErr — see CreateGatewayInput / ListGatewaysInput / GetGatewayMetricsInput / ActivateDeviceInput"
  - "Single-flight TTL cache: sync.RWMutex map + singleflight.Group + injectable clock — reusable for any per-key external fetch with bounded freshness"
  - "1.0.x 3-key copy: enforced once at the boundary (ActivateDevice), surfaced once on the way out (DeviceActivation.NwkSKey reads NwkSEncKey)"

requirements-completed: [GW-01, GW-02, GW-03, CHIRP-05, CHIRP-06]

# Metrics
duration: 7 min
completed: 2026-05-11
---

# Phase 3 Plan 03: ChirpStack v4 RPC wrappers + GetGatewayMetrics cache Summary

**6 GatewayService RPC wrappers (Create/Get/Update/Delete/List/GetMetrics), 3 DeviceService wrappers (Activate/GetKeys/GetActivation), a 1-minute TTL + singleflight metrics cache, and the typed ErrNotFound sentinel that unifies CS NotFound translation across the whole chirpstack package.**

## Performance

- **Duration:** 7 min
- **Started:** 2026-05-11T06:33:20Z
- **Completed:** 2026-05-11T06:40:38Z
- **Tasks:** 3 (TDD: tests + implementation per task)
- **Files modified:** 7 (2 created, 5 modified)
- **Tests added:** 16 (8 gateway, 4 metrics-cache, 4 device — all passing under `-race`)

## Accomplishments

- All 6 ChirpStack v4.17 GatewayService RPCs wrapped with idiomatic Go signatures matching Phase 2's `device.go` style — every handler in subsequent Phase 3 waves can rely on a stable `*chirpstack.Client` surface for gateway CRUD without importing `api.*` directly.
- Typed `ErrNotFound` sentinel + `wrapCSErr` helper landed; handlers branch with `errors.Is(err, chirpstack.ErrNotFound)` for 404/409 vs 5xx decisions (D-27 reveal-secrets endpoint depends on this).
- `MetricsCache` — 1-minute TTL + `singleflight.Group` deduplication — wired with an injectable clock so tests verify TTL boundaries without `time.Sleep`. 10-goroutine concurrent-Get fan-out produces exactly one underlying RPC (T-3-23 thundering-herd mitigation verified).
- ABP path (`ActivateDevice`) and reveal-secrets paths (`GetDeviceKeys` for OTAA, `GetDeviceActivation` for ABP) wrapped with the 1.0.x 3-key copy invariant enforced once at the boundary (`NwkSEncKey = SNwkSIntKey = FNwkSIntKey = NwkSKey`) — confirmed via direct mock-readback assertion in `TestActivateDevice`.

## Task Commits

Each task was committed atomically:

1. **Task 1: GatewayService 6-RPC wrapper + ErrNotFound sentinel** — `4398d5f` (feat)
2. **Task 2: MetricsCache (1-min TTL + singleflight)** — `f8c314f` (feat)
3. **Task 3: DeviceService Activate + GetKeys + GetActivation** — `cf23c3d` (feat)

## Files Created/Modified

- `internal/chirpstack/gateway.go` — 6 RPC wrappers + helpers (`gatewayProtoFromInput`, `gatewayProtoToDomain`, `listItemToDomain`, `gatewayStateString`, `parseGatewayOrderBy`, `parseAggregation`)
- `internal/chirpstack/gateway_metrics_cache.go` — `MetricsCache` struct + `NewMetricsCache` + `Get` + `Invalidate`
- `internal/chirpstack/errors.go` — added `ErrNotFound` sentinel + `wrapCSErr` translation helper
- `internal/chirpstack/device.go` — appended `ActivateDeviceInput`/`ActivateDevice`, `DeviceKeys`/`GetDeviceKeys`, `DeviceActivation`/`GetDeviceActivation`
- `internal/chirpstack/gateway_test.go` — 8 tests against the Phase 3 bufconn mock (replaced t.Skip skeletons)
- `internal/chirpstack/gateway_metrics_cache_test.go` — 4 tests pinning TTL / single-flight / per-key isolation / Invalidate (replaced t.Skip skeletons)
- `internal/chirpstack/device_test.go` — 4 new tests pinning the 1.0.x 3-key copy invariant + ErrNotFound translation for the reveal paths
- `internal/testsupport/chirpstack_mock_phase3.go` — renamed `GetDeviceKeys`/`GetDeviceActivation` mock methods to proto names `GetKeys`/`GetActivation`; added `Activate` handler that records the activation for follow-up `GetActivation` reads
- `go.mod` — promoted `golang.org/x/sync` from indirect to direct

## Public API Signatures

### Gateway wrappers (`internal/chirpstack/gateway.go`)
```go
type CreateGatewayInput struct { GatewayID, Name, Description, Region string; Lat, Lng, Altitude float64; Tags map[string]string; TenantID string }
type Gateway struct { GatewayID, Name, Description string; Lat, Lng, Altitude float64; Tags map[string]string; LastSeenAt *time.Time; State string; CreatedAt, UpdatedAt time.Time }
type ListGatewaysInput struct { TenantID, Search string; Limit, Offset uint32; OrderBy string; OrderByDesc bool }
type ListGatewaysOutput struct { TotalCount uint32; Items []Gateway }
type GetGatewayMetricsInput struct { GatewayID string; Start, End time.Time; Aggregation string }

func (c *Client) CreateGateway(ctx, CreateGatewayInput) error
func (c *Client) GetGateway(ctx, gatewayID string) (*Gateway, error)
func (c *Client) UpdateGateway(ctx, CreateGatewayInput) error
func (c *Client) DeleteGateway(ctx, gatewayID string) error
func (c *Client) ListGateways(ctx, ListGatewaysInput) (*ListGatewaysOutput, error)
func (c *Client) GetMetrics(ctx, GetGatewayMetricsInput) (*api.GetGatewayMetricsResponse, error)
```

### Metrics cache (`internal/chirpstack/gateway_metrics_cache.go`)
```go
type MetricsCache struct { /* unexported */ }
func NewMetricsCache(client *Client) *MetricsCache
func (m *MetricsCache) Get(ctx, gatewayID string) (*api.GetGatewayMetricsResponse, error)
func (m *MetricsCache) Invalidate(gatewayID string)
```

Observable behavior:
- **TTL:** 1 minute (constant `defaultMetricsTTL`). Within TTL → cache hit, no RPC. Past TTL → fresh fetch, cache updated.
- **Single-flight invariant:** N concurrent `Get` calls for the same `gatewayID` produce exactly 1 underlying RPC. Different `gatewayID`s proceed independently.
- **Invalidation hook:** `Invalidate(gatewayID)` drops the entry so the next `Get` bypasses the freshness check. Used by D-30 decommission / D-32 restore.

### Device extensions (`internal/chirpstack/device.go`)
```go
type ActivateDeviceInput struct { DevEUI, DevAddr, NwkSKey, AppSKey string; FCntUp, FCntDown uint32 }
type DeviceKeys struct { DevEUI, AppKey, NwkKey string }
type DeviceActivation struct { DevEUI, DevAddr, NwkSKey, AppSKey string; FCntUp, NFCntDown, AFCntDown uint32 }

func (c *Client) ActivateDevice(ctx, ActivateDeviceInput) error
func (c *Client) GetDeviceKeys(ctx, devEUI string) (*DeviceKeys, error)
func (c *Client) GetDeviceActivation(ctx, devEUI string) (*DeviceActivation, error)
```

### Error contract (`internal/chirpstack/errors.go`)
```go
var ErrNotFound = errors.New("chirpstack: not found")

// wrapCSErr translates:
//   nil               → nil
//   codes.NotFound    → ErrNotFound (wrapped, with original status message)
//   anything else     → fmt.Errorf-wrapped error preserving the cause for 5xx mapping
func wrapCSErr(err error) error
```

## CONTEXT vs Proto Correction

| Source | Method name | Status |
|--------|-------------|--------|
| CONTEXT D-02 (Phase 3 design doc) | `GetGatewayStats` | Obsolete pre-v4 name |
| chirpstack-api/go/v4@v4.17.0 proto | `GetMetrics` | Authoritative — wrappers use this |

**Resolution:** wrapper method name is `GetMetrics` (matches proto). The CACHE STRATEGY remains exactly as CONTEXT D-02 specifies (1-minute TTL, 24h window, hourly buckets) — only the underlying RPC name is corrected.

## LoRaWAN 1.0.x 3-Key Copy Convention

| Field on the wire (`api.DeviceActivation`) | 1.0.x source | Enforcement site |
|---|---|---|
| `NwkSEncKey` | `ActivateDeviceInput.NwkSKey` | `ActivateDevice` (line ~150 of `device.go`) |
| `SNwkSIntKey` | `ActivateDeviceInput.NwkSKey` | `ActivateDevice` |
| `FNwkSIntKey` | `ActivateDeviceInput.NwkSKey` | `ActivateDevice` |

Invariant pinned by `TestActivateDevice` which reads the activation back via the proto-level API and asserts all three keys are byte-equal to the input `NwkSKey`. On the reveal path, `GetDeviceActivation` surfaces `NwkSEncKey` as `NwkSKey` (the canonical 1.0.x label) on the domain struct.

## ErrNotFound Sentinel Translation Matrix

| Source code from CS | Wrapper return | Handler decision |
|---|---|---|
| `codes.OK` (with non-empty body) | typed domain struct, nil error | 200 + payload |
| `codes.OK` (with empty `DeviceActivation`, `DevAddr==""`) | `nil, ErrNotFound` | 409 no_credentials (D-27) |
| `codes.NotFound` | `nil, wrapped ErrNotFound` (errors.Is matches) | 404 / 409 depending on context |
| `codes.Unavailable` | `nil, fmt.Errorf("chirpstack: %w", err)` (NOT ErrNotFound) | 502 upstream |
| `codes.Unknown` | `nil, fmt.Errorf("chirpstack: %w", err)` (NOT ErrNotFound) | 502 upstream |
| `codes.DeadlineExceeded` | `nil, fmt.Errorf("chirpstack: %w", err)` (NOT ErrNotFound) | 504 upstream |

## Decisions Made

- **Method name `GetMetrics`, not `GetGatewayStats`** — Plan called it out explicitly; proto verified at `api/go/v4@v4.17.0/api/gateway_grpc.pb.go` line 29 (`GatewayService_GetMetrics_FullMethodName = "/api.GatewayService/GetMetrics"`). CONTEXT D-02 is documentation-stale; the cache strategy it specifies is unaffected.
- **`Location.Source = CONFIG` is unconditional** — Shifter never trusts GPS uplinks from a packet forwarder for gateway placement. CONFIG also prevents CS from running its "merge with GPS source" reconciliation against operator-typed coords.
- **Cache key is plain `gatewayID`, NOT a hash of (gatewayID, start, end, aggregation)** — D-02 only ever asks for the canonical 24h/HOUR shape, so the additional dimensions are constants from the cache's point of view. A future plan that needs arbitrary windows can re-key the cache; for now the simpler shape is correct.
- **`GetDeviceActivation` folds empty payload into ErrNotFound** — CS v4 occasionally returns OK with `DeviceActivation == nil` or `DevAddr == ""` for a device that exists but has no active session. Handler should treat that identically to codes.NotFound; folding at the wrapper boundary saves every handler from re-implementing the check.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Phase 3 mock method names were not proto-compliant**
- **Found during:** Task 3 (TestGetDeviceKeys / TestGetDeviceActivation initial run)
- **Issue:** Wave 0's `internal/testsupport/chirpstack_mock_phase3.go` named the device reveal mock methods `GetDeviceKeys` and `GetDeviceActivation`, but the gRPC proto requires methods named `GetKeys` and `GetActivation` on `DeviceServiceServer`. The Go method set would silently fall through to the embedded `UnimplementedDeviceServiceServer`, returning `codes.Unimplemented` for every call — the production tests would have failed with no signal beyond "method not implemented". Additionally, no `Activate` handler existed at all.
- **Fix:** Renamed the methods to `GetKeys` and `GetActivation`. Added a new `Activate` handler that stores the payload via the existing `deviceRevealState` sidecar so a follow-up `GetActivation` reads the activation back.
- **Files modified:** `internal/testsupport/chirpstack_mock_phase3.go`
- **Verification:** All 4 new Task 3 tests pass; full chirpstack suite (54 tests) green under `-race`.
- **Committed in:** `cf23c3d` (Task 3 commit)

**2. [Rule 1 - Bug] Plan's `DeviceKeys.JoinNonce` field doesn't exist in v4.17 proto**
- **Found during:** Task 3 (writing the `DeviceKeys` domain struct)
- **Issue:** Plan's `<interfaces>` block lists `JoinNonce uint32` on `api.DeviceKeys`. Verified directly against `~/go/pkg/mod/github.com/chirpstack/chirpstack/api/go/v4@v4.17.0/api/device.pb.go`: the proto `DeviceKeys` message has `dev_eui`, `nwk_key`, `app_key`, `gen_app_key` — no `join_nonce` field. Listing it on the domain struct would either fail compilation or silently store a zero value.
- **Fix:** Omitted `JoinNonce` from the `DeviceKeys` domain struct. Added a comment noting the proto omission. The reveal handler doesn't need it (D-27 returns AppKey + NwkKey only).
- **Files modified:** `internal/chirpstack/device.go`
- **Verification:** `go vet` clean; `TestGetDeviceKeys` asserts the populated fields (DevEUI, AppKey, NwkKey) match the mock-seeded values.
- **Committed in:** `cf23c3d` (Task 3 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug).
**Impact on plan:** Both deviations were narrow corrections — neither expanded scope, both were necessary for the planned tests to validate the planned behavior. The fix to the testsupport mock was a Wave 0 oversight (proto-name mismatch); the JoinNonce omission was an interfaces-block transcription drift from CONTEXT to plan.

## Authentication Gates

None — local unit tests against in-process bufconn mocks; no external service contact required.

## Issues Encountered

None — every task's `<verify>` block exited 0 on first attempt after the deviations above were addressed inline.

## User Setup Required

None — no external service configuration introduced. `golang.org/x/sync` was already an indirect dependency (no new module download needed for downstream installers).

## Self-Check: PASSED

- `internal/chirpstack/gateway.go` — FOUND
- `internal/chirpstack/gateway_metrics_cache.go` — FOUND
- `internal/chirpstack/errors.go` — FOUND (modified)
- `internal/chirpstack/device.go` — FOUND (modified)
- `internal/chirpstack/gateway_test.go` — FOUND (rewritten)
- `internal/chirpstack/gateway_metrics_cache_test.go` — FOUND (rewritten)
- `internal/chirpstack/device_test.go` — FOUND (extended)
- `internal/testsupport/chirpstack_mock_phase3.go` — FOUND (modified)
- Task 1 commit `4398d5f` — FOUND in `git log`
- Task 2 commit `f8c314f` — FOUND in `git log`
- Task 3 commit `cf23c3d` — FOUND in `git log`
- `go test ./internal/chirpstack -count=1 -race -timeout 120s` — 54 PASS, 0 FAIL
- `go vet ./internal/chirpstack/...` — No issues found
- `go build ./...` — Success
- Acceptance grep checks — all 7 of Task 1, all 4 of Task 2, all 6 of Task 3 — PASS
- "GetGatewayStats" not present in `internal/chirpstack/gateway.go` — confirmed absent

## Next Phase Readiness

- Wave 3 (gateway list/CRUD handlers — 03-04 onwards) can now consume `*chirpstack.Client` for all gateway operations without importing `api/go/v4`.
- The reveal-secrets endpoint (D-27, scheduled for Wave 3) has `GetDeviceKeys` / `GetDeviceActivation` ready; the handler only needs to layer the audit-write-before-return invariant on top.
- The ABP add-device path (Wave 3) has `ActivateDevice` ready; the 1.0.x 3-key copy is enforced inside the wrapper, so the handler can pass operator-typed `NwkSKey` once and trust the conversion.
- `MetricsCache` is ready to wire into the gateway list handler (Wave 3) — instantiate once at `serve` startup, attach to the handler, call `cache.Get(ctx, gw.GatewayID)` per row in the list response.

---
*Phase: 03-provisioning-gateways-devices-bulk-import*
*Completed: 2026-05-11*
