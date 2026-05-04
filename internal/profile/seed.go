package profile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/profile/codecs"
)

// RunSeedSync iterates ListUnsyncedProfiles (codec_js empty OR
// codec_js_synced_at NULL) and pushes each //go:embed-ed codec to ChirpStack.
//
// Idempotent: re-running is a no-op for already-synced profiles. After a
// successful CS push, MarkProfileSyncedToChirpStack records cs_profile_id +
// codec_js_synced_at = now() so the next call's ListUnsyncedProfiles excludes
// the row.
//
// Best-effort per Pitfall 9: this routine NEVER returns an error and NEVER
// blocks boot. CS reachability failures, tenant-not-bootstrapped, per-profile
// gRPC errors are all logged as warnings; the profile stays unsynced and the
// next boot retries. The resolver + ingest path do NOT depend on codec_js
// being synced — already-bound devices keep delivering uplinks even if
// RunSeedSync hasn't completed yet (CS itself runs the codec independently
// of Shifter; the ChirpStack-side codec is what matters for live ingest).
//
// Per CONTEXT D-09: codec_js source is delivered via //go:embed
// (Open Question #5 resolved in 02-RESEARCH.md), NOT inlined into the
// 0010_seed_profiles SQL migration.
func RunSeedSync(ctx context.Context, deps Deps) {
	if deps.Pool == nil {
		// Defensive — caller should never pass nil pool. Logged at
		// warn level rather than panicking.
		log := deps.Log
		if log == nil {
			log = slog.Default()
		}
		log.Warn("seed sync: nil pool, skipping")
		return
	}
	if deps.Log == nil {
		deps.Log = slog.Default()
	}

	q := sqlc.New(deps.Pool)
	unsynced, err := q.ListUnsyncedProfiles(ctx)
	if err != nil {
		deps.Log.Error("seed sync: list unsynced profiles failed", "err", err)
		return
	}
	if len(unsynced) == 0 {
		deps.Log.Debug("seed sync: nothing to do")
		return
	}

	// Pitfall 9 — short-circuit if CS hasn't been bootstrapped yet (D-28).
	// On a fresh install where the install wizard hasn't run, the
	// chirpstack_connection row exists with NULL cs_tenant_id; CS gRPC is
	// unusable until the wizard finishes. Log + return so boot proceeds.
	tenantID, _, err := deps.ConnStore.GetCSConnection(ctx)
	if err != nil {
		deps.Log.Warn("seed sync: read chirpstack_connection failed, retrying on next boot",
			"err", err, "unsynced_count", len(unsynced))
		return
	}
	if tenantID == "" {
		deps.Log.Warn("seed sync: chirpstack tenant not bootstrapped, retrying on next boot",
			"unsynced_count", len(unsynced))
		return
	}

	synced := 0
	skipped := 0
	for _, p := range unsynced {
		body := codecs.CodecBySlug(p.Slug)
		if body == "" {
			deps.Log.Warn("seed sync: no embedded codec for slug, skipping",
				"slug", p.Slug, "profile_id", uuidString(p.ID))
			skipped++
			continue
		}

		csProfileID, err := pushCodecToCS(ctx, deps.CSClient, p, tenantID, body)
		if err != nil {
			deps.Log.Warn("seed sync: CS push failed, retrying on next boot",
				"slug", p.Slug, "profile_id", uuidString(p.ID), "err", err)
			skipped++
			continue
		}

		if err := persistSyncResult(ctx, deps.Pool, p.ID, csProfileID, body); err != nil {
			deps.Log.Warn("seed sync: persist sync result failed, retrying on next boot",
				"slug", p.Slug, "profile_id", uuidString(p.ID), "err", err)
			skipped++
			continue
		}
		synced++
	}

	deps.Log.Info("seed sync complete",
		"synced", synced, "skipped", skipped, "total", len(unsynced))
}

// pushCodecToCS Creates the profile in CS when cs_profile_id is NULL, or
// Updates the existing profile otherwise. Returns the CS-side UUID (a fresh
// one for Create, the same one for Update).
func pushCodecToCS(ctx context.Context, cs CSProfileClient, p sqlc.DeviceProfile, tenantID, body string) (uuid.UUID, error) {
	if p.CsProfileID.Valid {
		csUUID := uuid.UUID(p.CsProfileID.Bytes)
		if err := cs.UpdateDeviceProfile(ctx, chirpstack.UpdateProfileInput{
			ID:      csUUID.String(),
			Name:    p.Name,
			CodecJS: body,
		}); err != nil {
			return uuid.Nil, fmt.Errorf("UpdateDeviceProfile: %w", err)
		}
		return csUUID, nil
	}

	region := stringPtrText(p.Region)
	if region == "" {
		region = "AS923_2" // INST-04 install-time default for new profiles
	}

	idStr, err := cs.CreateDeviceProfile(ctx, chirpstack.CreateProfileInput{
		TenantID:          tenantID,
		Name:              p.Name,
		Description:       fmt.Sprintf("Shifter seed profile %s (%s)", p.Slug, p.Vendor),
		Region:            region,
		MACVersion:        p.MacVersion,
		RegParamsRevision: "RP002_1_0_3",
		CodecJS:           body,
		SupportsOTAA:      true,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("CreateDeviceProfile: %w", err)
	}
	csUUID, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("CS returned non-UUID %q: %w", idStr, err)
	}
	return csUUID, nil
}

// persistSyncResult writes the codec_js body, cs_profile_id, and
// codec_js_synced_at = now() in a single Serializable transaction.
func persistSyncResult(ctx context.Context, pool interface {
	BeginTx(ctx context.Context, txOpts pgx.TxOptions) (pgx.Tx, error)
}, profileID pgtype.UUID, csUUID uuid.UUID, body string) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlc.New(tx)
	if err := q.SetProfileCodecJS(ctx, sqlc.SetProfileCodecJSParams{
		ID:      profileID,
		CodecJs: body,
	}); err != nil {
		return fmt.Errorf("SetProfileCodecJS: %w", err)
	}
	if err := q.MarkProfileSyncedToChirpStack(ctx, sqlc.MarkProfileSyncedToChirpStackParams{
		ID:          profileID,
		CsProfileID: pgtype.UUID{Bytes: csUUID, Valid: true},
	}); err != nil {
		return fmt.Errorf("MarkProfileSyncedToChirpStack: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// uuidString — defensive helper for log fields. NULL pgtype.UUID renders
// as empty string instead of "00000000-0000-0000-0000-000000000000".
func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

// ErrSeedTenantMissing is exported so cmd/serve can distinguish "CS not
// bootstrapped yet" from other failures if a future call site needs to
// degrade differently than RunSeedSync's silent-warn behavior.
var ErrSeedTenantMissing = errors.New("seed sync: chirpstack tenant not bootstrapped")
