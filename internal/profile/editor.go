package profile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// MaxCodecJSBytes caps the codec_js source the editor accepts. T-02-08-01
// mitigation: a runaway operator paste cannot stuff arbitrarily large blobs
// into Postgres or the ChirpStack profile record. 256 KiB is well above any
// hand-written QuickJS codec we've seen (~10 KiB typical, ~50 KiB for an
// every-vendor monstrosity) and well below pgsql TOAST thresholds.
const MaxCodecJSBytes = 256 * 1024

// validCapabilities is the D-04 vocabulary, mirrored exactly to the CHECK
// constraint in migration 0009. Editor rejects unknown tokens at the handler
// boundary; the CHECK is defense-in-depth.
var validCapabilities = map[string]struct{}{
	"cumulative":        {},
	"flow_rate":         {},
	"instant_power":     {},
	"battery":           {},
	"temperature":       {},
	"pressure":          {},
	"leak_detection":    {},
	"tamper_detection":  {},
	"multi_phase":       {},
	"power_quality":     {},
}

// validDataTypes mirrors migration 0013's mapping data_type CHECK.
var validDataTypes = map[string]struct{}{
	"numeric": {},
	"int":     {},
	"bool":    {},
	"text":    {},
}

// Mapping is a single device_profile_mapping row in flight to/from the
// editor. Scale is *big.Float for lossless decimal precision (NUMERIC column).
type Mapping struct {
	JSONPointer string     // RFC 6901 — empty allowed (whole-decoded-object scalar)
	Target      string     // canonical column name OR "extra.<key>"
	Scale       *big.Float // nil → defaults to 1
	DataType    string     // numeric|int|bool|text
	Position    int32      // mapping pass order (0-based)
}

// ProfileSaveInput is the value-shape an HTTP handler hands SaveProfile.
type ProfileSaveInput struct {
	UserID    uuid.UUID // operator (audit_log.user_id)
	RequestID string    // chi middleware.RequestID

	// Identity
	ID             uuid.UUID // zero = create; non-zero = update existing
	Slug           string    // immutable on update; lower-case
	Name           string
	Vendor         string
	Family         string // optional (empty → SQL NULL)
	Capabilities   []string
	CounterModulus int64
	Region         string // empty → inherit install region (SQL NULL)
	MACVersion     string

	// Codec
	CodecJS string // raw QuickJS source; "" allowed (sync skipped)

	// Mappings — full set; old mappings deleted + replaced atomically.
	Mappings []Mapping
}

// CSProfileClient is the narrow ChirpStack contract SaveProfile depends on.
// Implemented by *chirpstack.Client.
type CSProfileClient interface {
	CreateDeviceProfile(ctx context.Context, in chirpstack.CreateProfileInput) (string, error)
	UpdateDeviceProfile(ctx context.Context, in chirpstack.UpdateProfileInput) error
}

// ConnectionStore exposes the singleton chirpstack_connection row for the
// CS-tenant ID. Implemented by a thin sqlc-backed wrapper at cmd/serve.
type ConnectionStore interface {
	GetCSConnection(ctx context.Context) (csTenantID, csApplicationID string, err error)
}

// Deps bundles the shared infra a SaveProfile call needs.
type Deps struct {
	Pool      *pgxpool.Pool
	CSClient  CSProfileClient
	ConnStore ConnectionStore
	Log       *slog.Logger
}

// SaveProfile creates or updates a device_profile + its mapping set + pushes
// codec_js to ChirpStack, all atomically.
//
// Atomicity contract:
//
//   - All Postgres mutations (UPSERT profile, replace mappings, MarkSynced,
//     audit row) run inside ONE pgx.Serializable transaction.
//   - The CS gRPC call (CreateDeviceProfile / UpdateDeviceProfile) happens
//     INSIDE the transaction window, BEFORE Commit. A CS failure short-circuits
//     to tx.Rollback — the operator sees a single error, the DB stays clean.
//   - On commit success, both Postgres and ChirpStack are in agreement.
//
// Boundary cases:
//
//   - Empty CodecJS — DB row updated, mappings replaced, audit written, CS
//     push SKIPPED. Profile is "saved but unsynced"; the boot-time seed
//     routine (RunSeedSync) will not push because codec_js is empty too.
//   - Update path: cs_profile_id non-NULL → CS Update; NULL → CS Create.
//   - Capability vocabulary violation, codec size violation, missing tenant —
//     all return error BEFORE opening the transaction (no tx allocation cost
//     on bad input).
//
// Returns the persisted profile id (creates a fresh UUID on insert).
func SaveProfile(ctx context.Context, deps Deps, in ProfileSaveInput) (uuid.UUID, error) {
	if deps.Pool == nil {
		return uuid.Nil, errors.New("profile: nil pgxpool")
	}
	if deps.Log == nil {
		deps.Log = slog.Default()
	}

	// 1. Pre-tx validation — reject obvious bad input cheaply.
	if len(in.CodecJS) > MaxCodecJSBytes {
		return uuid.Nil, fmt.Errorf("profile: codec_js exceeds %d bytes (got %d)", MaxCodecJSBytes, len(in.CodecJS))
	}
	if in.CodecJS != "" && !strings.Contains(in.CodecJS, "function decodeUplink") {
		// ChirpStack QuickJS sandbox contract — codec MUST export decodeUplink.
		return uuid.Nil, errors.New("profile: codec_js must define a decodeUplink function (ChirpStack QuickJS sandbox contract — D-09)")
	}
	if in.Slug == "" || in.Slug != strings.ToLower(in.Slug) {
		return uuid.Nil, fmt.Errorf("profile: slug must be lowercase (got %q)", in.Slug)
	}
	if in.Name == "" {
		return uuid.Nil, errors.New("profile: name is required")
	}
	if in.Vendor == "" {
		return uuid.Nil, errors.New("profile: vendor is required")
	}
	if in.MACVersion == "" {
		return uuid.Nil, errors.New("profile: mac_version is required")
	}
	if in.CounterModulus <= 0 {
		return uuid.Nil, fmt.Errorf("profile: counter_modulus must be positive (got %d)", in.CounterModulus)
	}
	for _, c := range in.Capabilities {
		if _, ok := validCapabilities[c]; !ok {
			return uuid.Nil, fmt.Errorf("profile: invalid capability %q (allowed: cumulative, flow_rate, instant_power, battery, temperature, pressure, leak_detection, tamper_detection, multi_phase, power_quality)", c)
		}
	}
	for i, m := range in.Mappings {
		if _, ok := validDataTypes[m.DataType]; !ok {
			return uuid.Nil, fmt.Errorf("profile: mapping[%d] invalid data_type %q (allowed: numeric, int, bool, text)", i, m.DataType)
		}
		if m.Target == "" {
			return uuid.Nil, fmt.Errorf("profile: mapping[%d] target is required", i)
		}
		if m.JSONPointer != "" && !strings.HasPrefix(m.JSONPointer, "/") {
			return uuid.Nil, fmt.Errorf("profile: mapping[%d] json_pointer %q must be empty or start with '/'", i, m.JSONPointer)
		}
	}

	// 2. CS tenant must already be bootstrapped (D-28). If the codec is
	//    being pushed to CS, we need the tenant id for Create; an empty codec
	//    skips CS entirely so we can save without CS reachable.
	var tenantID string
	if in.CodecJS != "" {
		t, _, err := deps.ConnStore.GetCSConnection(ctx)
		if err != nil {
			return uuid.Nil, fmt.Errorf("profile: read chirpstack_connection: %w", err)
		}
		if t == "" {
			return uuid.Nil, errors.New("profile: ChirpStack tenant not bootstrapped — open Settings → Test connection (D-28)")
		}
		tenantID = t
	}

	// 3. Open the atomic transaction.
	tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return uuid.Nil, fmt.Errorf("profile: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlc.New(tx)

	// 4. Create or update the device_profile row.
	var (
		profileID    uuid.UUID
		auditAction  string
		preState     map[string]any
		csProfilePre pgtype.UUID // existing cs_profile_id (for choosing Create vs Update)
	)
	familyPtr := stringOrNil(in.Family)
	regionPtr := stringOrNil(in.Region)

	if in.ID == uuid.Nil {
		row, err := q.CreateDeviceProfile(ctx, sqlc.CreateDeviceProfileParams{
			Slug:           in.Slug,
			Name:           in.Name,
			Vendor:         in.Vendor,
			Family:         familyPtr,
			Capabilities:   in.Capabilities,
			CounterModulus: in.CounterModulus,
			CodecJs:        in.CodecJS,
			Region:         regionPtr,
			MacVersion:     in.MACVersion,
		})
		if err != nil {
			return uuid.Nil, fmt.Errorf("profile: create row: %w", err)
		}
		profileID = uuid.UUID(row.ID.Bytes)
		auditAction = audit.ActionProfileCreate
		preState = nil // CREATE → audit_log.before SQL NULL per D-24
		csProfilePre = row.CsProfileID
	} else {
		// Capture pre-state for the audit diff.
		existing, err := q.GetDeviceProfile(ctx, pgtype.UUID{Bytes: in.ID, Valid: true})
		if err != nil {
			return uuid.Nil, fmt.Errorf("profile: load existing row: %w", err)
		}
		preState = map[string]any{
			"name":            existing.Name,
			"vendor":          existing.Vendor,
			"family":          stringPtrText(existing.Family),
			"capabilities":    existing.Capabilities,
			"counter_modulus": existing.CounterModulus,
			"region":          stringPtrText(existing.Region),
			"mac_version":     existing.MacVersion,
			"mapping_count":   nil, // mapping_count is captured separately if needed
		}
		csProfilePre = existing.CsProfileID

		row, err := q.UpdateDeviceProfile(ctx, sqlc.UpdateDeviceProfileParams{
			ID:             pgtype.UUID{Bytes: in.ID, Valid: true},
			Name:           in.Name,
			Vendor:         in.Vendor,
			Family:         familyPtr,
			Capabilities:   in.Capabilities,
			CounterModulus: in.CounterModulus,
			CodecJs:        in.CodecJS,
			Region:         regionPtr,
			MacVersion:     in.MACVersion,
		})
		if err != nil {
			return uuid.Nil, fmt.Errorf("profile: update row: %w", err)
		}
		profileID = uuid.UUID(row.ID.Bytes)
		auditAction = audit.ActionProfileUpdate
	}

	// 5. SetProfileCodecJS clears codec_js_synced_at (NULL) so the next boot
	//    re-syncs if we don't push during this save.
	if err := q.SetProfileCodecJS(ctx, sqlc.SetProfileCodecJSParams{
		ID:      pgtype.UUID{Bytes: profileID, Valid: true},
		CodecJs: in.CodecJS,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("profile: set codec_js: %w", err)
	}

	// 6. Replace mappings — DELETE all then INSERT each in declared order.
	if err := q.DeleteMappingsByProfile(ctx, pgtype.UUID{Bytes: profileID, Valid: true}); err != nil {
		return uuid.Nil, fmt.Errorf("profile: delete old mappings: %w", err)
	}
	for i, m := range in.Mappings {
		scale := m.Scale
		if scale == nil {
			scale = big.NewFloat(1)
		}
		scaleNumeric, err := numericFromBigFloat(scale)
		if err != nil {
			return uuid.Nil, fmt.Errorf("profile: mapping[%d] scale encode: %w", i, err)
		}
		if _, err := q.CreateMapping(ctx, sqlc.CreateMappingParams{
			DeviceProfileID: pgtype.UUID{Bytes: profileID, Valid: true},
			JsonPointer:     m.JSONPointer,
			Target:          m.Target,
			Scale:           scaleNumeric,
			DataType:        m.DataType,
			Position:        m.Position,
		}); err != nil {
			return uuid.Nil, fmt.Errorf("profile: create mapping[%d] (%s → %s): %w", i, m.JSONPointer, m.Target, err)
		}
	}

	// 7. Push codec_js to ChirpStack INSIDE the tx window (before commit) so
	//    a CS failure rolls back the Postgres mutations. Pitfall 9 mitigation
	//    re: tenant-not-bootstrapped is upstream (step 2); here we only run
	//    when codec is non-empty AND tenant is known.
	var csProfileID string
	if in.CodecJS != "" {
		if csProfilePre.Valid {
			// Existing CS-side profile — Update.
			csProfileID = uuid.UUID(csProfilePre.Bytes).String()
			if err := deps.CSClient.UpdateDeviceProfile(ctx, chirpstack.UpdateProfileInput{
				ID:      csProfileID,
				Name:    in.Name,
				CodecJS: in.CodecJS,
			}); err != nil {
				return uuid.Nil, fmt.Errorf("profile: CS UpdateDeviceProfile: %w", err)
			}
		} else {
			// Create-side. CreateProfileInput.RegParamsRevision defaults
			// upstream to "RP002_1_0_3" if blank — but the chirpstack package's
			// regParamsRevisionEnum REJECTS blank, so we set a sensible default
			// here. Phase 6 will surface this on the editor UI if a customer
			// needs RP-A or RP-B.
			region := in.Region
			if region == "" {
				region = "AS923_2" // INST-04 Thai default; matches install wizard
			}
			created, err := deps.CSClient.CreateDeviceProfile(ctx, chirpstack.CreateProfileInput{
				TenantID:          tenantID,
				Name:              in.Name,
				Description:       fmt.Sprintf("Shifter profile %s (%s)", in.Slug, in.Vendor),
				Region:            region,
				MACVersion:        in.MACVersion,
				RegParamsRevision: "RP002_1_0_3",
				CodecJS:           in.CodecJS,
				SupportsOTAA:      true,
			})
			if err != nil {
				return uuid.Nil, fmt.Errorf("profile: CS CreateDeviceProfile: %w", err)
			}
			csProfileID = created
		}

		csUUID, err := uuid.Parse(csProfileID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("profile: CS returned non-UUID id %q: %w", csProfileID, err)
		}
		if err := q.MarkProfileSyncedToChirpStack(ctx, sqlc.MarkProfileSyncedToChirpStackParams{
			ID:          pgtype.UUID{Bytes: profileID, Valid: true},
			CsProfileID: pgtype.UUID{Bytes: csUUID, Valid: true},
		}); err != nil {
			return uuid.Nil, fmt.Errorf("profile: mark synced: %w", err)
		}
	}

	// 8. Audit row INSIDE same tx (D-23 / AUDIT-01).
	postState := map[string]any{
		"name":            in.Name,
		"vendor":          in.Vendor,
		"family":          in.Family,
		"capabilities":    in.Capabilities,
		"counter_modulus": in.CounterModulus,
		"region":          in.Region,
		"mac_version":     in.MACVersion,
		"mapping_count":   len(in.Mappings),
		"codec_synced":    in.CodecJS != "",
	}
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     in.UserID,
		Action:     auditAction,
		EntityType: audit.EntityTypeDeviceProfile,
		EntityID:   profileID,
		Before:     preState,
		After:      postState,
		RequestID:  in.RequestID,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("profile: write audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("profile: commit: %w", err)
	}

	deps.Log.Info("profile saved",
		"profile_id", profileID,
		"slug", in.Slug,
		"action", auditAction,
		"mapping_count", len(in.Mappings),
		"codec_pushed", in.CodecJS != "",
	)
	return profileID, nil
}

// numericFromBigFloat encodes a *big.Float as pgtype.Numeric via its decimal
// text form. pgtype.Numeric.Scan(string) parses the text into the lossless
// Int + Exp representation.
func numericFromBigFloat(f *big.Float) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	text := f.Text('f', -1)
	if err := n.Scan(text); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("scan %q: %w", text, err)
	}
	return n, nil
}

// stringOrNil returns nil for empty, &s otherwise. Pairs with sqlc's
// emit_pointers_for_null_types=true.
func stringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

// stringPtrText dereferences a nil-able TEXT column for inclusion in audit
// JSONB. nil → empty string (which JSON-encodes to "" rather than the literal
// "null"). Phase 6 audit browse distinguishes added/removed keys via the
// EXPLICIT-NULL semantics in audit.ChangedFields, not via empty-vs-null
// string contents.
func stringPtrText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
