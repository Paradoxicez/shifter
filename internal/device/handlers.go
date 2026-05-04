// Package device — HTTP handlers for the Add Device atomic flow (CHIRP-04)
// + decommission (D-15) + list/search/preflight/parse-deveui helpers.
//
// The atomic Add Device flow is the headline contract: a single user action
// in the UI → ONE pgx.Serializable transaction that runs (a) ChirpStack
// preflight (b) ChirpStack EnsureTenantAndApplication if needed (c)
// ChirpStack CreateDevice + CreateDeviceKeys (d) Shifter device row insert
// (e) optional initial binding (f) audit row, and either commits both stores
// or rolls both back. The CS-side rollback is best-effort (uses a fresh
// context per Pitfall 02-05) — a Postgres failure after CS Create triggers
// a CS DeleteDevice attempt so the operator doesn't see "device created in
// CS but not in Shifter."
//
// AppKey policy (DEV-09 / T-02-10-02): the request body accepts AppKey, the
// handler forwards it to ChirpStack via CreateDeviceKeys, and Shifter NEVER
// writes it to a Shifter table. The schema has no app_key column on device
// — DEV-09 is structural, not policy-based.
package device

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// CSDeviceClient is the narrow ChirpStack contract Add Device + Decommission
// depend on. *chirpstack.Client structurally satisfies this interface; tests
// substitute a fakeCSClient without standing up a real CS instance.
type CSDeviceClient interface {
	CreateDevice(ctx context.Context, in chirpstack.CreateDeviceInput) error
	CreateDeviceKeys(ctx context.Context, devEUI, appKey string) error
	DeleteDevice(ctx context.Context, devEUI string) error
}

// CSPreflighter probes ChirpStack reachability (gRPC + MQTT). The dialog uses
// this AFTER the operator picks the profile but BEFORE submitting Add Device,
// so a flaky link surfaces as "ChirpStack unreachable" not "device half-
// created in CS." *chirpstack.Client wraps both gRPC pings; MQTT pings live
// in chirpstack.PingMQTT (package-level fn).
type CSPreflighter interface {
	PingDevices(ctx context.Context, applicationID string) error
}

// CSBootstrapper is the EnsureTenantAndApplication contract. Defined here as
// a 1-method interface so AddDevice tests can stub the bootstrap with a
// pre-canned (tenantID, appID) without wiring a fake ConnectionStore + fake
// Client just to exercise the happy path.
type CSBootstrapper interface {
	EnsureTenantAndApplication(ctx context.Context) (tenantID, appID string, err error)
}

// Deps bundles the shared infra a Device handler call needs.
type Deps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger

	// CS — the gRPC wrapper for device CRUD.
	CS CSDeviceClient

	// Bootstrap — D-28 idempotent tenant + application ensurer. Called once
	// per AddDevice request to lazily bootstrap on first device add. Idempotent
	// after that — store-cached UUIDs short-circuit the calls.
	Bootstrap CSBootstrapper

	// PingGRPC + PingMQTT — preflight probes for the dialog's "test connection
	// before submit" button. PingGRPC takes the resolved CS application ID
	// (so the probe matches what AddDevice will actually use); PingMQTT takes
	// the raw broker URL + creds. Either may be nil at construction — the
	// preflight handler returns 503 in that case.
	PingGRPC func(ctx context.Context, applicationID string) error
	PingMQTT func(ctx context.Context) error
}

// RegisterRoutes mounts every Device endpoint under /api/devices on the
// given chi router.
//
// Route table:
//
//	GET   /api/devices                     device.read         — admin + viewer
//	GET   /api/devices/search              device.read         — admin + viewer
//	GET   /api/devices/by-site/{siteID}    device.read         — admin + viewer
//	GET   /api/devices/{id}                device.read         — admin + viewer
//	POST  /api/devices/preflight           device.add          — admin only (probe is mutating-flow-adjacent)
//	POST  /api/devices/parse-deveui        device.read         — admin + viewer (read-only sticker parse)
//	POST  /api/devices                     device.add          — admin only (CHIRP-04 atomic)
//	POST  /api/devices/{id}/decommission   device.decommission — admin only (D-15)
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Route("/api/devices", func(r chi.Router) {
		// Read group.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionDeviceRead))
			rt.Get("/", listDevices(deps))
			rt.Get("/search", searchDevices(deps))
			rt.Get("/by-site/{siteID}", listBySite(deps))
			rt.Get("/{id}", getDevice(deps))
			// parse-deveui is read-only string transformation; viewer can use
			// it from a hypothetical future "test the sticker" tool.
			rt.Post("/parse-deveui", parseDevEUIHandler(deps))
		})
		// Mutate group — admin only (preflight is admin-only because it's part
		// of the Add Device flow; viewer doesn't need to test connection from
		// the device list).
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionDeviceAdd))
			rt.Post("/preflight", preflightCS(deps))
			rt.Post("/", addDevice(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionDeviceDecommission))
			rt.Post("/{id}/decommission", decommissionDevice(deps))
		})
	})
}

// ----- request shapes -----------------------------------------------------

// AddDeviceRequest is the JSON body of POST /api/devices — the CHIRP-04
// atomic add-device payload.
type AddDeviceRequest struct {
	DevEUI          string  `json:"dev_eui"`           // lowercase 16-hex (UI normalizes via parse-deveui)
	Name            string  `json:"name"`
	DeviceProfileID string  `json:"device_profile_id"` // UUID string
	AppKey          string  `json:"app_key"`           // 32 hex; sent to CS, NEVER stored in Shifter
	JoinEUI         string  `json:"join_eui,omitempty"` // optional; default "0000000000000000"
	Description     string  `json:"description,omitempty"`

	// MeteringPointID + InitialReading — when set, opens an initial binding
	// in the same atomic txn (D-10 step 3). MeteringPointID empty = device
	// created without a binding; the operator can wire it up later.
	MeteringPointID string `json:"metering_point_id,omitempty"`
	InitialReading  string `json:"initial_reading,omitempty"` // decimal text; defaults to "0"
}

// ParseDevEUIRequest is the JSON body of POST /api/devices/parse-deveui.
type ParseDevEUIRequest struct {
	Raw string `json:"raw"`
}

// ----- handlers -----------------------------------------------------------

func listDevices(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := sqlc.New(deps.Pool)
		// Phase 2 minimal pagination: hardcoded LIMIT/OFFSET via query params.
		rows, err := q.ListActiveDevices(r.Context(), sqlc.ListActiveDevicesParams{
			Limit: 100, Offset: 0,
		})
		if err != nil {
			internalError(deps.Log, w, "list devices", err)
			return
		}
		writeJSON(w, http.StatusOK, devicesToJSON(rows))
	}
}

func searchDevices(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qstr := r.URL.Query().Get("q")
		var qPtr *string
		if qstr != "" {
			qPtr = &qstr
		}
		q := sqlc.New(deps.Pool)
		rows, err := q.SearchDevices(r.Context(), sqlc.SearchDevicesParams{
			Column1: qPtr, Limit: 100, Offset: 0,
		})
		if err != nil {
			internalError(deps.Log, w, "search devices", err)
			return
		}
		writeJSON(w, http.StatusOK, devicesToJSON(rows))
	}
}

func listBySite(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "siteID")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		rows, err := q.ListDevicesBySite(r.Context(), pgUUID(id))
		if err != nil {
			internalError(deps.Log, w, "list devices by site", err)
			return
		}
		writeJSON(w, http.StatusOK, devicesToJSON(rows))
	}
}

func getDevice(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		row, err := q.GetDevice(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "get device", err)
			return
		}
		writeJSON(w, http.StatusOK, deviceToJSON(row))
	}
}

// preflightCS pings ChirpStack gRPC + MQTT before the operator submits Add
// Device. Returns {grpc: "ok"|"err", mqtt: "ok"|"err", ...}. Does NOT mutate.
func preflightCS(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		out := map[string]any{
			"grpc": "ok",
			"mqtt": "ok",
		}
		anyErr := false

		if deps.PingGRPC != nil {
			if err := deps.PingGRPC(ctx, ""); err != nil {
				out["grpc"] = "err"
				out["grpc_detail"] = err.Error()
				anyErr = true
			}
		} else {
			out["grpc"] = "skipped"
		}
		if deps.PingMQTT != nil {
			if err := deps.PingMQTT(ctx); err != nil {
				out["mqtt"] = "err"
				out["mqtt_detail"] = err.Error()
				anyErr = true
			}
		} else {
			out["mqtt"] = "skipped"
		}

		code := http.StatusOK
		if anyErr {
			code = http.StatusBadGateway
		}
		writeJSON(w, code, out)
	}
}

// parseDevEUIHandler returns the D-11 sticker-parse preview. Pure string
// transformation; no DB or CS calls.
func parseDevEUIHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in ParseDevEUIRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		preview, err := ParseDevEUI(in.Raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_dev_eui", Detail: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"msb":         preview.MSB,
			"lsb":         preview.LSB,
			"msb_vendor":  preview.MSBVendor,
			"lsb_vendor":  preview.LSBVendor,
		})
	}
}

// addDevice is the CHIRP-04 atomic add-device handler.
//
// Flow:
//
//  1. RBAC + decode + server-side validate (DevEUI, AppKey, JoinEUI).
//  2. EnsureTenantAndApplication (idempotent — store-cached UUIDs short-circuit).
//  3. Load profile, verify cs_profile_id is non-NULL (Plan 02-08 seed sync done).
//  4. BEGIN Serializable txn:
//     4a. CS CreateDevice (sets csCreated=true).
//     4b. CS CreateDeviceKeys (CS rolls back its own device on keys failure).
//     4c. INSERT device row.
//     4d. If MP picked, OPEN initial binding.
//     4e. audit.WriteEntry inside same tx.
//  5. COMMIT.
//
// On any post-CS failure: rollback the txn AND attempt CS DeleteDevice with
// a FRESH context (Pitfall 02-05 — caller's ctx may already be cancelled).
//
// AppKey is forwarded to CS via CreateDeviceKeys and NEVER persisted in
// Shifter (DEV-09 / T-02-10-02 — structural via schema, defensive via this
// handler).
func addDevice(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionDeviceAdd)
		if !ok {
			return
		}
		var in AddDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}

		// 1. Server-side validation.
		in.DevEUI = strings.ToLower(strings.TrimSpace(in.DevEUI))
		in.AppKey = strings.ToLower(strings.TrimSpace(in.AppKey))
		in.JoinEUI = strings.ToLower(strings.TrimSpace(in.JoinEUI))
		if !isHex(in.DevEUI, 16) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_dev_eui",
				Detail: "dev_eui must be 16 lowercase hex chars"})
			return
		}
		if !isHex(in.AppKey, 32) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_app_key",
				Detail: "app_key must be 32 lowercase hex chars"})
			return
		}
		if in.JoinEUI == "" {
			in.JoinEUI = "0000000000000000"
		}
		if !isHex(in.JoinEUI, 16) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_join_eui",
				Detail: "join_eui must be 16 lowercase hex chars"})
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation",
				Detail: "name is required"})
			return
		}
		profileID, err := uuid.Parse(in.DeviceProfileID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_device_profile_id"})
			return
		}
		var mpUUID uuid.UUID
		if in.MeteringPointID != "" {
			mpUUID, err = uuid.Parse(in.MeteringPointID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_metering_point_id"})
				return
			}
		}

		// 2. Bootstrap CS tenant + application (idempotent).
		if deps.Bootstrap == nil {
			internalError(deps.Log, w, "bootstrap", errors.New("Bootstrap is nil"))
			return
		}
		_, appID, err := deps.Bootstrap.EnsureTenantAndApplication(r.Context())
		if err != nil {
			deps.Log.Error("addDevice: bootstrap CS", "err", err)
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error: "chirpstack_unreachable", Detail: err.Error(),
			})
			return
		}

		// 3. Load device profile, verify cs_profile_id.
		q := sqlc.New(deps.Pool)
		prof, err := q.GetDeviceProfile(r.Context(), pgUUID(profileID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "device_profile_not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load device profile", err)
			return
		}
		if !prof.CsProfileID.Valid {
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error:  "profile_not_synced",
				Detail: "device_profile has no cs_profile_id — open Settings → Sync profiles before adding devices",
			})
			return
		}
		csProfileIDStr := uuid.UUID(prof.CsProfileID.Bytes).String()

		// 4. Open the atomic Serializable txn.
		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		// 4a. CS CreateDevice.
		if err := deps.CS.CreateDevice(r.Context(), chirpstack.CreateDeviceInput{
			DevEUI:          in.DevEUI,
			ApplicationID:   appID,
			DeviceProfileID: csProfileIDStr,
			Name:            in.Name,
			Description:     in.Description,
			AppKey:          in.AppKey, // forwarded to CS; NEVER persisted in Shifter
			JoinEUI:         in.JoinEUI,
		}); err != nil {
			deps.Log.Error("addDevice: CS CreateDevice", "err", err, "dev_eui", in.DevEUI)
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error: "cs_create_failed", Detail: err.Error(),
			})
			return
		}
		csCreated := true

		// 4b. CS CreateDeviceKeys. CS itself rolls back its own device on keys
		// failure (CreateDeviceWithKeys does this; calling pair manually here
		// so the Postgres mutations can interleave).
		if err := deps.CS.CreateDeviceKeys(r.Context(), in.DevEUI, in.AppKey); err != nil {
			// CS keys failed — clean up the orphan CS device with a fresh ctx.
			cleanupCS(deps, in.DevEUI)
			deps.Log.Error("addDevice: CS CreateDeviceKeys", "err", err, "dev_eui", in.DevEUI)
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error: "cs_keys_failed", Detail: err.Error(),
			})
			return
		}

		// 4c. INSERT Shifter device row.
		txQ := sqlc.New(tx)
		var joinEUIPtr *string
		joinEUIVal := in.JoinEUI
		joinEUIPtr = &joinEUIVal
		var descPtr *string
		if in.Description != "" {
			descVal := in.Description
			descPtr = &descVal
		}
		dev, err := txQ.CreateDevice(r.Context(), sqlc.CreateDeviceParams{
			DevEui:          in.DevEUI,
			Name:            strings.TrimSpace(in.Name),
			DeviceProfileID: pgUUID(profileID),
			JoinEui:         joinEUIPtr,
			Description:     descPtr,
		})
		if err != nil {
			if csCreated {
				cleanupCS(deps, in.DevEUI)
			}
			if isUniqueViolation(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "duplicate_dev_eui"})
				return
			}
			if isConstraintViolation(err) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "constraint", Detail: err.Error()})
				return
			}
			internalError(deps.Log, w, "insert device", err)
			return
		}

		// 4d. Open initial binding if MP picked.
		var bindingIDStr string
		if mpUUID != uuid.Nil {
			offsetText := in.InitialReading
			if offsetText == "" {
				offsetText = "0"
			}
			// Validate text parses to a number (lossless via big.Float).
			if _, _, err := big.ParseFloat(offsetText, 10, 128, big.ToNearestEven); err != nil {
				if csCreated {
					cleanupCS(deps, in.DevEUI)
				}
				writeJSON(w, http.StatusBadRequest, errorResp{
					Error: "invalid_initial_reading", Detail: err.Error(),
				})
				return
			}
			var offsetNum pgtype.Numeric
			if err := offsetNum.Scan(offsetText); err != nil {
				if csCreated {
					cleanupCS(deps, in.DevEUI)
				}
				internalError(deps.Log, w, "encode initial offset", err)
				return
			}
			b, err := txQ.OpenBinding(r.Context(), sqlc.OpenBindingParams{
				MeteringPointID: pgUUID(mpUUID),
				DeviceID:        dev.ID,
				ValidFrom:       pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				ReadingOffset:   offsetNum,
			})
			if err != nil {
				if csCreated {
					cleanupCS(deps, in.DevEUI)
				}
				if isConstraintViolation(err) {
					writeJSON(w, http.StatusBadRequest, errorResp{Error: "binding_constraint", Detail: err.Error()})
					return
				}
				internalError(deps.Log, w, "open initial binding", err)
				return
			}
			bindingIDStr = uuid.UUID(b.ID.Bytes).String()
		}

		// 4e. Audit row INSIDE same tx.
		after := map[string]any{
			"dev_eui":           in.DevEUI,
			"name":              in.Name,
			"device_profile_id": profileID.String(),
			"join_eui":          in.JoinEUI,
			"description":       in.Description,
		}
		if mpUUID != uuid.Nil {
			after["initial_binding"] = map[string]any{
				"metering_point_id": mpUUID.String(),
				"binding_id":        bindingIDStr,
				"reading_offset":    in.InitialReading,
			}
		}
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionCreate,
			EntityType: audit.EntityTypeDevice,
			EntityID:   uuid.UUID(dev.ID.Bytes),
			Before:     nil,
			After:      after,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			if csCreated {
				cleanupCS(deps, in.DevEUI)
			}
			internalError(deps.Log, w, "audit add device", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			if csCreated {
				cleanupCS(deps, in.DevEUI)
			}
			internalError(deps.Log, w, "commit add device", err)
			return
		}

		// Success — never delete CS at this point.
		writeJSON(w, http.StatusCreated, deviceToJSON(dev))
	}
}

// decommissionDevice closes the active binding (if any) AND marks the device
// decommissioned, in one Serializable txn with an audit row. D-15 contract.
func decommissionDevice(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionDeviceDecommission)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		q := sqlc.New(tx)

		// Look up active binding (if any) so we can close it in the same tx.
		// Use the device-id keyed query indirectly: GetActiveBindingByDevEUI
		// requires dev_eui which we don't have yet — load the device first.
		dev, err := q.GetDevice(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "get device", err)
			return
		}
		if dev.DecommissionedAt.Valid {
			writeJSON(w, http.StatusConflict, errorResp{Error: "already_decommissioned"})
			return
		}

		// Find active binding for this device (if any) and close it.
		var bindingID pgtype.UUID
		var bindingFound bool
		err = tx.QueryRow(r.Context(),
			`SELECT id FROM binding WHERE device_id = $1 AND valid_to IS NULL LIMIT 1`,
			dev.ID,
		).Scan(&bindingID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// no active binding — fine.
		case err != nil:
			internalError(deps.Log, w, "find active binding", err)
			return
		default:
			bindingFound = true
		}
		if bindingFound {
			if _, err := q.CloseBinding(r.Context(), sqlc.CloseBindingParams{
				ID:      bindingID,
				ValidTo: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			}); err != nil {
				internalError(deps.Log, w, "close binding", err)
				return
			}
		}

		decommissioned, err := q.DecommissionDevice(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusConflict, errorResp{Error: "already_decommissioned"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "decommission device", err)
			return
		}

		after := map[string]any{
			"decommissioned_at":  timestamptzText(decommissioned.DecommissionedAt),
			"closed_binding":     bindingFound,
		}
		if bindingFound {
			after["binding_id"] = uuid.UUID(bindingID.Bytes).String()
		}
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionDecommission,
			EntityType: audit.EntityTypeDevice,
			EntityID:   uuid.UUID(decommissioned.ID.Bytes),
			Before:     map[string]any{"decommissioned_at": nil},
			After:      after,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit decommission", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit decommission", err)
			return
		}
		writeJSON(w, http.StatusOK, deviceToJSON(decommissioned))
	}
}

// ----- helpers ------------------------------------------------------------

// cleanupCS attempts a best-effort CS DeleteDevice with a FRESH context so a
// caller-cancelled ctx can't pre-empt the cleanup (Pitfall 02-05). The error
// is intentionally swallowed — the caller already has a primary error to
// surface; a noisy double-error obscures the real failure.
func cleanupCS(deps Deps, devEUI string) {
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := deps.CS.DeleteDevice(cleanup, devEUI); err != nil && deps.Log != nil {
		deps.Log.Warn("addDevice: CS cleanup failed", "err", err, "dev_eui", devEUI)
	}
}

// isHex reports whether s is exactly n lowercase hex characters.
func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	if s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

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
		log.Error("device handler", "op", op, "err", err)
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

func isConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23514") ||
		strings.Contains(msg, "23503") ||
		strings.Contains(msg, "23P01") ||
		strings.Contains(msg, "violates check constraint") ||
		strings.Contains(msg, "violates foreign key constraint") ||
		strings.Contains(msg, "violates exclusion constraint")
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") || strings.Contains(msg, "violates unique constraint")
}

// ----- JSON serialization -------------------------------------------------

func deviceToJSON(d sqlc.Device) map[string]any {
	out := map[string]any{
		"id":                 uuidString(d.ID),
		"dev_eui":            d.DevEui,
		"name":               d.Name,
		"device_profile_id":  uuidString(d.DeviceProfileID),
		"join_eui":           derefString(d.JoinEui),
		"description":        derefString(d.Description),
		"last_seen_at":       timestamptzText(d.LastSeenAt),
		"decommissioned_at":  timestamptzText(d.DecommissionedAt),
		"created_at":         timestamptzText(d.CreatedAt),
		"updated_at":         timestamptzText(d.UpdatedAt),
	}
	// cs_device_uuid is intentionally omitted from the wire — operators
	// don't address devices by CS UUID, only DevEUI.
	_ = d.CsDeviceUuid
	return out
}

func devicesToJSON(rows []sqlc.Device) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, deviceToJSON(r))
	}
	return out
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

// silence unused-import lint when fmt is only used by tests.
var _ = fmt.Sprintf
