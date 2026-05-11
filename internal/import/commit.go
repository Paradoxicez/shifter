package importpkg

// Commit pass — turns a preview import_job into committed rows. Per row:
// Serializable tx → CS CreateDevice + Keys/Activate → PG INSERT device →
// audit row (action='create', notes='bulk_import') → COMMIT. On CS failure
// best-effort CS DeleteDevice cleanup (fresh ctx) per Phase 2 D-16.
//
// After every valid row is processed, ONE envelope audit row writes
// (action='device.bulk_import', entity='import_job'). All N+1 rows share
// request_id = job_id (D-34) so a single WHERE request_id query
// reconstructs the import.
//
// Per D-Discretion #6 commit is synchronous; >10K rows can be deferred to
// River later. Phase 3's row-cap is 5000 anyway.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// CommitCSClient is the narrow CS contract the commit pass needs. Test
// fakes satisfy this without standing up a real CS container. Mirrors
// internal/device/handlers.CSDeviceClient + adds Activate.
type CommitCSClient interface {
	CreateDevice(ctx context.Context, in chirpstack.CreateDeviceInput) error
	CreateDeviceKeys(ctx context.Context, devEUI, appKey string) error
	ActivateDevice(ctx context.Context, in chirpstack.ActivateDeviceInput) error
	DeleteDevice(ctx context.Context, devEUI string) error
}

// CommitBootstrap matches the device-handler bootstrap interface so the
// commit pass can resolve the CS application_id once at job start.
type CommitBootstrap interface {
	EnsureTenantAndApplication(ctx context.Context) (tenantID, appID string, err error)
}

// CommitDeps bundles every handle the commit pass needs. Built once at
// app start and passed to Commit per request.
type CommitDeps struct {
	Pool      *pgxpool.Pool
	CS        CommitCSClient
	Bootstrap CommitBootstrap
	Log       *slog.Logger
}

// CommitSummary is the per-job aggregate the handler returns to the client
// after Commit. Counters match import_job.* columns.
type CommitSummary struct {
	JobID              uuid.UUID
	Total              int
	Created            int
	AlreadyExists      int
	Failed             int
	Invalid            int
	Valid              int
	EnvelopeAuditWritten bool
}

// Commit iterates the import_job's `valid` rows, attempts each one
// atomically, then writes the envelope audit row. Already-committed jobs
// short-circuit to their stored counters (idempotency: same job_id +
// re-commit returns the same summary).
//
// actorID — the admin user committing the job (audit user_id). Required.
func (d *CommitDeps) Commit(ctx context.Context, jobID uuid.UUID, actorID uuid.UUID) (CommitSummary, error) {
	if d.Pool == nil {
		return CommitSummary{}, fmt.Errorf("commit: nil pool")
	}
	if d.CS == nil {
		return CommitSummary{}, fmt.Errorf("commit: nil CS client")
	}
	if d.Bootstrap == nil {
		return CommitSummary{}, fmt.Errorf("commit: nil bootstrap")
	}

	q := sqlc.New(d.Pool)

	// 1. Load the job. The caller (handler) has already run ExpireIfStale
	// and rejected expired / non-preview jobs — but we re-check here as
	// defence in depth.
	job, err := q.GetImportJobByJobID(ctx, pgtypeUUID(jobID))
	if err != nil {
		return CommitSummary{}, fmt.Errorf("commit: load job: %w", err)
	}

	// Idempotency (D-06 + D-11): re-running commit on a committed job
	// returns the existing summary, no further work.
	if job.Status == sqlc.ImportJobStatusCommitted {
		return summaryFromJob(job), nil
	}
	if job.Status != sqlc.ImportJobStatusPreview {
		return CommitSummary{}, fmt.Errorf("commit: job status %q is not commitable", job.Status)
	}

	// 2. Bootstrap CS (idempotent — store-cached UUIDs short-circuit on
	// repeat calls). We need the application_id for CreateDevice.
	_, appID, err := d.Bootstrap.EnsureTenantAndApplication(ctx)
	if err != nil {
		return CommitSummary{}, fmt.Errorf("commit: bootstrap CS: %w", err)
	}

	// 3. Load every row, then iterate the valid ones.
	rows, err := q.ListImportJobRows(ctx, sqlc.ListImportJobRowsParams{
		ImportJobID: job.ID,
		Limit:       int32(job.TotalRows + 10),
		Offset:      0,
	})
	if err != nil {
		return CommitSummary{}, fmt.Errorf("commit: list rows: %w", err)
	}

	var created, alreadyExists, failed, invalid, validCount int
	for _, r := range rows {
		switch r.Status {
		case sqlc.ImportJobRowStatusValid:
			validCount++
		case sqlc.ImportJobRowStatusAlreadyExists:
			alreadyExists++
		case sqlc.ImportJobRowStatusInvalid:
			invalid++
		}
	}

	for _, r := range rows {
		if r.Status != sqlc.ImportJobRowStatusValid {
			continue
		}
		ok, devID, reason := d.commitRow(ctx, q, r, appID, actorID, jobID)
		if ok {
			created++
			_, _ = q.UpdateImportJobRowOutcome(ctx, sqlc.UpdateImportJobRowOutcomeParams{
				ID:              r.ID,
				Status:          sqlc.ImportJobRowStatusCreated,
				Reason:          nil,
				CreatedDeviceID: pgtypeUUID(devID),
			})
		} else {
			failed++
			reasonCopy := reason
			_, _ = q.UpdateImportJobRowOutcome(ctx, sqlc.UpdateImportJobRowOutcomeParams{
				ID:              r.ID,
				Status:          sqlc.ImportJobRowStatusFailed,
				Reason:          &reasonCopy,
				CreatedDeviceID: pgtype.UUID{Valid: false},
			})
		}
	}

	// 4. Update import_job counters + status. Use a transaction to attach
	// the envelope audit row atomically with the status flip.
	tx, err := d.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CommitSummary{}, fmt.Errorf("commit: begin envelope tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txQ := sqlc.New(tx)
	updated, err := txQ.UpdateImportJobToCommitted(ctx, sqlc.UpdateImportJobToCommittedParams{
		ID:                 job.ID,
		CreatedCount:       int32(created),
		AlreadyExistsCount: int32(alreadyExists),
		FailedCount:        int32(failed),
		InvalidCount:       int32(invalid),
		ValidCount:         int32(validCount),
	})
	if err != nil {
		return CommitSummary{}, fmt.Errorf("commit: update job status: %w", err)
	}

	// 5. Envelope audit row (D-33 / D-34). action=device.bulk_import,
	// entity_type=import_job, request_id=job_id.
	envelopeAfter := map[string]any{
		"total":          int(job.TotalRows),
		"created":        created,
		"already_exists": alreadyExists,
		"failed":         failed,
		"invalid":        invalid,
		"valid":          validCount,
		"job_id":         jobID.String(),
		"file_name":      job.FileName,
		"file_format":    job.FileFormat,
	}
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     actorID,
		Action:     audit.ActionBulkImport,
		EntityType: audit.EntityTypeImportJob,
		EntityID:   jobID,
		Before:     nil,
		After:      envelopeAfter,
		Notes:      "",
		RequestID:  jobID.String(), // D-34: request_id = job_id
	}); err != nil {
		return CommitSummary{}, fmt.Errorf("commit: envelope audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CommitSummary{}, fmt.Errorf("commit: tx commit: %w", err)
	}

	return CommitSummary{
		JobID:                jobID,
		Total:                int(updated.TotalRows),
		Created:              created,
		AlreadyExists:        alreadyExists,
		Failed:               failed,
		Invalid:              invalid,
		Valid:                validCount,
		EnvelopeAuditWritten: true,
	}, nil
}

// commitRow attempts a single valid row inside its own Serializable tx.
// Returns (ok, deviceID, reason). On any failure the CS-side device (if
// created) is cleaned up best-effort with a fresh context per Phase 2 D-16.
func (d *CommitDeps) commitRow(
	ctx context.Context,
	q *sqlc.Queries,
	r sqlc.ImportJobRow,
	appID string,
	actorID uuid.UUID,
	jobID uuid.UUID,
) (bool, uuid.UUID, string) {
	parsed := map[string]any{}
	if len(r.Parsed) > 0 {
		if err := json.Unmarshal(r.Parsed, &parsed); err != nil {
			return false, uuid.Nil, fmt.Sprintf("parsed_unmarshal:%v", err)
		}
	} else if len(r.RawPayload) > 0 {
		// Defensive fallback: dry-run should have populated `parsed`, but
		// if not, return a clear error.
		return false, uuid.Nil, "parsed_missing"
	}

	devEUI, _ := parsed["dev_eui"].(string)
	name, _ := parsed["name"].(string)
	mode, _ := parsed["activation_mode"].(string)
	profIDStr, _ := parsed["device_profile_id"].(string)
	appKey, _ := parsed["app_key"].(string)
	joinEUI, _ := parsed["join_eui"].(string)
	description, _ := parsed["description"].(string)
	devAddr, _ := parsed["dev_addr"].(string)
	nwkSKey, _ := parsed["nwk_s_key"].(string)
	appSKey, _ := parsed["app_s_key"].(string)

	if devEUI == "" || name == "" || mode == "" {
		return false, uuid.Nil, "parsed_incomplete"
	}

	profileID, err := uuid.Parse(profIDStr)
	if err != nil {
		return false, uuid.Nil, "invalid_device_profile_id"
	}

	// Resolve the CS device-profile UUID (sqlc query needs the PG profile
	// row to read cs_profile_id).
	prof, err := q.GetDeviceProfile(ctx, pgtypeUUID(profileID))
	if err != nil {
		return false, uuid.Nil, "device_profile_lookup_failed"
	}
	if !prof.CsProfileID.Valid {
		return false, uuid.Nil, "profile_not_synced"
	}
	csProfileIDStr := uuid.UUID(prof.CsProfileID.Bytes).String()

	// Open the per-row Serializable tx.
	tx, err := d.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, uuid.Nil, fmt.Sprintf("begin_tx:%v", err)
	}
	rolledBack := false
	defer func() {
		if !rolledBack {
			_ = tx.Rollback(ctx)
		}
	}()

	// CS CreateDevice.
	if err := d.CS.CreateDevice(ctx, chirpstack.CreateDeviceInput{
		DevEUI:          devEUI,
		ApplicationID:   appID,
		DeviceProfileID: csProfileIDStr,
		Name:            name,
		Description:     description,
		AppKey:          appKey, // forwarded to CS; never persisted in Shifter
		JoinEUI:         joinEUI,
	}); err != nil {
		if d.Log != nil {
			d.Log.Warn("commit: CS CreateDevice failed", "dev_eui", devEUI, "err", err)
		}
		return false, uuid.Nil, fmt.Sprintf("cs_create_failed:%v", err)
	}
	csCreated := true

	// Mode-specific second CS call.
	switch mode {
	case "OTAA":
		if err := d.CS.CreateDeviceKeys(ctx, devEUI, appKey); err != nil {
			if csCreated {
				d.cleanupCS(devEUI)
			}
			return false, uuid.Nil, fmt.Sprintf("cs_keys_failed:%v", err)
		}
	case "ABP":
		fcntUp := parseUint32(parsedString(parsed, "f_cnt_up"))
		fcntDown := parseUint32(parsedString(parsed, "f_cnt_down"))
		if err := d.CS.ActivateDevice(ctx, chirpstack.ActivateDeviceInput{
			DevEUI:   devEUI,
			DevAddr:  devAddr,
			NwkSKey:  nwkSKey,
			AppSKey:  appSKey,
			FCntUp:   fcntUp,
			FCntDown: fcntDown,
		}); err != nil {
			if csCreated {
				d.cleanupCS(devEUI)
			}
			return false, uuid.Nil, fmt.Sprintf("cs_activate_failed:%v", err)
		}
	}

	// PG INSERT device.
	txQ := sqlc.New(tx)
	var joinEUIPtr *string
	if joinEUI != "" {
		j := joinEUI
		joinEUIPtr = &j
	}
	var descPtr *string
	if description != "" {
		d2 := description
		descPtr = &d2
	}
	dev, err := txQ.CreateDevice(ctx, sqlc.CreateDeviceParams{
		DevEui:          devEUI,
		Name:            name,
		DeviceProfileID: pgtypeUUID(profileID),
		JoinEui:         joinEUIPtr,
		Description:     descPtr,
	})
	if err != nil {
		if csCreated {
			d.cleanupCS(devEUI)
		}
		return false, uuid.Nil, fmt.Sprintf("pg_insert_failed:%v", err)
	}

	// Per-device audit row (D-33: action='create', notes='bulk_import',
	// request_id=job_id).
	after := map[string]any{
		"dev_eui":           devEUI,
		"name":              name,
		"device_profile_id": profileID.String(),
		"join_eui":          joinEUI,
		"description":       description,
		"activation_mode":   mode,
	}
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     actorID,
		Action:     audit.ActionCreate,
		EntityType: audit.EntityTypeDevice,
		EntityID:   uuid.UUID(dev.ID.Bytes),
		Before:     nil,
		After:      after,
		Notes:      "bulk_import",
		RequestID:  jobID.String(), // D-34
	}); err != nil {
		if csCreated {
			d.cleanupCS(devEUI)
		}
		return false, uuid.Nil, fmt.Sprintf("audit_failed:%v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		if csCreated {
			d.cleanupCS(devEUI)
		}
		return false, uuid.Nil, fmt.Sprintf("tx_commit_failed:%v", err)
	}
	rolledBack = true // tx already committed; defer-rollback is a no-op.

	return true, uuid.UUID(dev.ID.Bytes), ""
}

func (d *CommitDeps) cleanupCS(devEUI string) {
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.CS.DeleteDevice(cleanup, devEUI); err != nil && d.Log != nil {
		d.Log.Warn("commit: CS cleanup failed", "dev_eui", devEUI, "err", err)
	}
}

func summaryFromJob(j sqlc.ImportJob) CommitSummary {
	return CommitSummary{
		JobID:                uuid.UUID(j.JobID.Bytes),
		Total:                int(j.TotalRows),
		Created:              int(j.CreatedCount),
		AlreadyExists:        int(j.AlreadyExistsCount),
		Failed:               int(j.FailedCount),
		Invalid:              int(j.InvalidCount),
		Valid:                int(j.ValidCount),
		EnvelopeAuditWritten: false, // envelope was written at first commit
	}
}

// parsedString reads a string field from the parsed JSONB map.
func parsedString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// parseUint32 best-effort parses an FCnt. Empty / unparseable → 0 (matches
// LoRaWAN default for fresh ABP devices).
func parseUint32(raw string) uint32 {
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

// pgtypeUUID is a tiny ctor for pgtype.UUID — pulled out so we don't repeat
// the `{Bytes: u, Valid: true}` litany at every call site.
func pgtypeUUID(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: u != uuid.Nil}
}

// errAlreadyDone is returned by helper paths that detect a no-op
// (idempotent re-commit). Reserved for future use; currently we short-
// circuit in Commit itself.
var errAlreadyDone = errors.New("import_job already committed")
