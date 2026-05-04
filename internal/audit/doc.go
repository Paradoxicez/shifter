// Package audit implements the AUDIT-01 / D-22 / D-23 / D-24 contract:
// every state-changing action on Site, MeteringPoint, Device, DeviceProfile,
// and Binding writes an audit_log row INSIDE the caller's transaction so the
// audit row commits atomically with the domain mutation it describes.
//
// Pattern 8 (RESEARCH §"Audit log writer that runs inside caller's
// transaction"): WriteEntry takes a pgx.Tx — NOT a Pool — so by API contract
// it cannot be called outside an active transaction. Combined with the
// INSERT-ONLY trigger from migration 0016, audit rows are append-only and
// atomic with mutations: an audit row LITERALLY cannot exist without its
// domain row, by Postgres atomicity.
//
// The Entry struct + ChangedFields helper produce the field-level diff that
// Phase 6 audit browse renders. Open Q #4 resolution: we use EXPLICIT-NULL
// semantics (a field cleared in `after` lands as JSON null) rather than
// missing-key semantics, so a Phase 6 reviewer can distinguish "this field
// wasn't part of the diff" from "this field was changed FROM something
// TO null."
package audit
