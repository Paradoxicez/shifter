package swap

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// Invalidator is the contract CommitSwap uses to drop a dev_eui from the
// resolver cache after a successful commit. Defense in depth on top of the
// 0017 binding_changed NOTIFY trigger — direct call ensures the cache is
// clean even if the listener is mid-reconnect when the swap commits.
//
// Implemented by *resolver.Resolver in the resolver package; this interface
// here keeps the swap package free of a circular import.
type Invalidator interface {
	Invalidate(devEUI string)
}

// Deps bundles the shared infra a CommitSwap call needs.
type Deps struct {
	Pool     *pgxpool.Pool
	Resolver Invalidator // optional: nil means "rely on NOTIFY trigger only"
	Log      *slog.Logger
}

// SwapInput is the value-shape an HTTP handler hands to CommitSwap once the
// operator has confirmed the swap dialog. Every field is required except
// OperatorOverride / OperatorNotes / OutgoingDevEUI / IncomingDevEUI (the
// last two are required for resolver invalidation but tolerated empty so
// tests can opt out).
type SwapInput struct {
	UserID    uuid.UUID // operator initiating the swap (audit_log.user_id)
	RequestID string    // chi middleware.RequestID for audit_log.request_id

	MeteringPointID uuid.UUID // the MP whose binding is being swapped

	OutgoingBindingID uuid.UUID // current active binding; will be closed
	OutgoingDevEUI    string    // for resolver invalidation (lowercase 16-hex)
	IncomingDeviceID  uuid.UUID // newly bound device row id
	IncomingDevEUI    string    // for resolver invalidation (lowercase 16-hex)

	ConfirmTime time.Time // operator click time = D-14 swap timing semantics

	OutgoingReadingR *big.Float // captured outgoing reading (D-12, R)
	IncomingInitialN *big.Float // new meter starting value (D-13, N — often 0)
	OperatorOverride *big.Float // optional: operator-supplied offset overriding ProposeOffset

	OperatorNotes string // free-text "why this swap" → audit_log.notes
}

// CommitSwap executes a swap as one atomic Serializable transaction:
//
//  1. CloseBinding(outgoing) — sets valid_to = ConfirmTime (D-14).
//  2. OpenBinding(incoming) — new active row with computed reading_offset.
//  3. audit.WriteEntry — single 'swap' action audit row inside the same tx
//     (D-23 / AUDIT-01). Before captures the outgoing binding's pre-close
//     state; After captures the new binding plus the {R, N, proposed,
//     applied, override_used} swap mechanics.
//  4. tx.Commit.
//  5. Defensive resolver.Invalidate for both dev_eui values (the 0017
//     trigger also fires NOTIFY binding_changed on close+open).
//
// Concurrency: the binding_no_overlap_per_mp + _per_device EXCLUDE
// constraints (0014 btree_gist) guarantee at most one active binding per
// MP and per device. Two concurrent CommitSwap calls on the same MP race
// in OpenBinding; one wins, the other gets 23P01 (exclusion_violation).
// Serializable isolation additionally collapses subtle read-then-write
// races (e.g. two operators clicking confirm in adjacent tabs).
//
// Returns the new binding's id on success — handlers use it for the
// 201 Created Location response (Phase 2 plan 02-10).
//
// Error wrapping: each step wraps with a stable prefix so callers can
// errors.Is the underlying pgconn.PgError if they need to map 23P01 to
// a 409 Conflict response. The exclusion_violation case is the dominant
// "expected" failure mode; everything else is a 500.
func CommitSwap(ctx context.Context, deps Deps, in SwapInput) (uuid.UUID, error) {
	if deps.Pool == nil {
		return uuid.Nil, fmt.Errorf("swap: nil pgxpool")
	}
	if deps.Log == nil {
		// Avoid nil deref in info/debug logs without forcing every caller to
		// supply a logger — production wiring always passes one; tests may not.
		deps.Log = slog.Default()
	}
	if in.OutgoingReadingR == nil || in.IncomingInitialN == nil {
		return uuid.Nil, fmt.Errorf("swap: OutgoingReadingR and IncomingInitialN are required")
	}

	// Compute proposed offset; operator override takes precedence per D-13.
	proposed := ProposeOffset(in.OutgoingReadingR, in.IncomingInitialN)
	appliedOffset := proposed
	if in.OperatorOverride != nil {
		appliedOffset = in.OperatorOverride
	}

	tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return uuid.Nil, fmt.Errorf("swap: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlc.New(tx)

	// 1. Close outgoing binding at swap.confirm_time (D-14).
	oldBinding, err := q.CloseBinding(ctx, sqlc.CloseBindingParams{
		ID:      pgtype.UUID{Bytes: in.OutgoingBindingID, Valid: true},
		ValidTo: pgtype.Timestamptz{Time: in.ConfirmTime, Valid: true},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("swap: close outgoing binding: %w", err)
	}

	// 2. Open incoming binding (EXCLUDE constraints enforce no overlap per
	//    MP AND per device — concurrent commits race here).
	offsetNumeric, err := numericFromBigFloat(appliedOffset)
	if err != nil {
		return uuid.Nil, fmt.Errorf("swap: encode offset: %w", err)
	}
	newBinding, err := q.OpenBinding(ctx, sqlc.OpenBindingParams{
		MeteringPointID: pgtype.UUID{Bytes: in.MeteringPointID, Valid: true},
		DeviceID:        pgtype.UUID{Bytes: in.IncomingDeviceID, Valid: true},
		ValidFrom:       pgtype.Timestamptz{Time: in.ConfirmTime, Valid: true},
		ReadingOffset:   offsetNumeric,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("swap: open incoming binding: %w", err)
	}

	// 3. Audit row INSIDE the same tx (D-23). EntityID = newBinding.id;
	//    the closed binding is referenced inside the diff JSON.
	before := map[string]any{
		"binding_id":     uuidFromPg(oldBinding.ID),
		"device_id":      uuidFromPg(oldBinding.DeviceID),
		"valid_to":       nil, // explicit-null per Open Q #4 — was open before close
		"reading_offset": numericText(oldBinding.ReadingOffset),
	}
	after := map[string]any{
		"binding_id":      uuidFromPg(newBinding.ID),
		"device_id":       in.IncomingDeviceID.String(),
		"valid_from":      in.ConfirmTime.UTC().Format(time.RFC3339Nano),
		"reading_offset":  appliedOffset.Text('g', 32),
		"outgoing_R":      in.OutgoingReadingR.Text('g', 32),
		"incoming_N":      in.IncomingInitialN.Text('g', 32),
		"proposed_offset": proposed.Text('g', 32),
		"override_used":   in.OperatorOverride != nil,
	}
	newBindingID := uuid.UUID(newBinding.ID.Bytes)
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     in.UserID,
		Action:     audit.ActionSwap,
		EntityType: audit.EntityTypeBinding,
		EntityID:   newBindingID,
		Before:     before,
		After:      after,
		Notes:      in.OperatorNotes,
		RequestID:  in.RequestID,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("swap: write audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("swap: commit: %w", err)
	}

	// 4. Defensive resolver invalidation (defense in depth on the 0017 NOTIFY
	//    trigger). Both dev_eui values get dropped because both meters are
	//    now resolved to a different binding state.
	if deps.Resolver != nil {
		if in.OutgoingDevEUI != "" {
			deps.Resolver.Invalidate(strings.ToLower(in.OutgoingDevEUI))
		}
		if in.IncomingDevEUI != "" {
			deps.Resolver.Invalidate(strings.ToLower(in.IncomingDevEUI))
		}
	}

	deps.Log.Info("swap committed",
		"metering_point_id", in.MeteringPointID,
		"old_binding_id", uuidFromPg(oldBinding.ID),
		"new_binding_id", newBindingID,
		"old_dev_eui", in.OutgoingDevEUI,
		"new_dev_eui", in.IncomingDevEUI,
		"applied_offset", appliedOffset.Text('g', 32),
		"override_used", in.OperatorOverride != nil,
	)
	return newBindingID, nil
}

// numericFromBigFloat encodes a *big.Float as a pgtype.Numeric by way of its
// text form. pgtype.Numeric.Scan(string) accepts decimal strings and parses
// them losslessly into the Int + Exp internal encoding.
func numericFromBigFloat(f *big.Float) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(f.Text('f', -1)); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("scan %q: %w", f.Text('f', -1), err)
	}
	return n, nil
}

// numericText renders a pgtype.Numeric as a decimal string for audit_log
// JSONB. Used for the swap before/after diff so a Phase 6 reviewer sees
// "12345.678" rather than the Int+Exp internal representation.
func numericText(n pgtype.Numeric) string {
	if !n.Valid {
		return ""
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		// Fall back to the raw integer form rather than panicking; the audit
		// row is still a record of "what we tried."
		if n.Int != nil {
			return n.Int.String()
		}
		return ""
	}
	return new(big.Float).SetFloat64(f.Float64).Text('g', 32)
}

// uuidFromPg returns the string form of a pgtype.UUID for inclusion in
// audit_log JSONB. NULL pgtype.UUID renders as empty string.
func uuidFromPg(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}
