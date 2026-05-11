// reveal.go — POST /api/devices/{eui}/keys (Phase 3 D-22 / D-26 / D-27 / D-28).
//
// Surface contract:
//
//	POST /api/devices/{eui}/keys           — admin-only via RequireAction(ActionDeviceRevealSecrets)
//	  200 OTAA: {activation_mode: "OTAA", dev_eui, join_eui, app_key, nwk_key}
//	  200 ABP : {activation_mode: "ABP",  dev_eui, dev_addr, nwk_s_key, app_s_key,
//	            f_cnt_up, n_f_cnt_down, a_f_cnt_down}
//	  400 invalid_dev_eui     — :eui is not 16-hex lowercase (caught before any DB/CS call)
//	  401 unauthorized        — no session (RequireAction)
//	  403 forbidden           — viewer (RequireAction; T-3-50)
//	  404 not_found           — device row absent in Shifter PG
//	  409 no_credentials_in_cs — both CS GetKeys AND GetActivation returned NotFound
//	  502 cs_get_keys_failed  / cs_get_activation_failed — CS gRPC error (not NotFound)
//
// DEV-09 contract:
//   - Shifter has no key columns on `device` (structural per Phase 2 D-22).
//   - This is the ONLY endpoint that surfaces secret material to the wire.
//   - The audit row records ONLY {activation_mode, dev_eui} — no key bytes
//     (T-3-51, defended by TestRevealSecrets_AuditNoSecretMaterial which
//     reads the audit_log JSONB and asserts no key hex appears).
//
// T-3-53 mitigation: Cache-Control: no-store + Pragma: no-cache headers are
// set BEFORE any 200 or post-lookup error response so revealed material is
// never cached in browser, intermediary proxy, or local disk.
package device

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// CSDeviceRevealer is the narrow ChirpStack contract the reveal handler
// requires. *chirpstack.Client structurally satisfies it; tests substitute a
// fake without standing up a real CS instance. Defined separately from
// CSDeviceClient (handlers.go) because reveal does NOT need
// CreateDevice/CreateDeviceKeys/DeleteDevice — keeping the interfaces narrow
// makes the test surface smaller and prevents accidental use of write
// operations from inside the reveal flow.
type CSDeviceRevealer interface {
	GetDeviceKeys(ctx context.Context, devEUI string) (*chirpstack.DeviceKeys, error)
	GetDeviceActivation(ctx context.Context, devEUI string) (*chirpstack.DeviceActivation, error)
}

// revealSecrets handles POST /api/devices/{eui}/keys.
//
// Two-stage strategy (RESEARCH §device.reveal_secrets):
//
//  1. GetDeviceKeys → success: OTAA response.
//  2. GetDeviceKeys returns ErrNotFound (no OTAA keys provisioned for this
//     device) → fall through to GetDeviceActivation → success: ABP response.
//  3. Both return ErrNotFound → 409 no_credentials_in_cs.
//
// Any non-NotFound error from CS surfaces as 502 with a distinct error code
// so admins can tell apart "CS gRPC down" vs "keys never provisioned".
//
// Uses deps.CS (CSDeviceClient) for cleanup and the EXPANDED interface for
// reveal — *chirpstack.Client implements both. When deps.CS is nil (router
// unit tests without CS wiring) the handler 503s instead of panicking.
func revealSecrets(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// RequireAction wraps this handler at route mount time (admin-only).
		// We still pull the session user here so the audit row carries actor.
		user, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		if !auth.Can(&user, auth.ActionDeviceRevealSecrets, nil) {
			writeJSON(w, http.StatusForbidden, errorResp{Error: "forbidden"})
			return
		}

		// 1. Validate the :eui parameter BEFORE any DB / CS call.
		devEUI := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "eui")))
		if !isHex(devEUI, 16) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_dev_eui"})
			return
		}

		// 2. Resolve the device in Shifter PG (404 if absent — defense in
		// depth so reveal cannot enumerate CS-only DevEUIs that have no
		// Shifter row).
		q := sqlc.New(deps.Pool)
		dev, err := q.GetDeviceByDevEUI(r.Context(), devEUI)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "lookup device by dev_eui", err)
			return
		}

		// 3. Cache prevention headers BEFORE any response body that may
		// carry key material (T-3-53). Applied to the 200 success AND to
		// any 502/409 that flows past this point — over-strict here is
		// strictly safer than under-strict.
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")

		revealer, revealerOK := deps.CS.(CSDeviceRevealer)
		if !revealerOK || revealer == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResp{
				Error:  "cs_unavailable",
				Detail: "ChirpStack reveal client not wired",
			})
			return
		}

		// 4. OTAA path — GetDeviceKeys.
		keys, errKeys := revealer.GetDeviceKeys(r.Context(), devEUI)
		if errKeys == nil {
			writeAuditReveal(r.Context(), deps, user, dev, "OTAA")
			writeJSON(w, http.StatusOK, map[string]any{
				"activation_mode": "OTAA",
				"dev_eui":         devEUI,
				"join_eui":        derefString(dev.JoinEui),
				"app_key":         keys.AppKey,
				"nwk_key":         keys.NwkKey,
			})
			return
		}
		if !errors.Is(errKeys, chirpstack.ErrNotFound) {
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error:  "cs_get_keys_failed",
				Detail: errKeys.Error(),
			})
			return
		}

		// 5. ABP path — GetDeviceActivation.
		act, errAct := revealer.GetDeviceActivation(r.Context(), devEUI)
		if errAct == nil {
			writeAuditReveal(r.Context(), deps, user, dev, "ABP")
			writeJSON(w, http.StatusOK, map[string]any{
				"activation_mode": "ABP",
				"dev_eui":         devEUI,
				"dev_addr":        act.DevAddr,
				"nwk_s_key":       act.NwkSKey,
				"app_s_key":       act.AppSKey,
				"f_cnt_up":        act.FCntUp,
				"n_f_cnt_down":    act.NFCntDown,
				"a_f_cnt_down":    act.AFCntDown,
			})
			return
		}
		if !errors.Is(errAct, chirpstack.ErrNotFound) {
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error:  "cs_get_activation_failed",
				Detail: errAct.Error(),
			})
			return
		}

		// 6. Neither OTAA keys nor ABP activation in CS.
		writeJSON(w, http.StatusConflict, errorResp{Error: "no_credentials_in_cs"})
	}
}

// writeAuditReveal writes the audit row for a successful reveal call.
//
// DEV-09 / D-28 contract: the `after` JSONB carries ONLY
// {activation_mode, dev_eui} — no key material. The test
// TestRevealSecrets_AuditNoSecretMaterial reads this row and asserts the
// JSON does NOT contain any of the AppKey / NwkKey / AppSKey / NwkSKey hex
// strings (negative grep + JSON substring check).
//
// The audit insert runs in its own short-lived transaction (NOT the request
// transaction — there isn't one for a read-only reveal). On insert failure
// the reveal RESPONSE still succeeds (we do not want to deny the operator a
// key reveal because audit_log had a transient hiccup); the failure is
// warning-logged for operations review.
func writeAuditReveal(ctx context.Context, deps Deps, user auth.User, dev sqlc.Device, mode string) {
	uid := mustParseUUID(user.ID)
	tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		if deps.Log != nil {
			deps.Log.Warn("reveal audit begin tx", "err", err, "dev_eui", dev.DevEui)
		}
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     uid,
		Action:     audit.ActionRevealSecrets,
		EntityType: audit.EntityTypeDevice,
		EntityID:   uuidFromPgUUID(dev.ID),
		Before:     nil,
		After: map[string]any{
			"activation_mode": mode,
			"dev_eui":         dev.DevEui,
		},
		RequestID: middleware.GetReqID(ctx),
	}); err != nil {
		if deps.Log != nil {
			deps.Log.Warn("reveal audit write", "err", err, "dev_eui", dev.DevEui)
		}
		return
	}
	if err := tx.Commit(ctx); err != nil {
		if deps.Log != nil {
			deps.Log.Warn("reveal audit commit", "err", err, "dev_eui", dev.DevEui)
		}
	}
}

// uuidFromPgUUID adapts pgtype.UUID → uuid.UUID for audit.Entry shape.
// Returns uuid.Nil when the pg UUID is invalid (defense — should not happen
// since dev.ID is a primary key, but better than panicking on a bug).
func uuidFromPgUUID(u pgtype.UUID) uuid.UUID {
	if !u.Valid {
		return uuid.Nil
	}
	return uuid.UUID(u.Bytes)
}
