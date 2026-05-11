---
phase: 05-aggregates-reports-map-floor-plans
plan: 07
type: execute
wave: 3
depends_on: [05]
files_modified:
  - internal/db/queries/floor_plan.sql
  - internal/db/sqlc/floor_plan.sql.go
  - internal/floorplan/placement.go
  - internal/floorplan/placement_test.go
  - internal/floorplan/static.go
  - internal/http/floorplan_static_test.go
  - internal/floorplan/routes.go
  - internal/device/handlers.go
  - internal/device/decommission_recovery_test.go
  - internal/audit/log.go
autonomous: true
requirements: [SITE-04, SITE-06]
threat_refs: [T-05-07-01, T-05-07-02, T-05-07-03]

must_haves:
  truths:
    - "POST /api/floor-plans/:id/placements accepts {device_id, x_frac, y_frac} and INSERTs (or UPSERTs) device_floor_plan_placement"
    - "PATCH /api/floor-plans/:id/placements/:device_id updates x_frac/y_frac for an existing placement (drag-to-nudge)"
    - "DELETE /api/floor-plans/:id/placements/:device_id removes placement (right-click → Remove from plan)"
    - "GET /api/floor-plans/:id/placements lists placements with device name + utility_class + last_seen_at + battery_pct + rssi (for client-side D-22 health state computation)"
    - "GET /api/floor-plans/:id/image streams the image file with auth gating — UUID validated, path composed server-side only"
    - "Server rejects placements where the device's site_id does not match the floor_plan's site_id (data integrity guard)"
    - "Device soft-delete (decommission) DELETEs device_floor_plan_placement row in the SAME pgx.Tx as the device UPDATE per D-25"
    - "Audit log entry written inside the same pgx.Tx as every placement INSERT/UPDATE/DELETE and as part of the decommission cascade"
    - "Anonymous GET /api/floor-plans/:id/image returns 401 (not the image bytes)"
  artifacts:
    - path: "internal/db/queries/floor_plan.sql"
      provides: "UpsertPlacement, UpdatePlacement, DeletePlacement, ListPlacementsByPlan, GetPlacementByDevice, DeletePlacementByDevice (decommission helper)"
      contains: "name: UpsertPlacement"
    - path: "internal/floorplan/placement.go"
      provides: "PlacementHandlers — Upsert/Update/Delete/List/Get + site_id integrity check + audit-in-tx"
      contains: "func UpsertPlacementHandler"
    - path: "internal/floorplan/static.go"
      provides: "ServeImage handler — UUID validation, content-type sniffing, auth gating; serves <ImageRoot>/<image_path>"
      contains: "func ServeImageHandler"
    - path: "internal/device/handlers.go"
      provides: "Decommission handler extension — DeletePlacementByDevice + audit entry inside existing decommission txn (D-25)"
      contains: "DeletePlacementByDevice"
  key_links:
    - from: "internal/floorplan/placement.go"
      to: "device_floor_plan_placement table (sqlc Upsert/Update/Delete)"
      via: "WithTx-bound queries inside placement handler txn"
      pattern: "q\\.(Upsert|Update|Delete)Placement"
    - from: "internal/device/handlers.go DecommissionHandler"
      to: "q.DeletePlacementByDevice(ctx, deviceID)"
      via: "same pgx.Tx as device.DecommissionDevice — D-25 atomicity"
      pattern: "DeletePlacementByDevice"
    - from: "internal/floorplan/static.go"
      to: "<ImageRoot>/<floor_plan.image_path>"
      via: "filepath.Join with UUID-validated id → DB lookup → image_path"
      pattern: "filepath\\.Join\\(deps\\.ImageRoot"
---

<objective>
Complete the floor-plan server stack with placement CRUD (SITE-04 fractional coords with same-site integrity guard), the auth-gated static image serve handler, and the decommission integration (D-25: soft-delete a device → delete its placement in the same transaction). Plan 05-05 shipped the schema + upload validators; this plan adds the operations that the canvas UI in plan 05-10 will call.

Purpose: Floor-plan placements are the load-bearing "fractional coords survive image replacement" mechanism. The site_id integrity guard prevents a stray UUID from binding a device on Site A to a plan belonging to Site B. The decommission integration closes the data-integrity loop so retired devices don't leave ghost pins behind.

Output: 6 new sqlc queries, 1 placement handler file, 1 static serve handler file, decommission extension to device handler, audit constants extended.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-05-floor-plan-schema-upload-PLAN.md
@internal/audit/log.go
@internal/device/handlers.go
@internal/floorplan/handlers.go
@internal/db/queries/floor_plan.sql
@internal/db/queries/devices.sql

<interfaces>
<!-- From plan 05-05: floor_plan + device_floor_plan_placement schema already exists. -->
<!-- This plan adds 6 new sqlc queries and extends the device decommission handler. -->

<!-- New audit constants -->
```go
const (
    ActionPlacementPin    = "placement.pin"
    ActionPlacementNudge  = "placement.nudge"
    ActionPlacementRemove = "placement.remove"
    EntityTypePlacement   = "placement"
)
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Placement sqlc queries + handlers (Upsert/Update/Delete/List) with same-site integrity guard</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-20 §D-21 §D-22
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Floor Plan — Canvas Coordinate Contract §Interaction States — Floor Plan Pinning Flow
    - internal/db/migrations/0032_device_floor_plan_placement.up.sql (CHECK x_frac/y_frac in [0,1])
    - internal/floorplan/handlers.go (existing chi pattern + Deps shape from plan 05-05)
    - internal/audit/log.go (existing WriteEntry + audit vocabulary)
  </read_first>
  <behavior>
    - Test 1: POST placement with x_frac=0.5 y_frac=0.5 → 201 + placement JSON
    - Test 2: POST placement with x_frac=1.5 → 422 (DB CHECK rejects; handler maps to 422)
    - Test 3: POST placement where device.site_id != floor_plan.site_id → 409 "site_mismatch"
    - Test 4: POST placement for device already pinned to another plan → device moves to new plan (UPSERT on device_id PK)
    - Test 5: PATCH placement nudge persists new x_frac/y_frac and writes audit entry with before/after
    - Test 6: DELETE placement removes row + writes audit entry
    - Test 7: GET /api/floor-plans/:id/placements returns rows joined with device.name, utility_class, last_seen_at, battery_pct, rssi (denormalized for D-22 client-side health computation)
  </behavior>
  <action>
**Step A — Extend `internal/db/queries/floor_plan.sql` with 6 new queries:**

```sql
-- name: UpsertPlacement :one
-- INSERT or UPDATE — device_id is PK (a device pins to one plan at a time).
INSERT INTO device_floor_plan_placement (device_id, floor_plan_id, x_frac, y_frac)
VALUES ($1, $2, $3, $4)
ON CONFLICT (device_id) DO UPDATE
SET floor_plan_id = EXCLUDED.floor_plan_id,
    x_frac        = EXCLUDED.x_frac,
    y_frac        = EXCLUDED.y_frac
RETURNING *;

-- name: UpdatePlacement :one
-- Drag-to-nudge: only x_frac / y_frac change; floor_plan_id stays.
UPDATE device_floor_plan_placement
SET x_frac = $2, y_frac = $3
WHERE device_id = $1
RETURNING *;

-- name: DeletePlacementByDevice :exec
-- Called by both right-click remove AND device decommission (D-25).
DELETE FROM device_floor_plan_placement WHERE device_id = $1;

-- name: ListPlacementsByPlan :many
-- Returns placements joined with the device + device_profile data needed for
-- client-side D-22 health computation (state colors) without a follow-up call.
SELECT
  p.device_id,
  p.floor_plan_id,
  p.x_frac,
  p.y_frac,
  p.created_at,
  d.name           AS device_name,
  d.last_seen_at,
  d.battery_pct,
  d.rssi,
  mp.id            AS metering_point_id,
  mp.utility_class,
  dp.expected_interval_s
FROM device_floor_plan_placement p
JOIN device d        ON d.id = p.device_id
LEFT JOIN binding b  ON b.device_id = d.id AND b.valid_to IS NULL
LEFT JOIN metering_point mp ON mp.id = b.metering_point_id
JOIN device_profile dp ON dp.id = d.device_profile_id
WHERE p.floor_plan_id = $1
  AND d.decommissioned_at IS NULL;

-- name: GetPlacementByDevice :one
SELECT * FROM device_floor_plan_placement WHERE device_id = $1;

-- name: GetDeviceSiteID :one
-- Helper for the same-site integrity check: returns the device's site via
-- its active binding's metering_point.
SELECT mp.site_id
FROM device d
LEFT JOIN binding b ON b.device_id = d.id AND b.valid_to IS NULL
LEFT JOIN metering_point mp ON mp.id = b.metering_point_id
WHERE d.id = $1
  AND d.decommissioned_at IS NULL;
```

**Caveat:** `device` doesn't have a direct site_id column — it's derived through the active binding → metering_point. The `GetDeviceSiteID` query above models this. If the device has no active binding, `site_id` is NULL → handler treats that as "cannot pin an unbound device" and returns 422.

`just sqlc`.

**Step B — Audit constants in `internal/audit/log.go`:**

```go
const (
    ActionPlacementPin    = "placement.pin"
    ActionPlacementNudge  = "placement.nudge"
    ActionPlacementRemove = "placement.remove"
    EntityTypePlacement   = "placement"
)
```

Update vocab CHECK constraint via new migration `0033_audit_vocab_phase5.up.sql` (folds in plan 05-05's floor-plan vocab AND this plan's placement vocab AND plan 05-03's report vocab; if 05-05 already shipped this migration, append-edit it rather than creating 0034).

**Step C — `internal/floorplan/placement.go`:**

```go
package floorplan

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"

    sqlc "shifter/internal/db/sqlc"
    "shifter/internal/audit"
    "shifter/internal/auth"
)

type PinRequest struct {
    DeviceID string  `json:"device_id"`
    XFrac    float32 `json:"x_frac"`
    YFrac    float32 `json:"y_frac"`
}

// UpsertPlacementHandler — POST /api/floor-plans/{id}/placements
// Same-site integrity guard: returns 409 site_mismatch if device's site
// (via active binding → metering_point.site_id) ≠ floor_plan.site_id.
func UpsertPlacementHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, _ := auth.UserFromCtx(r.Context())
        planID, err := uuid.Parse(chi.URLParam(r, "id"))
        if err != nil { writeError(w, 400, "invalid_plan_id"); return }

        var req PinRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeError(w, 400, "invalid_json"); return
        }
        deviceID, err := uuid.Parse(req.DeviceID)
        if err != nil { writeError(w, 400, "invalid_device_id"); return }
        // Defense-in-depth: clamp to [0,1] even though DB CHECK enforces it.
        if req.XFrac < 0 || req.XFrac > 1 || req.YFrac < 0 || req.YFrac > 1 {
            writeError(w, 422, "fraction_out_of_range"); return
        }

        tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
        if err != nil { writeError(w, 500, "tx_begin"); return }
        defer tx.Rollback(r.Context())
        q := deps.Queries.WithTx(tx)

        // Same-site integrity check.
        plan, err := q.GetFloorPlan(r.Context(), planID)
        if errors.Is(err, pgx.ErrNoRows) { writeError(w, 404, "plan_not_found"); return }
        if err != nil { writeError(w, 500, "plan_load_failed"); return }

        deviceSiteID, err := q.GetDeviceSiteID(r.Context(), deviceID)
        if err != nil { writeError(w, 404, "device_not_found"); return }
        if !deviceSiteID.Valid {
            writeError(w, 422, "device_unbound"); return
        }
        if uuid.UUID(deviceSiteID.Bytes) != uuid.UUID(plan.SiteID.Bytes) {
            writeError(w, 409, "site_mismatch"); return
        }

        // Capture "before" state for audit (might be empty = first pin).
        before, _ := q.GetPlacementByDevice(r.Context(), deviceID)
        action := audit.ActionPlacementPin
        if before.DeviceID != uuid.Nil {
            // existing placement → this is a nudge or a re-pin
            action = audit.ActionPlacementNudge
        }

        placement, err := q.UpsertPlacement(r.Context(), sqlc.UpsertPlacementParams{
            DeviceID: deviceID, FloorPlanID: planID, XFrac: req.XFrac, YFrac: req.YFrac,
        })
        if err != nil {
            // CHECK violation on x_frac/y_frac → 422 (defense in depth — handler already clamped).
            if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23514" {
                writeError(w, 422, "fraction_out_of_range")
                return
            }
            writeError(w, 500, "upsert_failed"); return
        }

        beforeMap := map[string]any{}
        if before.DeviceID != uuid.Nil {
            beforeMap = map[string]any{"floor_plan_id": before.FloorPlanID, "x_frac": before.XFrac, "y_frac": before.YFrac}
        }
        if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
            UserID: user.ID, Action: action,
            EntityType: audit.EntityTypePlacement, EntityID: deviceID,
            Before: beforeMap,
            After: map[string]any{"floor_plan_id": planID, "x_frac": req.XFrac, "y_frac": req.YFrac},
            RequestID: middleware.GetReqID(r.Context()),
        }); err != nil { writeError(w, 500, "audit_failed"); return }

        if err := tx.Commit(r.Context()); err != nil { writeError(w, 500, "tx_commit"); return }
        writeJSON(w, http.StatusCreated, placement)
    }
}

// UpdatePlacementHandler — PATCH /api/floor-plans/{id}/placements/{deviceID}
// Drag-to-nudge. Same audit-in-tx pattern; action = placement.nudge.

// DeletePlacementHandler — DELETE /api/floor-plans/{id}/placements/{deviceID}
// Same audit-in-tx; action = placement.remove.

// ListPlacementsHandler — GET /api/floor-plans/{id}/placements
// Returns denormalized rows for client-side D-22 health computation.
```

**Step D — `internal/floorplan/routes.go` (extend the existing RegisterRoutes from plan 05-05):**

```go
func RegisterRoutes(r chi.Router, deps Deps) {
    // From plan 05-05 (re-stated for clarity):
    r.Post("/api/sites/{siteID}/floor-plans", UploadImageHandler(deps))
    r.Get("/api/sites/{siteID}/floor-plans", ListBySiteHandler(deps))
    r.Get("/api/floor-plans/{id}", GetHandler(deps))
    r.Patch("/api/floor-plans/{id}", ReplaceImageHandler(deps))
    r.Patch("/api/floor-plans/{id}/label", RenameHandler(deps))
    r.Delete("/api/floor-plans/{id}", DeleteHandler(deps))

    // New in plan 05-07:
    r.Post("/api/floor-plans/{id}/placements", UpsertPlacementHandler(deps))
    r.Patch("/api/floor-plans/{id}/placements/{deviceID}", UpdatePlacementHandler(deps))
    r.Delete("/api/floor-plans/{id}/placements/{deviceID}", DeletePlacementHandler(deps))
    r.Get("/api/floor-plans/{id}/placements", ListPlacementsHandler(deps))
    r.Get("/api/floor-plans/{id}/image", ServeImageHandler(deps))  // Task 2
}
```

**Step E — Replace `t.Skip` in `internal/floorplan/placement_test.go`:**

Minimum cases:

1. `TestPlacementCRUD` — POST/PATCH/DELETE happy path + DB row + audit entries
2. `TestPlacement_RejectsFractionOutOfRange` — x_frac=1.5 → 422
3. `TestPlacement_RejectsSiteMismatch` — device on Site A, plan on Site B → 409
4. `TestPlacement_RejectsUnboundDevice` — device with no active binding → 422
5. `TestPlacement_UpsertMovesPin` — first POST pins to plan1, second POST to plan2 → device row exists once with floor_plan_id=plan2; audit shows action=placement.nudge for the second call
6. `TestListPlacements_DenormalizedFields` — response rows include device_name, utility_class, last_seen_at, battery_pct, rssi
7. `TestPlacement_DBCheckFiresEvenIfHandlerSkipped` — direct DB test (defense-in-depth)
  </action>
  <verify>
    <automated>just sqlc &amp;&amp; go test ./internal/floorplan/... -race -count=1 -short=false -run "TestPlacementCRUD|TestPlacement_Rejects|TestPlacement_Upsert|TestListPlacements"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/queries/floor_plan.sql` contains all 6 new query names listed in artifacts
    - `internal/floorplan/placement.go` exports `UpsertPlacementHandler`, `UpdatePlacementHandler`, `DeletePlacementHandler`, `ListPlacementsHandler`
    - Same-site integrity guard implemented: `internal/floorplan/placement.go` contains literal `site_mismatch`
    - `internal/audit/log.go` contains literal `ActionPlacementPin = "placement.pin"`, `ActionPlacementNudge = "placement.nudge"`, `ActionPlacementRemove = "placement.remove"`, `EntityTypePlacement = "placement"`
    - At least 7 named test cases in `placement_test.go` covering happy CRUD + out-of-range + site-mismatch + unbound device + move pin + denormalized list + DB CHECK
    - `TestPlacement_RejectsSiteMismatch` asserts response code 409 and body contains `site_mismatch`
    - `go test ./internal/floorplan/... -short=false` exits 0
  </acceptance_criteria>
  <done>Placement CRUD + same-site integrity + audit-in-tx live.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Auth-gated static image serve + decommission integration (D-25)</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-18 §D-25
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Security Domain
    - internal/floorplan/handlers.go (Deps.ImageRoot constant)
    - internal/device/handlers.go (existing decommission handler — context lines 1000–1052)
    - internal/device/decommission_recovery_test.go (existing test fixture pattern)
  </read_first>
  <behavior>
    - Test 1: GET /api/floor-plans/:id/image with valid auth session → 200 + image bytes + correct Content-Type (image/png or image/jpeg)
    - Test 2: GET /api/floor-plans/:id/image with no session cookie → 401
    - Test 3: GET with non-UUID id → 400 BEFORE any filesystem touch
    - Test 4: GET with valid UUID for a non-existent plan → 404 (DB lookup fails before file is opened)
    - Test 5: Path traversal attempt via `/api/floor-plans/..%2Fetc%2Fpasswd/image` → 400 (chi route doesn't even match, but UUID parse rejects)
    - Test 6: Decommissioning a device that has a placement → placement row deleted AND audit entry with action=device.decommission AND another audit entry with action=placement.remove (or before/after diff captures both) inside the SAME tx
    - Test 7: Recovery test — inject post-placement-delete failure → device row unchanged AND placement row still present (full rollback)
  </behavior>
  <action>
**Step A — `internal/floorplan/static.go`:**

```go
package floorplan

import (
    "errors"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"

    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "shifter/internal/auth"
)

// ServeImageHandler — GET /api/floor-plans/{id}/image
//
// Returns the raw image bytes with proper Content-Type. Authenticated path
// (T-05-07-02): anonymous requests are rejected by the chi middleware stack
// that mounts this route BEFORE the handler runs. This handler additionally
// re-asserts via auth.UserFromCtx to defend against routing misconfig.
//
// Path traversal mitigation (T-05-07-03): the `id` URL param is UUID-parsed
// BEFORE any filepath operation. The image_path column stores a server-
// generated relative path (`<uuid>.<ext>` per plan 05-05) — no client bytes
// reach the filesystem call. We additionally call filepath.Clean and verify
// the resolved path begins with deps.ImageRoot.
func ServeImageHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if _, ok := auth.UserFromCtx(r.Context()); !ok {
            writeError(w, 401, "unauthorized"); return
        }

        id, err := uuid.Parse(chi.URLParam(r, "id"))
        if err != nil { writeError(w, 400, "invalid_id"); return }

        plan, err := deps.Queries.GetFloorPlan(r.Context(), id)
        if errors.Is(err, pgx.ErrNoRows) { writeError(w, 404, "plan_not_found"); return }
        if err != nil { writeError(w, 500, "plan_lookup_failed"); return }

        // Defense-in-depth: compose absolute path then verify containment.
        absPath := filepath.Join(deps.ImageRoot, plan.ImagePath)
        cleaned := filepath.Clean(absPath)
        if !strings.HasPrefix(cleaned, filepath.Clean(deps.ImageRoot)+string(filepath.Separator)) {
            // Should never happen — image_path is server-generated UUID.<ext>.
            // Defensive: T-05-07-03 catch-all.
            writeError(w, 400, "invalid_image_path"); return
        }

        f, err := os.Open(cleaned)
        if err != nil { writeError(w, 404, "image_missing"); return }
        defer f.Close()

        // Sniff Content-Type from first 512 bytes for safety (validated on upload too).
        sniffBuf := make([]byte, 512)
        n, _ := f.Read(sniffBuf)
        sniffBuf = sniffBuf[:n]
        ct := http.DetectContentType(sniffBuf)
        switch ct {
        case "image/png", "image/jpeg":
            w.Header().Set("Content-Type", ct)
        default:
            writeError(w, 500, "bad_content_type")
            return
        }

        w.Header().Set("Cache-Control", "private, max-age=600")  // 10-min client cache; revalidate
        if _, err := f.Seek(0, 0); err != nil { writeError(w, 500, "seek_failed"); return }
        _, _ = io.Copy(w, f)
    }
}
```

**Step B — `internal/http/floorplan_static_test.go` (replace skeleton):**

```go
package http_test

func TestFloorPlanImageAuthGated(t *testing.T) {
    // 1) Upload a floor plan via the API (or seed DB directly + write file to ImageRoot).
    // 2) GET /api/floor-plans/<id>/image with no session → 401
    // 3) GET with valid session → 200 + Content-Type image/png + body matches uploaded bytes
    // 4) GET with non-UUID id → 400
    // 5) GET with valid UUID for non-existent plan → 404
    require.Equal(t, http.StatusUnauthorized, anonResp.StatusCode)
    require.Equal(t, http.StatusOK, authResp.StatusCode)
    require.Equal(t, "image/png", authResp.Header.Get("Content-Type"))
    require.Equal(t, http.StatusBadRequest, invalidIDResp.StatusCode)
    require.Equal(t, http.StatusNotFound, missingPlanResp.StatusCode)
}

func TestFloorPlanImageAuthGated_PathTraversalRejected(t *testing.T) {
    // chi routes will not even match "../etc/passwd" (not a valid URL segment).
    // But test the defense-in-depth filepath.Clean prefix check: directly seed
    // a floor_plan row with image_path = "../malicious.png" via SQL bypass,
    // then GET → expect 400 invalid_image_path.
    _, err := pool.Exec(ctx, `UPDATE floor_plan SET image_path = '../malicious.png' WHERE id = $1`, planID)
    require.NoError(t, err)
    resp := authGet(t, fmt.Sprintf("/api/floor-plans/%s/image", planID))
    require.Equal(t, http.StatusBadRequest, resp.StatusCode)
    body, _ := io.ReadAll(resp.Body)
    require.Contains(t, string(body), "invalid_image_path")
}
```

**Step C — Extend `internal/device/handlers.go` DecommissionHandler:**

Find the existing decommission body (handlers.go around lines 1000–1052 — `q.DecommissionDevice(r.Context(), pgUUID(id))` then `audit.WriteEntry`). BEFORE the existing audit.WriteEntry call, add the placement deletion:

```go
// D-25: Decommissioned devices auto-remove from any floor plan they were
// pinned to. Same pgx.Tx as the device UPDATE so a rollback restores BOTH.
placementBefore, placementErr := q.GetPlacementByDevice(r.Context(), pgUUID(id))
hadPlacement := placementErr == nil && uuid.UUID(placementBefore.DeviceID.Bytes) != uuid.Nil
if hadPlacement {
    if err := q.DeletePlacementByDevice(r.Context(), pgUUID(id)); err != nil {
        internalError(deps.Log, w, "delete placement on decommission", err)
        return
    }
    // Audit the placement removal as a distinct entry so Phase 6 audit browse
    // surfaces both the decommission and the unpinning.
    if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
        UserID: mustParseUUID(user.ID),
        Action: audit.ActionPlacementRemove,
        EntityType: audit.EntityTypePlacement,
        EntityID:   uuid.UUID(placementBefore.DeviceID.Bytes),
        Before: map[string]any{
            "floor_plan_id": uuid.UUID(placementBefore.FloorPlanID.Bytes),
            "x_frac": placementBefore.XFrac, "y_frac": placementBefore.YFrac,
        },
        After: map[string]any{"reason": "device_decommission"},
        RequestID: middleware.GetReqID(r.Context()),
    }); err != nil {
        internalError(deps.Log, w, "audit placement remove", err)
        return
    }
}
```

Update the existing decommission audit entry's `after` map to record `placement_removed: hadPlacement` so the device audit row also captures the cascade.

**Step D — Extend `internal/device/decommission_recovery_test.go` with two new test cases:**

```go
func TestDecommission_RemovesFloorPlanPlacement(t *testing.T) {
    // Setup: device with active binding + floor_plan + placement.
    // POST /api/devices/{id}/decommission → 200
    // Assert: device.decommissioned_at NOT NULL, placement row DELETED,
    //         two audit rows in DB (device.decommission AND placement.remove).
}

func TestDecommission_PlacementRollsBackOnFailure(t *testing.T) {
    // Setup: device + placement; inject a post-placement-delete tx failure
    // (e.g. mock audit.WriteEntry to return error after first call).
    // Assert: device.decommissioned_at STILL NULL, placement row STILL EXISTS,
    //         zero new audit rows. Full tx rollback.
}

func TestDecommission_NoPlacement_NoOp(t *testing.T) {
    // Pre-existing decommission flow (device with no placement) still works:
    // exactly one audit entry (device.decommission), no placement.remove entry.
}
```
  </action>
  <verify>
    <automated>go test ./internal/floorplan/... ./internal/device/... ./internal/http/... -race -count=1 -short=false -run "TestFloorPlanImageAuthGated|TestDecommission"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/floorplan/static.go` contains literal `auth.UserFromCtx`, `uuid.Parse`, `filepath.Clean`, `strings.HasPrefix`, and `http.DetectContentType`
    - `internal/floorplan/static.go` exports `ServeImageHandler`
    - `internal/device/handlers.go` decommission body contains literal `q.DeletePlacementByDevice(r.Context(), pgUUID(id))` AND literal `audit.ActionPlacementRemove`
    - `internal/device/handlers.go` decommission body places the placement DELETE + placement-remove audit INSIDE the existing pgx.Tx (verified by reading; the deletion and audit must occur before `tx.Commit`)
    - `internal/http/floorplan_static_test.go` contains at minimum `TestFloorPlanImageAuthGated` and `TestFloorPlanImageAuthGated_PathTraversalRejected`
    - `TestFloorPlanImageAuthGated_PathTraversalRejected` body forces `image_path = '../malicious.png'` via direct SQL UPDATE and asserts 400 + `invalid_image_path` in body
    - `internal/device/decommission_recovery_test.go` contains at minimum `TestDecommission_RemovesFloorPlanPlacement`, `TestDecommission_PlacementRollsBackOnFailure`, `TestDecommission_NoPlacement_NoOp`
    - `go test ./internal/{floorplan,device,http}/... -short=false` exits 0
  </acceptance_criteria>
  <done>Static image serve auth-gated + path-traversal-defended; decommission D-25 invariant proven by recovery tests.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Client → POST/PATCH/DELETE placement | JSON body; UUID + fraction validation before binding |
| Client → GET /api/floor-plans/:id/image | Auth-gated; UUID parsed before path lookup |
| Decommission handler → placement DELETE | Same pgx.Tx as the device UPDATE (D-25 atomicity) |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-07-01 | Tampering | Cross-site placement abuse (pinning Site A's device to Site B's plan) | medium | mitigate | `GetDeviceSiteID` query in same tx + handler returns 409 site_mismatch when mismatch detected. Test `TestPlacement_RejectsSiteMismatch` pins. |
| T-05-07-02 | Information Disclosure | Unauthenticated access to floor-plan images | medium | mitigate | Handler asserts `auth.UserFromCtx` AND mounted inside the authenticated chi router group. Test `TestFloorPlanImageAuthGated` asserts 401 for anon GET. |
| T-05-07-03 | Elevation of Privilege | Path traversal via crafted image_path | high | mitigate | image_path is server-generated `<uuid>.<ext>` (plan 05-05 invariant). Static handler additionally applies `filepath.Clean` and `strings.HasPrefix` containment check. Test `TestFloorPlanImageAuthGated_PathTraversalRejected` injects malicious path via direct DB UPDATE and asserts 400. |
</threat_model>

<verification>
1. `go test ./internal/{floorplan,device,http}/... -race -count=1 -short=false` exits 0
2. `grep "DeletePlacementByDevice" internal/device/handlers.go` returns ≥1 match (D-25 integration landed)
3. `grep "site_mismatch" internal/floorplan/placement.go` returns ≥1 match
4. `grep "filepath.Clean" internal/floorplan/static.go` returns ≥1 match
5. `grep -E "ActionPlacement(Pin|Nudge|Remove)" internal/audit/log.go` returns 3 matches
</verification>

<success_criteria>
- Placement CRUD: Upsert/Update/Delete/List with audit-in-tx and same-site integrity guard
- Static image serve: auth-gated + UUID-validated + path-clean containment check
- D-25 invariant: device decommission deletes placement in same tx; rollback test proves atomicity
- New audit vocab entries (placement.{pin,nudge,remove}) added to CHECK constraint
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-07-SUMMARY.md` recording:
- Total placement endpoints shipped
- Whether the audit vocab migration approach (extend 05-05's vocab migration vs new 0033) was taken
- Defense-in-depth verification: DB CHECK fires even when handler is bypassed (TestPlacement_DBCheckFiresEvenIfHandlerSkipped)
- Decommission rollback test outcome (was placement preserved on failure?)
</output>
