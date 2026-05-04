// Package swap — HTTP handler for the meter-swap commit (DATA-04).
//
// This file is the HTTP layer; CommitSwap (commit.go) owns the Serializable
// transaction and audit-in-tx semantics. Handlers here only:
//
//  1. Decode + validate the request body (decimal text → big.Float).
//  2. Look up the active binding + outgoing/incoming devices to fill SwapInput.
//  3. Capture user_id from session + request_id from chi middleware.
//  4. Call CommitSwap and translate its errors to HTTP status codes:
//     - pgconn 23P01 (exclusion_violation) → 409 concurrent_swap
//     - pgx.ErrNoRows                       → 404 (no active binding / device)
//     - other err                           → 500
//
// The swap route is mounted under /api/metering-points/{id}/swap. The
// metering-points package owns CRUD on /api/metering-points/* — chi composes
// the prefix correctly: this RegisterRoutes adds the /:id/swap subroute
// without conflicting with meteringpoint.RegisterRoutes.
package swap

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// HTTPDeps bundles the shared infra a swap HTTP handler call needs.
// Distinct from swap.Deps (which is the lower-level CommitSwap input shape)
// because handlers also need the SessionMgr for RBAC and auth.UserIDFromSession.
type HTTPDeps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger

	// Resolver is optional — passed through to swap.Deps for invalidation.
	// nil tolerated (CommitSwap will skip the defensive cache invalidation
	// and rely on the 0017 NOTIFY trigger only).
	Resolver Invalidator
}

// SwapRequest is the JSON body of POST /api/metering-points/{id}/swap.
// Decimal fields are TEXT to preserve precision (math/big.Float at 128 bits).
type SwapRequest struct {
	IncomingDeviceID string  `json:"incoming_device_id"`           // UUID required
	OutgoingReadingR string  `json:"outgoing_reading_r"`           // decimal text required
	IncomingInitialN string  `json:"incoming_initial_n"`           // decimal text optional, default "0"
	OperatorOverride *string `json:"operator_override,omitempty"`  // decimal text optional
	OperatorNotes    string  `json:"operator_notes,omitempty"`
}

// RegisterRoutes mounts the swap endpoint on the given chi router.
//
// Route table:
//
//	POST /api/metering-points/{id}/swap   meter.swap   — admin only
//
// The chi r.Group wrapper applies auth.RequireAction(meter.swap) before
// the handler runs. The handler ALSO calls requireAdmin internally as
// defense-in-depth (mirrors site/handlers.go pattern).
func RegisterRoutes(r chi.Router, deps HTTPDeps) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionMeterSwap))
		rt.Post("/api/metering-points/{id}/swap", swapHandler(deps))
	})
}

// swapHandler decodes the body, fetches the active binding + devices,
// builds SwapInput, and calls CommitSwap. Errors map to HTTP status codes
// as documented in the package preamble.
func swapHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionMeterSwap)
		if !ok {
			return
		}

		mpID, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}

		var in SwapRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "invalid JSON"})
			return
		}

		// Required body fields.
		if in.IncomingDeviceID == "" {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "incoming_device_id is required"})
			return
		}
		if in.OutgoingReadingR == "" {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "outgoing_reading_r is required"})
			return
		}

		incomingDevID, err := uuid.Parse(in.IncomingDeviceID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "incoming_device_id is not a UUID"})
			return
		}

		outgoingR, err := parseBigFloat(in.OutgoingReadingR)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "outgoing_reading_r: " + err.Error()})
			return
		}

		// Default IncomingInitialN to 0 when empty.
		incomingNStr := in.IncomingInitialN
		if incomingNStr == "" {
			incomingNStr = "0"
		}
		incomingN, err := parseBigFloat(incomingNStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "incoming_initial_n: " + err.Error()})
			return
		}

		var override *big.Float
		if in.OperatorOverride != nil && *in.OperatorOverride != "" {
			override, err = parseBigFloat(*in.OperatorOverride)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "operator_override: " + err.Error()})
				return
			}
		}

		// Look up the active binding for this MP (for OutgoingBindingID +
		// outgoing dev_eui). pgxpool query — no tx needed; CommitSwap opens
		// its own Serializable tx.
		q := sqlc.New(deps.Pool)
		activeBinding, err := q.GetActiveBindingByMPID(r.Context(), pgUUID(mpID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "no_active_binding"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "get active binding", err)
			return
		}

		// Outgoing dev_eui — schema FK guarantees the device row exists.
		outDev, err := q.GetDevice(r.Context(), activeBinding.DeviceID)
		if err != nil {
			internalError(deps.Log, w, "load outgoing device", err)
			return
		}

		// Incoming device — body-supplied id; user error if not found.
		inDev, err := q.GetDevice(r.Context(), pgUUID(incomingDevID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "incoming_device_not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load incoming device", err)
			return
		}

		// Operator id — session user is admin (requireAdmin already verified).
		operatorID := mustParseUUID(user.ID)

		// Build SwapInput. ConfirmTime captured AT THIS POINT (D-14).
		swapIn := SwapInput{
			UserID:            operatorID,
			RequestID:         middleware.GetReqID(r.Context()),
			MeteringPointID:   mpID,
			OutgoingBindingID: uuid.UUID(activeBinding.ID.Bytes),
			OutgoingDevEUI:    outDev.DevEui,
			IncomingDeviceID:  incomingDevID,
			IncomingDevEUI:    inDev.DevEui,
			ConfirmTime:       time.Now().UTC(),
			OutgoingReadingR:  outgoingR,
			IncomingInitialN:  incomingN,
			OperatorOverride:  override,
			OperatorNotes:     in.OperatorNotes,
		}

		newBindingID, err := CommitSwap(r.Context(), Deps{
			Pool:     deps.Pool,
			Resolver: deps.Resolver,
			Log:      deps.Log,
		}, swapIn)
		if err != nil {
			// 23P01 → 409 concurrent_swap (most common race).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
				writeJSON(w, http.StatusConflict, errorResp{
					Error:  "concurrent_swap",
					Detail: "another swap committed first; refresh and retry",
				})
				return
			}
			// pgx.ErrNoRows after the early lookup means the active binding
			// was closed by a competing swap between our lookup and
			// CloseBinding. Surface as 409 too — same operator-facing meaning.
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusConflict, errorResp{
					Error:  "concurrent_swap",
					Detail: "active binding was closed by a competing swap; refresh and retry",
				})
				return
			}
			internalError(deps.Log, w, "commit swap", err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"binding_id": newBindingID.String()})
	}
}

// ----- helpers ------------------------------------------------------------

// errorResp is the JSON envelope every Phase 2 handler emits on error.
// Mirrors site/meteringpoint/device pattern so the SPA's apiFetch logic
// doesn't need to special-case swap routes.
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
		log.Error("swap handler", "op", op, "err", err)
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

// requireAdmin loads the current session user and verifies the action via
// auth.Can. The chi router additionally wraps the route in RequireAction
// middleware (defense-in-depth) — this in-handler check guards against
// future router refactors that accidentally drop the wrapper.
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
		// Session user IDs come from the database and are always valid UUIDs;
		// hitting this path means the session got corrupted, which we treat
		// as "system event" (uuid.Nil) so the audit row still writes.
		return uuid.Nil
	}
	return u
}

// parseBigFloat parses decimal text into a 128-bit *big.Float. Rejects NaN /
// Inf via big.ParseFloat's error path (T-02-11-01).
func parseBigFloat(s string) (*big.Float, error) {
	f, _, err := big.ParseFloat(s, 10, numericPrecision, big.ToNearestEven)
	if err != nil {
		return nil, err
	}
	return f, nil
}
