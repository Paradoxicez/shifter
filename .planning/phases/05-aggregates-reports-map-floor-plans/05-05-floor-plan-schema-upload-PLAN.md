---
phase: 05-aggregates-reports-map-floor-plans
plan: 05
type: execute
wave: 2
depends_on: [01]
files_modified:
  - internal/db/migrations/0031_floor_plan.up.sql
  - internal/db/migrations/0031_floor_plan.down.sql
  - internal/db/migrations/0032_device_floor_plan_placement.up.sql
  - internal/db/migrations/0032_device_floor_plan_placement.down.sql
  - internal/db/queries/floor_plan.sql
  - internal/db/sqlc/floor_plan.sql.go
  - internal/floorplan/doc.go
  - internal/floorplan/handlers.go
  - internal/floorplan/handlers_test.go
  - internal/floorplan/image.go
  - internal/floorplan/routes.go
  - compose/bundled.yml
  - compose/external.yml
  - internal/audit/log.go
autonomous: true
requirements: [SITE-02, SITE-03]
threat_refs: [T-05-05-01, T-05-05-02, T-05-05-03]

must_haves:
  truths:
    - "floor_plan table exists with columns (id, site_id, label, sort_order, image_path, image_w, image_h, uploaded_at, updated_at) per D-16"
    - "device_floor_plan_placement table exists with (device_id PK, floor_plan_id, x_frac, y_frac CHECK [0,1], created_at)"
    - "Site can have multiple floor_plan rows ordered by sort_order — vertical (multi-floor) layout supported per SITE-02"
    - "POST /api/sites/:id/floor-plans validates: MIME type ∈ {image/png, image/jpeg}; file size ≤ 10MB; dimensions ≤ 8192×8192 (D-19)"
    - "Server rejects PDFs (D-17: PDF conversion is client-side via pdf.js; server NEVER sees PDF bytes)"
    - "Upload writes to /var/lib/shifter/floor-plans/<floor_plan_id>.<ext> (D-18); volume mounted in both compose flavors"
    - "Image dimensions are probed BEFORE writing to disk to prevent disk-exhaustion vector (T-05-05-01)"
    - "PATCH /api/floor-plans/:id replaces image while keeping all device_floor_plan_placement rows intact (D-24)"
    - "DELETE /api/floor-plans/:id cascades to delete placements + audit row written in same tx"
    - "Audit log entry written inside the same pgx.Tx as every floor_plan + placement mutation (D-23 invariant)"
  artifacts:
    - path: "internal/db/migrations/0031_floor_plan.up.sql"
      provides: "floor_plan table + unique (site_id, sort_order) + FK on site"
      contains: "CREATE TABLE floor_plan"
    - path: "internal/db/migrations/0032_device_floor_plan_placement.up.sql"
      provides: "device_floor_plan_placement table + CHECK x_frac/y_frac in [0,1] + FK on device + FK on floor_plan with ON DELETE CASCADE for floor_plan"
      contains: "CREATE TABLE device_floor_plan_placement"
    - path: "internal/db/queries/floor_plan.sql"
      provides: "CreateFloorPlan, ListFloorPlansBySite, GetFloorPlan, UpdateFloorPlanImage, UpdateFloorPlanLabel, DeleteFloorPlan, CountPinsOnFloorPlan"
      contains: "name: ListFloorPlansBySite"
    - path: "internal/floorplan/handlers.go"
      provides: "POST/GET/PATCH/DELETE chi handlers; UploadImage handler enforces MIME + dim + size before write; audit-in-tx pattern"
      contains: "func UploadImageHandler"
    - path: "internal/floorplan/image.go"
      provides: "ValidateImageHeader (MIME sniff via http.DetectContentType) + ProbeDimensions (image.DecodeConfig); both run BEFORE persistence"
      contains: "func ValidateImageHeader"
    - path: "compose/bundled.yml"
      provides: "Volume mount for /var/lib/shifter/floor-plans declared on the shifter service"
      contains: "floor_plans"
    - path: "compose/external.yml"
      provides: "Same volume mount for external flavor"
      contains: "floor_plans"
  key_links:
    - from: "internal/floorplan/handlers.go"
      to: "/var/lib/shifter/floor-plans/<floor_plan_id>.<ext>"
      via: "filepath.Join with validated UUID"
      pattern: "filepath\\.Join\\(deps\\.ImageRoot"
    - from: "internal/floorplan/handlers.go"
      to: "audit.WriteEntry(ctx, tx, …)"
      via: "same pgx.Tx as INSERT/UPDATE/DELETE"
      pattern: "audit\\.WriteEntry\\(.*tx,"
    - from: "compose/bundled.yml + compose/external.yml"
      to: "shifter:/var/lib/shifter/floor-plans"
      via: "named volume floor_plans"
      pattern: "floor_plans:/var/lib/shifter/floor-plans"
---

<objective>
Land the floor-plan data model (D-16 + SITE-02 vertical layouts via multi-row sort_order), the per-site CRUD HTTP handlers, the image-upload validator (MIME sniff + dimension probe BEFORE write, 10MB / 8192² caps per D-19, PNG+JPG only per D-17), and the compose volume mount that backs `image_path`. PDF→PNG conversion stays client-side (D-17) — this plan's server NEVER accepts `application/pdf`. Placement CRUD + decommission integration ships in plan 05-07 (depends on this plan's schema + endpoints).

Purpose: Floor plans are SITE-02..04's load-bearing artifact. Getting the schema right (especially x_frac/y_frac fractional coords) and the upload validation right (Pitfall: oversized images / MIME confusion DoS) is the foundation for plans 05-07 (placement CRUD) and 05-10 (canvas + pinning UX).

Output: 2 migrations, 1 sqlc query file, 1 Go package `internal/floorplan/` (handlers + image validation + routes), volume-mount edits to both compose flavors, audit constants extended.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@internal/db/migrations/0007_site.up.sql
@internal/db/migrations/0012_device.up.sql
@internal/audit/log.go
@internal/site/handlers.go
@internal/gateway/handlers.go
@compose/bundled.yml
@compose/external.yml

<interfaces>
<!-- 0031_floor_plan.up.sql will create -->
```sql
floor_plan (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id     UUID NOT NULL REFERENCES site(id) ON DELETE CASCADE,
  label       TEXT NOT NULL CHECK (length(trim(label)) > 0 AND length(label) <= 64),
  sort_order  INTEGER NOT NULL DEFAULT 0,
  image_path  TEXT NOT NULL,
  image_w     INTEGER NOT NULL CHECK (image_w > 0 AND image_w <= 8192),
  image_h     INTEGER NOT NULL CHECK (image_h > 0 AND image_h <= 8192),
  uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (site_id, sort_order)
)
```

<!-- 0032_device_floor_plan_placement.up.sql will create -->
```sql
device_floor_plan_placement (
  device_id      UUID PRIMARY KEY REFERENCES device(id) ON DELETE CASCADE,
  floor_plan_id  UUID NOT NULL REFERENCES floor_plan(id) ON DELETE CASCADE,
  x_frac         REAL NOT NULL CHECK (x_frac >= 0 AND x_frac <= 1),
  y_frac         REAL NOT NULL CHECK (y_frac >= 0 AND y_frac <= 1),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
)
```

<!-- New audit constants this plan adds -->
```go
const (
    ActionFloorPlanUpload  = "floor_plan.upload"
    ActionFloorPlanReplace = "floor_plan.replace_image"
    ActionFloorPlanRename  = "floor_plan.rename"
    ActionFloorPlanDelete  = "floor_plan.delete"
    EntityTypeFloorPlan    = "floor_plan"
)
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Migrations 0031 + 0032 + sqlc queries</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-16 §D-18 §D-25
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Recommended Project Structure (migration file naming)
    - internal/db/migrations/0007_site.up.sql (site table — FK target)
    - internal/db/migrations/0012_device.up.sql (device table — FK target; soft-delete flag)
    - internal/db/queries/sites.sql (sqlc query patterns for similar list/CRUD)
  </read_first>
  <behavior>
    - Test 1: 0031 round-trip — migrate-up creates floor_plan table; migrate-down drops it
    - Test 2: 0032 round-trip — creates device_floor_plan_placement; drops it
    - Test 3: ON DELETE CASCADE from site → floor_plan → device_floor_plan_placement works (deleting a site deletes its plans and placements)
    - Test 4: CHECK constraint rejects x_frac=1.5 or y_frac=-0.1
    - Test 5: UNIQUE (site_id, sort_order) rejects duplicate sort_order within a site
    - Test 6: All sqlc queries compile and `ListFloorPlansBySite` returns ordered rows
  </behavior>
  <action>
**Step A — `internal/db/migrations/0031_floor_plan.up.sql`:**

```sql
-- 0031_floor_plan.up.sql
-- Site floor plans (Phase 5 SITE-02 + SITE-03 + D-16).
--
-- One row per uploaded image. Vertical (multi-floor) layouts modelled as
-- multiple rows ordered by sort_order with admin-typed labels (e.g. B1, GF,
-- 1F, 2F). No layout_type enum — horizontal vs vertical inferred from row
-- count. Site is FK source with ON DELETE CASCADE so site deletion sweeps
-- the plans (and via 0032's cascade, the placements).
--
-- image_path stores a RELATIVE path under the floor_plans volume root
-- (/var/lib/shifter/floor-plans). The static-serve handler in plan 05-07
-- composes the absolute path from a server-side constant + this column.
--
-- image_w + image_h capped at 8192 (D-19 / Pitfall §upload DoS). Validated
-- both server-side (Task 2) AND via DB CHECK to defense-in-depth catch any
-- bypass of the handler.

CREATE TABLE floor_plan (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id     UUID NOT NULL REFERENCES site(id) ON DELETE CASCADE,
  label       TEXT NOT NULL CHECK (length(trim(label)) > 0 AND length(label) <= 64),
  sort_order  INTEGER NOT NULL DEFAULT 0,
  image_path  TEXT NOT NULL,
  image_w     INTEGER NOT NULL CHECK (image_w > 0 AND image_w <= 8192),
  image_h     INTEGER NOT NULL CHECK (image_h > 0 AND image_h <= 8192),
  uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (site_id, sort_order)
);

CREATE INDEX floor_plan_site_sort_idx ON floor_plan (site_id, sort_order);

COMMENT ON TABLE floor_plan IS 'Floor plan images per site (SITE-02/03). Multiple rows = multi-floor layout (D-16).';
COMMENT ON COLUMN floor_plan.image_path IS 'Relative path under /var/lib/shifter/floor-plans (D-18). Static serve via plan 05-07.';
```

**Step B — `internal/db/migrations/0031_floor_plan.down.sql`:**

```sql
DROP TABLE IF EXISTS floor_plan;
```

**Step C — `internal/db/migrations/0032_device_floor_plan_placement.up.sql`:**

```sql
-- 0032_device_floor_plan_placement.up.sql
-- Device placement on a floor plan (Phase 5 SITE-04 + D-20 + D-25).
--
-- device_id is the PRIMARY KEY — a device sits on AT MOST one floor plan at
-- any time. Fractional coordinates (x_frac, y_frac) are the load-bearing
-- design choice: resolution-independent, survive image replacement (D-24)
-- and Retina/mobile DPR.
--
-- ON DELETE CASCADE on both FKs:
--   - device deleted (hard) → placement deleted (cascade)
--   - floor_plan deleted → placements on it deleted (cascade)
--
-- BUT decommission is a SOFT delete (sets decommissioned_at, no DELETE FROM
-- device). Plan 05-07 handles the soft-delete case by issuing a DELETE on
-- the placement row inside the decommission handler's tx (D-25 invariant).

CREATE TABLE device_floor_plan_placement (
  device_id     UUID PRIMARY KEY REFERENCES device(id) ON DELETE CASCADE,
  floor_plan_id UUID NOT NULL REFERENCES floor_plan(id) ON DELETE CASCADE,
  x_frac        REAL NOT NULL CHECK (x_frac >= 0 AND x_frac <= 1),
  y_frac        REAL NOT NULL CHECK (y_frac >= 0 AND y_frac <= 1),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX device_floor_plan_placement_plan_idx ON device_floor_plan_placement (floor_plan_id);
```

**Step D — `internal/db/migrations/0032_device_floor_plan_placement.down.sql`:**

```sql
DROP TABLE IF EXISTS device_floor_plan_placement;
```

**Step E — `internal/db/queries/floor_plan.sql`:**

```sql
-- name: CreateFloorPlan :one
INSERT INTO floor_plan (site_id, label, sort_order, image_path, image_w, image_h)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListFloorPlansBySite :many
SELECT id, site_id, label, sort_order, image_path, image_w, image_h, uploaded_at, updated_at
FROM floor_plan
WHERE site_id = $1
ORDER BY sort_order, uploaded_at;

-- name: GetFloorPlan :one
SELECT id, site_id, label, sort_order, image_path, image_w, image_h, uploaded_at, updated_at
FROM floor_plan
WHERE id = $1;

-- name: UpdateFloorPlanImage :one
UPDATE floor_plan
SET image_path = $2, image_w = $3, image_h = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateFloorPlanLabel :one
UPDATE floor_plan
SET label = $2, sort_order = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteFloorPlan :exec
DELETE FROM floor_plan WHERE id = $1;

-- name: CountPinsOnFloorPlan :one
SELECT count(*) FROM device_floor_plan_placement WHERE floor_plan_id = $1;
```

Run `just sqlc`. Update `internal/db/migrations_test.go`:
- Bump migration count assertions (5 from plan 05-02 + 2 from this plan = +7 total since prior baseline)
- Add `TestRunMigrations_FloorPlanCascadeFromSite` — insert site + floor_plan + device + placement; DELETE FROM site; assert floor_plan AND placement rows both gone
- Add `TestRunMigrations_PlacementFractionalBounds_Rejected` — INSERT with x_frac=1.5 → expect 23514 (check_violation)
  </action>
  <verify>
    <automated>just sqlc &amp;&amp; go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations_(Clean|FloorPlanCascadeFromSite|PlacementFractionalBounds)"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0031_floor_plan.up.sql` contains literal `CREATE TABLE floor_plan` and `image_w INTEGER NOT NULL CHECK (image_w > 0 AND image_w <= 8192)` and `UNIQUE (site_id, sort_order)`
    - `internal/db/migrations/0032_device_floor_plan_placement.up.sql` contains literal `device_id UUID PRIMARY KEY REFERENCES device(id) ON DELETE CASCADE` and `CHECK (x_frac >= 0 AND x_frac <= 1)` and `CHECK (y_frac >= 0 AND y_frac <= 1)`
    - Both .down.sql files contain `DROP TABLE IF EXISTS`
    - `internal/db/queries/floor_plan.sql` contains all 7 queries listed in artifacts
    - `internal/db/sqlc/floor_plan.sql.go` exists after `just sqlc`
    - `TestRunMigrations_FloorPlanCascadeFromSite` asserts both floor_plan + placement gone after site DELETE
    - `TestRunMigrations_PlacementFractionalBounds_Rejected` asserts pgconn 23514 on out-of-range x_frac
    - `go test ./internal/db/... -short=false` exits 0
  </acceptance_criteria>
  <done>Schema + sqlc layer for floor plans live with cascade + bounds CHECK proven.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Image upload handler — MIME sniff + dim probe BEFORE write + size cap</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-17 §D-18 §D-19
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Security Domain
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Error States (upload error copy strings)
    - internal/audit/log.go (audit constants + WriteEntry pattern)
    - internal/import/handlers.go (existing multipart upload reference — Phase 3 bulk import)
  </read_first>
  <behavior>
    - Test 1: POST /api/sites/:id/floor-plans with PNG ≤10MB → 201 + floor_plan JSON; file lands at /var/lib/shifter/floor-plans/<id>.png with image_w/h persisted
    - Test 2: POST with `application/pdf` body → 415 (D-17: server NEVER processes PDF — frontend converts)
    - Test 3: POST with image/png + 11MB body → 413 BEFORE the body is fully read (http.MaxBytesReader cap)
    - Test 4: POST with PNG body that decodes to 10000×10000 → 422 (dimension > 8192 — probed before write)
    - Test 5: POST with mismatched Content-Type (claims PNG but bytes are JPEG) → server sniffs the actual MIME (http.DetectContentType) and accepts/rejects based on sniffed type, not the header
    - Test 6: Audit row written inside the same tx as the floor_plan INSERT
    - Test 7: GET /api/sites/:id/floor-plans returns rows ordered by sort_order
    - Test 8: PATCH /api/floor-plans/:id (new image upload) keeps device_floor_plan_placement rows intact
  </behavior>
  <action>
**Step A — Add audit constants to `internal/audit/log.go`:**

```go
const (
    ActionFloorPlanUpload  = "floor_plan.upload"
    ActionFloorPlanReplace = "floor_plan.replace_image"
    ActionFloorPlanRename  = "floor_plan.rename"
    ActionFloorPlanDelete  = "floor_plan.delete"
    EntityTypeFloorPlan    = "floor_plan"
)
```

Update the audit_log CHECK migration (preferred: new migration `0033_audit_vocab_floor_plan.up.sql` that ALTERs the existing CHECK constraint to add the four new actions + new entity type). Read 0020_audit_log_vocabulary first to see exact constraint shape.

**Step B — `internal/floorplan/doc.go`:**

```go
// Package floorplan owns site floor-plan uploads, replacement, listing,
// rename, and delete (Phase 5 SITE-02/03; placements live in plan 05-07).
//
// # Upload validation order (Pitfall: oversized image DoS)
//
//   1. http.MaxBytesReader caps the request body at 10MB BEFORE any read
//   2. multipart parsing produces an io.Reader for the file part
//   3. ValidateImageHeader sniffs the first 512 bytes via http.DetectContentType
//      and rejects anything outside {image/png, image/jpeg}
//   4. ProbeDimensions runs image.DecodeConfig (header-only, no full decode)
//      and rejects > 8192×8192 BEFORE the body is written to disk
//   5. Only after all four pass does the handler write to the floor_plans
//      volume and INSERT the floor_plan row
//
// PDF is REJECTED at step 3 (D-17 — client converts via pdf.js; server
// never sees PDF bytes). Test TestUploadHandler_RejectsPDF pins this.
//
// # Volume mount (D-18)
//
// Both compose flavors mount a named volume `floor_plans` at
// /var/lib/shifter/floor-plans (configurable via SHIFTER_FLOOR_PLANS_DIR).
// image_path stores the RELATIVE path (just "<uuid>.<ext>"); the static
// serve handler in plan 05-07 composes the absolute path with auth gating.
//
// # D-23 atomicity
//
// INSERT/UPDATE/DELETE on floor_plan land in a pgx.Tx with audit.WriteEntry
// in the same tx (Phase 2 D-23 invariant; Phase 3 audit-in-tx pattern).
package floorplan
```

**Step C — `internal/floorplan/image.go`:**

```go
package floorplan

import (
    "bytes"
    "errors"
    "fmt"
    "image"
    _ "image/jpeg"  // register decoders for DecodeConfig
    _ "image/png"
    "io"
    "net/http"
)

const (
    MaxUploadBytes = 10 << 20  // 10 MiB (D-19)
    MaxDimension   = 8192      // px (D-19)
)

var (
    ErrUnsupportedMIME = errors.New("unsupported_mime_type")
    ErrDimensionTooLarge = errors.New("dimensions_too_large")
    ErrImageDecodeFailed = errors.New("image_decode_failed")
)

// ValidateImageHeader sniffs the first 512 bytes and returns "image/png" or
// "image/jpeg" if valid, or ErrUnsupportedMIME otherwise. Does NOT advance
// the reader — returns a new io.Reader composed of the sniff buffer + the
// remaining body so the caller can stream the full image elsewhere.
//
// PDF is explicitly rejected per D-17 — server NEVER processes PDF; frontend
// converts via pdf.js. Test pins this.
func ValidateImageHeader(r io.Reader) (mime string, replay io.Reader, err error) {
    head := make([]byte, 512)
    n, _ := io.ReadFull(r, head)
    head = head[:n]
    sniffed := http.DetectContentType(head)
    switch sniffed {
    case "image/png":
        return "image/png", io.MultiReader(bytes.NewReader(head), r), nil
    case "image/jpeg":
        return "image/jpeg", io.MultiReader(bytes.NewReader(head), r), nil
    case "application/pdf":
        return "", nil, fmt.Errorf("%w: PDF not accepted (convert client-side via pdf.js)", ErrUnsupportedMIME)
    default:
        return "", nil, fmt.Errorf("%w: %s", ErrUnsupportedMIME, sniffed)
    }
}

// ProbeDimensions reads image header (DecodeConfig) — does NOT decode pixels.
// Returns (width, height, error). Rejects > 8192 in either dimension before
// the caller writes to disk.
func ProbeDimensions(r io.Reader) (w, h int, replay io.Reader, err error) {
    var buf bytes.Buffer
    cfg, _, err := image.DecodeConfig(io.TeeReader(r, &buf))
    if err != nil {
        return 0, 0, nil, fmt.Errorf("%w: %v", ErrImageDecodeFailed, err)
    }
    if cfg.Width > MaxDimension || cfg.Height > MaxDimension {
        return cfg.Width, cfg.Height, nil, fmt.Errorf("%w: %d×%d > %d", ErrDimensionTooLarge, cfg.Width, cfg.Height, MaxDimension)
    }
    return cfg.Width, cfg.Height, io.MultiReader(&buf, r), nil
}
```

**Step D — `internal/floorplan/handlers.go`:**

```go
package floorplan

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    sqlc "shifter/internal/db/sqlc"
    "shifter/internal/audit"
    "shifter/internal/auth"
)

type Deps struct {
    Pool      *pgxpool.Pool
    Queries   *sqlc.Queries
    ImageRoot string  // /var/lib/shifter/floor-plans
}

// UploadImageHandler — POST /api/sites/:id/floor-plans
// multipart/form-data with fields: `file` (binary), `label` (string),
// optional `sort_order` (int — defaults to max+1 within site).
func UploadImageHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, _ := auth.UserFromCtx(r.Context())
        siteID, err := uuid.Parse(chi.URLParam(r, "siteID"))
        if err != nil { writeError(w, 400, "invalid_site_id"); return }

        r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)  // T-05-05-01
        if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
            // ParseMultipartForm returns the same error type for size and parse failures;
            // detect by string match on "request body too large" (net/http convention).
            if errors.As(err, new(*http.MaxBytesError)) {
                writeError(w, 413, "file_too_large"); return
            }
            writeError(w, 400, "multipart_parse_failed"); return
        }

        file, hdr, err := r.FormFile("file")
        if err != nil { writeError(w, 400, "file_missing"); return }
        defer file.Close()
        _ = hdr

        label := r.FormValue("label")
        if label == "" { writeError(w, 422, "label_required"); return }

        mime, body1, err := ValidateImageHeader(file)
        if err != nil {
            switch {
            case errors.Is(err, ErrUnsupportedMIME):
                writeError(w, 415, "unsupported_mime_type")
            default:
                writeError(w, 400, "image_validation_failed")
            }
            return
        }

        width, height, body2, err := ProbeDimensions(body1)
        if err != nil {
            switch {
            case errors.Is(err, ErrDimensionTooLarge):
                writeError(w, 422, "dimensions_too_large")
            case errors.Is(err, ErrImageDecodeFailed):
                writeError(w, 422, "image_decode_failed")
            default:
                writeError(w, 400, "image_validation_failed")
            }
            return
        }

        // Reserve UUID, build path, write file BEFORE DB so we can clean up on rollback.
        planID := uuid.New()
        ext := extFromMIME(mime)  // "png" | "jpg"
        relPath := fmt.Sprintf("%s.%s", planID.String(), ext)
        absPath := filepath.Join(deps.ImageRoot, relPath)
        out, err := os.Create(absPath)
        if err != nil { writeError(w, 500, "fs_create_failed"); return }
        if _, err := io.Copy(out, body2); err != nil { out.Close(); os.Remove(absPath); writeError(w, 500, "fs_write_failed"); return }
        out.Close()

        tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
        if err != nil { os.Remove(absPath); writeError(w, 500, "tx_begin"); return }
        rolledBack := true
        defer func() { if rolledBack { tx.Rollback(r.Context()); os.Remove(absPath) } }()
        q := deps.Queries.WithTx(tx)

        sortOrder := parseSortOrderOrNextSlot(r.Context(), q, siteID, r.FormValue("sort_order"))
        plan, err := q.CreateFloorPlan(r.Context(), sqlc.CreateFloorPlanParams{
            ID: planID, SiteID: siteID, Label: label, SortOrder: sortOrder,
            ImagePath: relPath, ImageW: int32(width), ImageH: int32(height),
        })
        if err != nil {
            // UNIQUE violation on (site_id, sort_order) → 409.
            writeError(w, 500, "db_create_failed"); return
        }

        if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
            UserID: user.ID, Action: audit.ActionFloorPlanUpload,
            EntityType: audit.EntityTypeFloorPlan, EntityID: planID,
            After: map[string]any{"site_id": siteID, "label": label, "image_w": width, "image_h": height, "image_path": relPath},
            RequestID: middleware.GetReqID(r.Context()),
        }); err != nil { writeError(w, 500, "audit_failed"); return }

        if err := tx.Commit(r.Context()); err != nil { writeError(w, 500, "tx_commit"); return }
        rolledBack = false

        writeJSON(w, http.StatusCreated, plan)
    }
}

// ReplaceImageHandler — PATCH /api/floor-plans/:id (multipart) — replaces image
// while keeping all device_floor_plan_placement rows (D-24 fractional coords).
// Same validation pipeline; on success: writes new file, UPDATEs floor_plan,
// audit row (action=floor_plan.replace_image) with before/after image_path.
// On commit success the OLD file is unlinked from disk; on rollback the NEW
// file is unlinked.

// ListByPlan, GetHandler, RenameHandler, DeleteHandler — straightforward
// sqlc-backed handlers, each with audit-in-tx for the mutating operations.

// RegisterRoutes mounts:
//   POST   /api/sites/{siteID}/floor-plans      → UploadImageHandler
//   GET    /api/sites/{siteID}/floor-plans      → ListBySiteHandler
//   GET    /api/floor-plans/{id}                → GetHandler
//   PATCH  /api/floor-plans/{id}                → multipart → ReplaceImageHandler
//   PATCH  /api/floor-plans/{id}/label          → JSON → RenameHandler
//   DELETE /api/floor-plans/{id}                → DeleteHandler
func RegisterRoutes(r chi.Router, deps Deps) { /* ... */ }
```

**Step E — Volume mounts in BOTH compose flavors:**

In `compose/bundled.yml` and `compose/external.yml`:

1. Add to top-level `volumes:` block: `floor_plans:`
2. Add to the `shifter` service `volumes:` list: `- floor_plans:/var/lib/shifter/floor-plans`
3. Reuse the existing `x-logging` anchor pattern

Read both files first to see exact indentation + existing volume blocks.

**Step F — Replace `t.Skip` in `internal/floorplan/handlers_test.go`:**

Minimum test cases:

1. `TestImageUpload_PNG_HappyPath` — upload valid 100×100 PNG → 201; floor_plan row exists; audit row exists; file present at `<tmp>/<uuid>.png`
2. `TestImageUpload_RejectsPDF` — POST with `application/pdf` MIME → 415; no DB row; no file written
3. `TestImageUpload_RejectsOversizedFile` — POST 11MB body → 413
4. `TestImageUpload_RejectsOversizedDimensions` — POST PNG that decodes to 10000×10000 → 422
5. `TestImageUpload_MimeSniffOverridesHeader` — POST with `Content-Type: image/png` but JPEG bytes → server sniffs `image/jpeg` and persists with `.jpg` extension (accepts because JPEG is in allowlist)
6. `TestImageUpload_AuditAndDBRollback` — inject DB failure post-audit; assert file is unlinked AND no floor_plan row AND no audit row remains
7. `TestMultiFloor` — INSERT 3 floor_plan rows for one site with sort_order 1/2/3; ListFloorPlansBySite returns them in order
8. `TestReplaceKeepsPins` — upload plan, INSERT 2 placement rows, PATCH replace image, assert both placement rows still exist with unchanged x_frac/y_frac
  </action>
  <verify>
    <automated>go test ./internal/floorplan/... -race -count=1 -short -run "TestImageUpload|TestMultiFloor|TestReplaceKeepsPins" &amp;&amp; grep -q "floor_plans:/var/lib/shifter/floor-plans" compose/bundled.yml &amp;&amp; grep -q "floor_plans:/var/lib/shifter/floor-plans" compose/external.yml</automated>
  </verify>
  <acceptance_criteria>
    - `internal/floorplan/image.go` contains literal `MaxUploadBytes = 10 << 20` and `MaxDimension = 8192` and `http.DetectContentType` and explicit `application/pdf` rejection case
    - `internal/floorplan/handlers.go` contains literal `http.MaxBytesReader(w, r.Body, MaxUploadBytes)` AND `image.DecodeConfig` reachable via ProbeDimensions
    - `internal/floorplan/handlers.go` contains literal `audit.WriteEntry(r.Context(), tx,` and `audit.ActionFloorPlanUpload`
    - Both compose YAMLs contain literal `floor_plans:/var/lib/shifter/floor-plans` in the shifter service volumes list AND a top-level `floor_plans:` named volume entry
    - At least 8 named tests in `internal/floorplan/handlers_test.go` covering: PNG happy, PDF reject (415), oversized file (413), oversized dims (422), MIME-sniff-overrides-header, audit-and-DB-rollback, multi-floor list ordering, replace-keeps-pins
    - `TestImageUpload_RejectsPDF` asserts response is 415 AND no file written AND no DB row
    - `TestImageUpload_AuditAndDBRollback` asserts the file is unlinked on rollback (defensive against orphan files when DB tx fails)
    - `go test ./internal/floorplan/... -short` exits 0
  </acceptance_criteria>
  <done>Floor-plan upload validated end-to-end with all D-17/D-18/D-19 constraints + volume mount visible in both compose flavors.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Untrusted upload → handler | Multipart body crosses; size capped via http.MaxBytesReader BEFORE any parse |
| Sniffed MIME → file extension | Server sniffs first 512 bytes, not the Content-Type header |
| Dimension probe → filesystem | image.DecodeConfig reads only header bytes; no full decode, no DoS via decompression bomb |
| filepath.Join(ImageRoot, "<uuid>.<ext>") → disk | UUID is server-generated; ext from sniffed MIME — no client-controlled string in path |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-05-01 | Denial of Service | Oversized image upload exhausts disk + memory | high | mitigate | `http.MaxBytesReader(w, r.Body, 10<<20)` BEFORE any read; `image.DecodeConfig` (header-only, no pixel decode) bounds 8192×8192 dim cap; CHECK constraint on `image_w`/`image_h` as defense-in-depth |
| T-05-05-02 | Tampering | MIME confusion attack — client sets Content-Type: image/png but uploads executable | high | mitigate | `http.DetectContentType` sniff drives the decision; Content-Type header ignored. PDF explicitly rejected (D-17). Only `image/png` and `image/jpeg` accepted. |
| T-05-05-03 | Elevation of Privilege | Path traversal via `image_path` column | medium | mitigate | image_path stores `<server-generated-uuid>.<sniffed-ext>` only — no client-controlled bytes in the path. ImageRoot is a server-side constant. Plan 05-07 static serve validates the floor_plan UUID before composing the path. |
</threat_model>

<verification>
1. `go test ./internal/floorplan/... -race -count=1 -short` exits 0
2. `go test ./internal/db/... -race -count=1 -short=false -run TestRunMigrations` exits 0 (0031 + 0032 round-trip)
3. `grep "floor_plans" compose/bundled.yml compose/external.yml | wc -l` returns ≥ 4 (2 lines × 2 files)
4. NO PDF acceptance: `grep "application/pdf" internal/floorplan/image.go` shows the rejection case
5. NO client-controlled filename leak: `! grep "hdr.Filename" internal/floorplan/handlers.go` (we ignore the client-provided filename)
</verification>

<success_criteria>
- Two new migrations (0031, 0032) round-trip clean against TimescaleDB testcontainer
- sqlc CRUD layer for floor_plan generated
- Upload handler validates in correct order (size → MIME sniff → dim probe → write → tx); rejects PDF at MIME sniff
- Volume mount declared in BOTH compose flavors
- Audit constants extended; vocab migration updates the CHECK
- 8+ test cases covering happy path, all rejection paths, and rollback
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-05-SUMMARY.md` recording:
- Migration count (cumulative) after Phase 5 Plans 1 + 2 + 3 + 5 land
- audit_log vocabulary migration approach (new file vs ALTER existing)
- Disk path layout used in tests (testify's t.TempDir() vs custom)
- Any surprises with image.DecodeConfig vs full image.Decode performance
</output>
