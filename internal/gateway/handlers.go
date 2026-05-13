// Package gateway — HTTP handlers for the Phase 3 Gateway CRUD surface
// (D-29 + D-30 + D-32) plus the GW-01 list page with cached metrics.
//
// Every mutation handler runs the Phase 2 atomic CS+PG transaction shape
// (D-16 mirror):
//
//  1. Open a pgx.Serializable transaction.
//  2. Side-effect ChirpStack first (so PG can roll back on CS failure).
//  3. Run the sqlc mutation.
//  4. Write an audit_log row INSIDE the same tx via audit.WriteEntry.
//  5. Commit.
//
// On any failure between (2) and (5):
//
//   - CS error → tx.Rollback, 502 to operator.
//   - PG error AFTER CS Create succeeded → best-effort CS DeleteGateway
//     with a FRESH context (Pitfall 02-05). Operator sees 500.
//
// Archive (D-30 verbatim — user decision 2026-05-11) inverts the order
// to keep the snapshot atomic:
//
//  1. Capture the CS Gateway proto via chirpstackClient.GetGatewayProto.
//  2. Open Serializable tx.
//  3. q.ArchiveGateway writes archived_at + archived_reason +
//     archived_snapshot (the proto JSON).
//  4. chirpstackClient.DeleteGateway.
//  5. audit row.
//  6. tx.Commit.
//
// On CS DeleteGateway error → tx.Rollback + 502.
// On tx.Commit error AFTER CS Delete succeeded → best-effort CS
// CreateGatewayFromProto with FRESH ctx + the captured snapshot;
// warning log; 500 to operator (state recovered to "still in CS, still
// active in PG" so a retry is safe).
//
// Restore: read archived_snapshot → unmarshal → CS CreateGatewayFromProto
// → q.RestoreGateway clears archive columns → audit → commit. On
// tx.Commit error after CS recreate, best-effort CS DeleteGateway to
// keep "PG archived, CS gone" consistent.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// CSGatewayClient is the narrow ChirpStack contract every gateway handler
// depends on. *chirpstack.Client structurally satisfies it; tests substitute
// a fake without a live CS instance.
type CSGatewayClient interface {
	CreateGateway(ctx context.Context, in chirpstack.CreateGatewayInput) error
	CreateGatewayFromProto(ctx context.Context, gw *api.Gateway) error
	GetGatewayProto(ctx context.Context, gatewayID string) (*api.Gateway, error)
	UpdateGateway(ctx context.Context, in chirpstack.CreateGatewayInput) error
	DeleteGateway(ctx context.Context, gatewayID string) error
}

// CSBootstrapper exposes EnsureTenantAndApplication so the create-gateway
// handler can resolve the single global tenant ID per D-28. Mirrors the
// device.CSBootstrapper interface — same shape so cmd/serve can pass the
// same adapter.
type CSBootstrapper interface {
	EnsureTenantAndApplication(ctx context.Context) (tenantID, appID string, err error)
}

// MetricsCacheAccessor is the subset of *chirpstack.MetricsCache the list
// handler depends on. Tests substitute a fake without a real CS connection.
type MetricsCacheAccessor interface {
	Get(ctx context.Context, gatewayID string) (*api.GetGatewayMetricsResponse, error)
	Invalidate(gatewayID string)
}

// InstallStateReader exposes the install_state default region for D-03.
// The create-gateway dialog pre-selects this value; if a request omits
// `region` we read it server-side too so the dialog default and the API
// default cannot drift.
type InstallStateReader interface {
	GetLoRaWANRegionDefault(ctx context.Context) (string, error)
}

// CacheRefresherTrigger is the optional async refresher wired in Task 3.
// nil-safe — listGateways skips the trigger when Refresher is nil so the
// router test fixture can omit it.
type CacheRefresherTrigger interface {
	Trigger(ctx context.Context, gateways []GatewayRow)
}

// GatewayRow is the minimal projection the refresher consumes. Decoupled
// from sqlc.Gateway so the chirpstack package never imports sqlc.
type GatewayRow struct {
	ID        uuid.UUID
	GatewayID string
}

// Deps bundles the shared infra every Gateway handler needs.
type Deps struct {
	Pool         *pgxpool.Pool
	SessionMgr   *scs.SessionManager
	Log          *slog.Logger
	CS           CSGatewayClient
	Bootstrap    CSBootstrapper
	MetricsCache MetricsCacheAccessor
	InstallState InstallStateReader
	Refresher    CacheRefresherTrigger
}

// fallbackRegion is the Thailand default per CLAUDE.md when install_state
// is unavailable (e.g. early-boot integration tests). Matches the AS923_2
// install-wizard pre-select.
const fallbackRegion = "as923_2"

// RegisterRoutes mounts /api/gateways under r.
//
// Route table:
//
//	GET    /api/gateways                gateway.read    — admin + viewer
//	GET    /api/gateways/archived       gateway.read    — admin + viewer
//	GET    /api/gateways/{id}           gateway.read    — admin + viewer
//	POST   /api/gateways                gateway.create  — admin only
//	PATCH  /api/gateways/{id}           gateway.update  — admin only
//	POST   /api/gateways/{id}/archive   gateway.archive — admin only
//	POST   /api/gateways/{id}/restore   gateway.restore — admin only
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Route("/api/gateways", func(r chi.Router) {
		// Read group — admin + viewer.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionGatewayRead))
			rt.Get("/", listGateways(deps))
			rt.Get("/archived", listArchivedGateways(deps))
			rt.Get("/{id}", getGateway(deps))
		})
		// Mutate groups — admin only. Per-route Use(...) so each verb is
		// gated by the action-specific RequireAction (defense-in-depth on
		// top of the in-handler auth.Can checks).
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionGatewayCreate))
			rt.Post("/", createGateway(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionGatewayUpdate))
			rt.Patch("/{id}", updateGateway(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionGatewayArchive))
			rt.Post("/{id}/archive", archiveGateway(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionGatewayRestore))
			rt.Post("/{id}/restore", restoreGateway(deps))
		})
	})
}

// ----- request shapes -----------------------------------------------------

// CreateGatewayRequest is the JSON body of POST /api/gateways.
// `gateway_id` is the lowercase 16-hex EUI64 (the schema CHECK enforces).
// `region` is optional — when empty the handler reads the install default
// (D-03). `tenant_id` is INTENTIONALLY ignored if present: T-3-31
// mitigates client-supplied tenant by always using the single-global
// tenant from chirpstack_connection.
type CreateGatewayRequest struct {
	GatewayID   string            `json:"gateway_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Region      string            `json:"region,omitempty"`
	Lat         *float64          `json:"lat,omitempty"`
	Lng         *float64          `json:"lng,omitempty"`
	Altitude    *float64          `json:"altitude,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// UpdateGatewayRequest is the JSON body of PATCH /api/gateways/{id}.
// `gateway_id` is intentionally NOT updatable — changing the EUI is a new
// gateway, not an edit.
type UpdateGatewayRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Region      string            `json:"region,omitempty"`
	Lat         *float64          `json:"lat,omitempty"`
	Lng         *float64          `json:"lng,omitempty"`
	Altitude    *float64          `json:"altitude,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// ArchiveGatewayRequest is the JSON body of POST /api/gateways/{id}/archive.
// Reason defaults to "operator decommission" if omitted (D-31).
type ArchiveGatewayRequest struct {
	Reason string `json:"reason,omitempty"`
}

// ----- handlers -----------------------------------------------------------

func listGateways(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := paginationFromQuery(r)
		q := sqlc.New(deps.Pool)
		rows, err := q.ListGatewaysActive(r.Context(), sqlc.ListGatewaysActiveParams{
			Limit: limit, Offset: offset,
		})
		if err != nil {
			internalError(deps.Log, w, "list active gateways", err)
			return
		}
		total, err := q.CountGatewaysActive(r.Context())
		if err != nil {
			internalError(deps.Log, w, "count active gateways", err)
			return
		}

		items := make([]map[string]any, 0, len(rows))
		gwRows := make([]GatewayRow, 0, len(rows))
		for _, gw := range rows {
			items = append(items, gatewayToJSON(gw, deps))
			gwRows = append(gwRows, GatewayRow{
				ID:        uuid.UUID(gw.ID.Bytes),
				GatewayID: gw.GatewayID,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"total": total,
			"items": items,
		})

		// Best-effort async refresh AFTER the response has been written so
		// the operator sees the response immediately and the next refresh
		// has fresh stats. The refresher uses a fresh ctx internally — the
		// request ctx is gone after the response is flushed.
		if deps.Refresher != nil {
			go deps.Refresher.Trigger(context.Background(), gwRows)
		}
	}
}

func listArchivedGateways(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := paginationFromQuery(r)
		q := sqlc.New(deps.Pool)
		rows, err := q.ListGatewaysIncludingArchived(r.Context(), sqlc.ListGatewaysIncludingArchivedParams{
			Limit: limit, Offset: offset,
		})
		if err != nil {
			internalError(deps.Log, w, "list archived gateways", err)
			return
		}
		// Filter to only archived rows for the dedicated /archived endpoint —
		// the underlying query returns active+archived ordered by archive
		// time. Keeping the SQL "include all" lets future plans add a query
		// param for the unified "Show archived" toggle in one handler.
		out := make([]map[string]any, 0, len(rows))
		for _, gw := range rows {
			if gw.ArchivedAt.Valid {
				out = append(out, gatewayToJSON(gw, deps))
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getGateway(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		row, err := q.GetGateway(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "get gateway", err)
			return
		}
		writeJSON(w, http.StatusOK, gatewayToJSON(row, deps))
	}
}

// createGateway implements the D-29 atomic CS+PG add-gateway flow.
//
// Order: CS Create first → PG insert → audit → commit. On PG / audit / commit
// failure, best-effort CS DeleteGateway with FRESH ctx.
func createGateway(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionGatewayCreate)
		if !ok {
			return
		}
		var in CreateGatewayRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}

		// Server-side validation.
		gwID := strings.ToLower(strings.TrimSpace(in.GatewayID))
		if !isHex(gwID, 16) {
			writeJSON(w, http.StatusBadRequest, errorResp{
				Error: "invalid_gateway_id", Detail: "gateway_id must be 16 lowercase hex chars",
			})
			return
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: "name is required"})
			return
		}
		if err := validateLatLng(in.Lat, in.Lng); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: err.Error()})
			return
		}

		// D-03 region default. Fall back to AS923_2 (Thailand) when the
		// install-state reader is absent or returns an empty string —
		// install_state may be cleared post-wizard in some test fixtures.
		region := strings.TrimSpace(in.Region)
		if region == "" {
			region = resolveInstallRegion(r.Context(), deps)
		}

		// Bootstrap CS tenant + application (idempotent). T-3-31: ignore
		// any client-supplied tenant_id — always use the server-side value.
		var tenantID string
		if deps.Bootstrap != nil {
			t, _, berr := deps.Bootstrap.EnsureTenantAndApplication(r.Context())
			if berr != nil {
				if deps.Log != nil {
					deps.Log.Error("createGateway: bootstrap CS", "err", berr)
				}
				writeJSON(w, http.StatusBadGateway, errorResp{
					Error: "chirpstack_unreachable", Detail: berr.Error(),
				})
				return
			}
			tenantID = t
		}

		csInput := chirpstack.CreateGatewayInput{
			GatewayID:   gwID,
			Name:        name,
			Description: in.Description,
			Region:      region,
			Lat:         derefFloat(in.Lat),
			Lng:         derefFloat(in.Lng),
			Altitude:    derefFloat(in.Altitude),
			Tags:        in.Tags,
			TenantID:    tenantID,
		}

		// 1. CS CreateGateway. On error → 502, no rollback needed (PG not touched).
		if err := deps.CS.CreateGateway(r.Context(), csInput); err != nil {
			if deps.Log != nil {
				deps.Log.Error("createGateway: CS Create", "err", err, "gateway_id", gwID)
			}
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error: "cs_create_failed", Detail: err.Error(),
			})
			return
		}
		csCreated := true

		// 2. Open Serializable tx for PG mutation + audit.
		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			if csCreated {
				cleanupCSGateway(deps, gwID)
			}
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		txQ := sqlc.New(tx)

		tagsJSON, err := encodeTags(in.Tags)
		if err != nil {
			cleanupCSGateway(deps, gwID)
			internalError(deps.Log, w, "encode tags", err)
			return
		}

		row, err := txQ.CreateGateway(r.Context(), sqlc.CreateGatewayParams{
			GatewayID:   gwID,
			Name:        name,
			Description: nullableStr(in.Description),
			Region:      region,
			Lat:         in.Lat,
			Lng:         in.Lng,
			Altitude:    in.Altitude,
			Tags:        tagsJSON,
			CsTenantID:  nullableStr(tenantID),
		})
		if err != nil {
			cleanupCSGateway(deps, gwID)
			if isUniqueViolation(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "duplicate_gateway_id"})
				return
			}
			if isConstraintViolation(err) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "constraint", Detail: err.Error()})
				return
			}
			internalError(deps.Log, w, "insert gateway", err)
			return
		}

		// 3. Audit inside the same tx.
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionGatewayCreate,
			EntityType: audit.EntityTypeGateway,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     nil,
			After: map[string]any{
				"gateway_id": row.GatewayID,
				"name":       row.Name,
				"region":     row.Region,
				"lat":        in.Lat,
				"lng":        in.Lng,
				"tags":       in.Tags,
			},
			RequestID: middleware.GetReqID(r.Context()),
		}); err != nil {
			cleanupCSGateway(deps, gwID)
			internalError(deps.Log, w, "audit create gateway", err)
			return
		}

		// 4. Commit. On commit failure → CS rollback (best-effort).
		if err := tx.Commit(r.Context()); err != nil {
			cleanupCSGateway(deps, gwID)
			internalError(deps.Log, w, "commit create gateway", err)
			return
		}
		writeJSON(w, http.StatusCreated, gatewayToJSON(row, deps))
	}
}

// updateGateway implements PATCH /api/gateways/{id}. CS Update + PG Update
// atomic via the Serializable tx ordering: PG first (so a CS error rolls
// back PG); on commit failure we accept the divergence (CS already
// updated, PG didn't) — the operator can retry idempotently.
func updateGateway(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionGatewayUpdate)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		var in UpdateGatewayRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: "name is required"})
			return
		}
		if err := validateLatLng(in.Lat, in.Lng); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: err.Error()})
			return
		}

		// Resolve current row (need gateway_id for the CS call + before-state
		// for the audit diff).
		q := sqlc.New(deps.Pool)
		existing, err := q.GetGateway(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load existing gateway", err)
			return
		}
		if existing.ArchivedAt.Valid {
			writeJSON(w, http.StatusConflict, errorResp{Error: "archived"})
			return
		}
		region := strings.TrimSpace(in.Region)
		if region == "" {
			region = existing.Region
		}

		// CS Update is full-replace semantics — caller passes the FULL
		// shape. Use the existing tenant id; we never let the client change it.
		var tenantID string
		if existing.CsTenantID != nil {
			tenantID = *existing.CsTenantID
		}
		csInput := chirpstack.CreateGatewayInput{
			GatewayID:   existing.GatewayID,
			Name:        name,
			Description: in.Description,
			Region:      region,
			Lat:         derefFloat(in.Lat),
			Lng:         derefFloat(in.Lng),
			Altitude:    derefFloat(in.Altitude),
			Tags:        in.Tags,
			TenantID:    tenantID,
		}

		if err := deps.CS.UpdateGateway(r.Context(), csInput); err != nil {
			if deps.Log != nil {
				deps.Log.Error("updateGateway: CS Update", "err", err, "gateway_id", existing.GatewayID)
			}
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error: "cs_update_failed", Detail: err.Error(),
			})
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		txQ := sqlc.New(tx)

		tagsJSON, err := encodeTags(in.Tags)
		if err != nil {
			internalError(deps.Log, w, "encode tags", err)
			return
		}

		row, err := txQ.UpdateGateway(r.Context(), sqlc.UpdateGatewayParams{
			ID:          pgUUID(id),
			Name:        name,
			Description: nullableStr(in.Description),
			Region:      region,
			Lat:         in.Lat,
			Lng:         in.Lng,
			Altitude:    in.Altitude,
			Tags:        tagsJSON,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found_or_archived"})
			return
		}
		if err != nil {
			if isConstraintViolation(err) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "constraint", Detail: err.Error()})
				return
			}
			internalError(deps.Log, w, "update gateway", err)
			return
		}

		before := map[string]any{
			"name":   existing.Name,
			"region": existing.Region,
		}
		after := map[string]any{
			"name":   row.Name,
			"region": row.Region,
		}
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionGatewayUpdate,
			EntityType: audit.EntityTypeGateway,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     before,
			After:      after,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit update gateway", err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit update gateway", err)
			return
		}
		// Stats cache is unaffected by metadata-only updates, but invalidate
		// to be defensive: if a future plan starts persisting region or tags
		// to the cache row this is the single safe choice.
		if deps.MetricsCache != nil {
			deps.MetricsCache.Invalidate(row.GatewayID)
		}
		writeJSON(w, http.StatusOK, gatewayToJSON(row, deps))
	}
}

// archiveGateway implements D-30 verbatim (user decision 2026-05-11):
//
//  1. Capture CS Gateway proto via GetGatewayProto BEFORE any mutation.
//  2. Open Serializable tx.
//  3. q.ArchiveGateway writes archived_at + archived_reason + archived_snapshot.
//  4. d.CS.DeleteGateway.
//  5. audit row.
//  6. tx.Commit.
//
// CS Delete error → tx.Rollback + 502.
// Commit error AFTER CS Delete succeeded → best-effort CS CreateGatewayFromProto
// with fresh ctx + the captured snapshot; warning log; 500 to operator.
func archiveGateway(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionGatewayArchive)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		var in ArchiveGatewayRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
				return
			}
		}
		reason := strings.TrimSpace(in.Reason)
		if reason == "" {
			reason = "operator decommission"
		}

		// Load existing row — must exist + not already archived.
		q := sqlc.New(deps.Pool)
		existing, err := q.GetGateway(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load gateway", err)
			return
		}
		if existing.ArchivedAt.Valid {
			writeJSON(w, http.StatusConflict, errorResp{Error: "already_archived"})
			return
		}

		// 1. Snapshot the CS Gateway proto pre-mutation.
		snapshotJSON, snapshotProto := snapshotCSGateway(r.Context(), deps, existing.GatewayID)

		// 2. Open the Serializable tx.
		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		txQ := sqlc.New(tx)

		// 3. PG archive (writes archived_snapshot).
		row, err := txQ.ArchiveGateway(r.Context(), sqlc.ArchiveGatewayParams{
			ID:             pgUUID(id),
			ArchivedReason: &reason,
			ArchivedSnapshot: snapshotJSON,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusConflict, errorResp{Error: "already_archived"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "archive gateway", err)
			return
		}

		// 4. CS DeleteGateway. On error → tx.Rollback + 502.
		if err := deps.CS.DeleteGateway(r.Context(), existing.GatewayID); err != nil {
			if errors.Is(err, chirpstack.ErrNotFound) {
				// CS already gone — treat as success path; the snapshot may
				// be "null" but PG-side archive is still valid.
				if deps.Log != nil {
					deps.Log.Warn("archiveGateway: CS gateway already absent",
						"gateway_id", existing.GatewayID)
				}
			} else {
				if deps.Log != nil {
					deps.Log.Error("archiveGateway: CS DeleteGateway",
						"err", err, "gateway_id", existing.GatewayID)
				}
				writeJSON(w, http.StatusBadGateway, errorResp{
					Error: "cs_delete_gateway_failed", Detail: err.Error(),
				})
				return
			}
		}

		// 5. Audit row inside same tx.
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionGatewayArchive,
			EntityType: audit.EntityTypeGateway,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     map[string]any{"archived_at": nil},
			After: map[string]any{
				"archived_at":     timestamptzText(row.ArchivedAt),
				"archived_reason": reason,
			},
			Notes:     reason,
			RequestID: middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit archive gateway", err)
			return
		}

		// 6. Commit. On commit failure AFTER CS Delete succeeded — best-effort
		// CS re-create from snapshot (D-30 verbatim recovery branch).
		if err := tx.Commit(r.Context()); err != nil {
			recoverArchiveFromSnapshot(deps, existing.GatewayID, snapshotProto)
			internalError(deps.Log, w, "commit archive gateway", err)
			return
		}
		if deps.MetricsCache != nil {
			deps.MetricsCache.Invalidate(existing.GatewayID)
		}
		writeJSON(w, http.StatusOK, gatewayToJSON(row, deps))
	}
}

// restoreGateway implements D-30 verbatim restore: re-create the gateway in
// CS from archived_snapshot, then clear PG archive columns atomic.
func restoreGateway(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionGatewayRestore)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}

		q := sqlc.New(deps.Pool)
		existing, err := q.GetGateway(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load gateway", err)
			return
		}
		if !existing.ArchivedAt.Valid {
			writeJSON(w, http.StatusConflict, errorResp{Error: "not_archived"})
			return
		}
		if len(existing.ArchivedSnapshot) == 0 || string(existing.ArchivedSnapshot) == "null" {
			writeJSON(w, http.StatusConflict, errorResp{
				Error: "cannot_restore_without_snapshot",
				Detail: "archived_snapshot is empty; gateway must be re-created manually",
			})
			return
		}

		// Unmarshal snapshot → api.Gateway proto.
		var proto api.Gateway
		if err := protojson.Unmarshal(existing.ArchivedSnapshot, &proto); err != nil {
			internalError(deps.Log, w, "unmarshal snapshot", err)
			return
		}

		// 1. CS CreateGatewayFromProto BEFORE the PG tx — if CS rejects we
		// don't touch PG. (Symmetric inverse of the archive path.)
		if err := deps.CS.CreateGatewayFromProto(r.Context(), &proto); err != nil {
			if deps.Log != nil {
				deps.Log.Error("restoreGateway: CS Create from snapshot",
					"err", err, "gateway_id", existing.GatewayID)
			}
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error: "cs_create_gateway_failed", Detail: err.Error(),
			})
			return
		}

		// 2. Open Serializable tx.
		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			cleanupCSGateway(deps, existing.GatewayID)
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		txQ := sqlc.New(tx)

		row, err := txQ.RestoreGateway(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			cleanupCSGateway(deps, existing.GatewayID)
			writeJSON(w, http.StatusConflict, errorResp{Error: "not_archived"})
			return
		}
		if err != nil {
			cleanupCSGateway(deps, existing.GatewayID)
			internalError(deps.Log, w, "restore gateway", err)
			return
		}

		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionGatewayRestore,
			EntityType: audit.EntityTypeGateway,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     map[string]any{"archived_at": "non-null"},
			After:      map[string]any{"archived_at": nil, "archived_reason": nil},
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			cleanupCSGateway(deps, existing.GatewayID)
			internalError(deps.Log, w, "audit restore gateway", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			cleanupCSGateway(deps, existing.GatewayID)
			internalError(deps.Log, w, "commit restore gateway", err)
			return
		}
		if deps.MetricsCache != nil {
			deps.MetricsCache.Invalidate(row.GatewayID)
		}
		writeJSON(w, http.StatusOK, gatewayToJSON(row, deps))
	}
}

// ----- helpers ------------------------------------------------------------

// snapshotCSGateway calls CS.GetGatewayProto and serialises the proto via
// protojson. Returns the JSON bytes + the proto pointer for the recovery
// branch. On NotFound (CS already gone) returns []byte("null") so the
// archive row still records the attempt and the restore-without-snapshot
// path triggers cleanly.
func snapshotCSGateway(ctx context.Context, deps Deps, gatewayID string) ([]byte, *api.Gateway) {
	gw, err := deps.CS.GetGatewayProto(ctx, gatewayID)
	if err != nil {
		if errors.Is(err, chirpstack.ErrNotFound) {
			return []byte("null"), nil
		}
		if deps.Log != nil {
			deps.Log.Warn("archiveGateway: GetGatewayProto failed; snapshot will be null",
				"err", err, "gateway_id", gatewayID)
		}
		return []byte("null"), nil
	}
	js, mErr := protojson.Marshal(gw)
	if mErr != nil {
		if deps.Log != nil {
			deps.Log.Warn("archiveGateway: protojson marshal failed",
				"err", mErr, "gateway_id", gatewayID)
		}
		return []byte("null"), gw
	}
	return js, gw
}

// recoverArchiveFromSnapshot is the D-30 verbatim recovery branch for the
// archive path: if tx.Commit fails AFTER CS DeleteGateway succeeded, the
// gateway is gone from CS but PG never got the archive write. Best-effort
// re-create it in CS from the snapshot so the operator can retry without
// the gateway disappearing.
//
// Fresh ctx (Pitfall 02-05) — caller's ctx may already be cancelled. Errors
// are intentionally swallowed: the operator already sees a 500 from the
// commit failure; a noisy double-error obscures the real cause.
func recoverArchiveFromSnapshot(deps Deps, gatewayID string, snapshotProto *api.Gateway) {
	if snapshotProto == nil {
		// Snapshot was null (e.g. CS was already gone before archive) —
		// nothing to recover.
		return
	}
	recoveryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := deps.CS.CreateGatewayFromProto(recoveryCtx, snapshotProto); err != nil {
		if deps.Log != nil {
			deps.Log.Warn("decommission recovery: CS re-create failed",
				"gateway_id", gatewayID, "err", err)
		}
		return
	}
	if deps.Log != nil {
		deps.Log.Warn("decommission recovery: PG commit failed; CS gateway re-created from snapshot",
			"gateway_id", gatewayID)
	}
}

// cleanupCSGateway is the best-effort CS rollback used by createGateway +
// restoreGateway when PG fails after CS Create succeeded. Mirrors the
// device.cleanupCS pattern (Pitfall 02-05).
func cleanupCSGateway(deps Deps, gatewayID string) {
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := deps.CS.DeleteGateway(cleanup, gatewayID); err != nil && deps.Log != nil {
		deps.Log.Warn("gateway handler: CS cleanup failed",
			"err", err, "gateway_id", gatewayID)
	}
}

// resolveInstallRegion reads the install state region default (D-03) or
// falls back to the Thailand AS923_2 sub-band. Phase 1 install_state may
// have been deleted post-wizard — the InstallStateReader interface can
// return ErrNoRows or "" in that case, both of which trigger the fallback.
func resolveInstallRegion(ctx context.Context, deps Deps) string {
	if deps.InstallState == nil {
		return fallbackRegion
	}
	r, err := deps.InstallState.GetLoRaWANRegionDefault(ctx)
	if err != nil || strings.TrimSpace(r) == "" {
		return fallbackRegion
	}
	return strings.TrimSpace(r)
}

// paginationFromQuery extracts limit / offset URL params with safe defaults.
// T-3-33: cap LIMIT at 200 so a malicious client cannot fetch the entire
// gateway table in one call.
func paginationFromQuery(r *http.Request) (int32, int32) {
	const (
		defaultLimit int32 = 50
		maxLimit     int32 = 200
	)
	limit := defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := parseInt32(v); err == nil && parsed > 0 {
			limit = parsed
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}
	var offset int32
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := parseInt32(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

func parseInt32(s string) (int32, error) {
	var n int32
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// encodeTags JSON-encodes the tags map for storage in the JSONB column.
// Returns []byte("{}") for a nil/empty map so the column never holds NULL.
func encodeTags(tags map[string]string) ([]byte, error) {
	if len(tags) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(tags)
}

func validateLatLng(lat, lng *float64) error {
	if lat != nil && (*lat < -90 || *lat > 90) {
		return fmt.Errorf("lat must be in [-90, 90] (got %v)", *lat)
	}
	if lng != nil && (*lng < -180 || *lng > 180) {
		return fmt.Errorf("lng must be in [-180, 180] (got %v)", *lng)
	}
	return nil
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	if s != strings.ToLower(s) {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// ----- shared helpers (mirror site/handlers.go patterns) -------------------

type errorResp struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func internalError(log *slog.Logger, w http.ResponseWriter, op string, err error) {
	if log != nil {
		log.Error("gateway handler", "op", op, "err", err)
	}
	writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, name)
	id, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_id"})
		return uuid.Nil, false
	}
	return id, true
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func requireAdmin(sm *scs.SessionManager, w http.ResponseWriter, r *http.Request, action auth.Action) (auth.User, bool) {
	user, ok := auth.GetUser(r.Context(), sm)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
		return auth.User{}, false
	}
	if !auth.Can(&user, action, nil) {
		writeJSON(w, http.StatusForbidden, errorResp{Error: "forbidden"})
		return auth.User{}, false
	}
	return user, true
}

func mustParseUUID(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return u
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") || strings.Contains(msg, "violates unique constraint")
}

func isConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23514") ||
		strings.Contains(msg, "23505") ||
		strings.Contains(msg, "23503") ||
		strings.Contains(msg, "violates check constraint") ||
		strings.Contains(msg, "violates unique constraint") ||
		strings.Contains(msg, "violates foreign key constraint")
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// ----- JSON serialization -------------------------------------------------

func gatewayToJSON(g sqlc.Gateway, deps Deps) map[string]any {
	out := map[string]any{
		"id":               uuidString(g.ID),
		"gateway_id":       g.GatewayID,
		"name":             g.Name,
		"description":      derefString(g.Description),
		"region":           g.Region,
		"lat":              g.Lat,
		"lng":              g.Lng,
		"altitude":         g.Altitude,
		"tags":             tagsToJSON(g.Tags),
		"archived_at":      timestamptzText(g.ArchivedAt),
		"archived_reason":  derefString(g.ArchivedReason),
		"created_at":       timestamptzText(g.CreatedAt),
		"updated_at":       timestamptzText(g.UpdatedAt),
	}
	// Surface the cached stats columns (D-02). Only include them when
	// stats_refreshed_at is non-NULL so freshly-created gateways don't
	// show stale zero counts.
	if g.StatsRefreshedAt.Valid {
		out["stats_refreshed_at"] = timestamptzText(g.StatsRefreshedAt)
		out["stats_rx_24h"] = g.StatsRx24h
		out["stats_tx_24h"] = g.StatsTx24h
		out["stats_tx_ok_24h"] = g.StatsTxOk24h
		out["stats_sparkline"] = rawJSON(g.StatsSparkline)
	}
	// Surface live-merged last_seen_at + derived state (UI status badge).
	// last_seen_at is written by the async cache refresher after a CS
	// GetGateway round-trip, so it lags the wire by at most one TTL window.
	out["last_seen_at"] = timestamptzText(g.LastSeenAt)
	out["state"] = deriveGatewayState(g.LastSeenAt)
	_ = deps // reserved for future use (e.g. region display lookup)
	return out
}

// deriveGatewayState maps last_seen_at into the wire-shape state string the
// SPA renders. The thresholds mirror chirpstack's own UI conventions:
//
//	last_seen <= 5 min ago  → ONLINE
//	last_seen <= 24h ago    → OFFLINE
//	null / older            → NEVER_SEEN
//
// Tuning these requires only changing the constants below.
func deriveGatewayState(lastSeen pgtype.Timestamptz) string {
	if !lastSeen.Valid {
		return "NEVER_SEEN"
	}
	age := time.Since(lastSeen.Time)
	if age <= 5*time.Minute {
		return "ONLINE"
	}
	if age <= 24*time.Hour {
		return "OFFLINE"
	}
	return "NEVER_SEEN"
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func derefString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func timestamptzText(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05.000000Z")
}

func tagsToJSON(b []byte) any {
	if len(b) == 0 {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]string{}
	}
	return m
}

func rawJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	return v
}
