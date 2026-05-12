// Package floorplan owns site floor-plan uploads, replacement, listing,
// rename, and delete (Phase 5 SITE-02/03; placements live in plan 05-07).
//
// # Upload validation order (Pitfall: oversized image DoS)
//
//  1. http.MaxBytesReader caps the request body at 10MB BEFORE any read
//  2. multipart parsing produces an io.Reader for the file part
//  3. ValidateImageHeader sniffs the first 512 bytes via http.DetectContentType
//     and rejects anything outside {image/png, image/jpeg}
//  4. ProbeDimensions runs image.DecodeConfig (header-only, no full decode)
//     and rejects > 8192×8192 BEFORE the body is written to disk
//  5. Only after all four pass does the handler write to the floor_plans
//     volume and INSERT the floor_plan row
//
// PDF is REJECTED at step 3 (D-17 — client converts via pdf.js; server
// never sees PDF bytes). Test TestImageUpload_RejectsPDF pins this.
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
