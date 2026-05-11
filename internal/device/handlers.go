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
	"math"
	"math/big"
	"net/http"
	"strconv"
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
//
// Phase 3 Plan 03-07 added ActivateDevice for the ABP branch — OTAA still
// uses CreateDeviceKeys, ABP uses Activate. Both share the same atomic
// CS+PG transaction shape; the wrapper's input types live in
// internal/chirpstack/device.go.
type CSDeviceClient interface {
	CreateDevice(ctx context.Context, in chirpstack.CreateDeviceInput) error
	CreateDeviceKeys(ctx context.Context, devEUI, appKey string) error
	ActivateDevice(ctx context.Context, in chirpstack.ActivateDeviceInput) error
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
			// Phase 3 D-17: bulk decommission (per-row atomic). Max 200 ids per
			// request. Lives next to per-device decommission so RBAC + audit
			// vocabulary share the same gate.
			rt.Post("/bulk-decommission", bulkDecommissionDevices(deps))
		})
		// Phase 3 D-26 / D-27 / D-28: reveal endpoint. Admin-only via the
		// dedicated ActionDeviceRevealSecrets bundle entry (viewer absent =
		// 403 fail-closed). POST-only by design (T-3-52); chi's default 405
		// covers GET. Mounted under /api/devices/{eui}/keys so the URL is
		// keyed by the LoRaWAN canonical identifier, not Shifter UUID.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionDeviceRevealSecrets))
			rt.Post("/{eui}/keys", revealSecrets(deps))
		})
	})
}

// ----- request shapes -----------------------------------------------------

// AddDeviceRequest is the JSON body of POST /api/devices — the CHIRP-04
// atomic add-device payload.
//
// Phase 3 Plan 03-07 (D-19..D-25) extends the Phase 2 OTAA-only shape into a
// discriminated union keyed by `activation_mode`:
//
//   - "OTAA" (default — Phase 2 backwards-compatible): requires AppKey;
//     optional JoinEUI defaults to "0000000000000000". CS path:
//     CreateDevice + CreateDeviceKeys.
//   - "ABP" (new): requires DevAddr (8 hex) + NwkSKey (32 hex) + AppSKey
//     (32 hex); optional FCntUp / FCntDown default to 0. CS path:
//     CreateDevice + ActivateDevice.
//
// DEV-09 invariant: NONE of the secret fields (AppKey, NwkSKey, AppSKey,
// DevAddr) are persisted in Shifter PG. They are forwarded to CS and
// echoed back in the response body once so the dialog's D-21 success state
// can render Copy keys — never columned.
type AddDeviceRequest struct {
	DevEUI          string  `json:"dev_eui"`           // lowercase 16-hex (UI normalizes via parse-deveui)
	Name            string  `json:"name"`
	DeviceProfileID string  `json:"device_profile_id"` // UUID string
	Description     string  `json:"description,omitempty"`

	// ActivationMode — "OTAA" (default — backwards-compat) or "ABP". Phase 3
	// D-19. Validated server-side; banana → 400.
	ActivationMode string `json:"activation_mode,omitempty"`

	// --- OTAA branch fields (activation_mode == "OTAA") -------------------
	AppKey  string `json:"app_key,omitempty"`  // 32 hex; sent to CS, NEVER stored in Shifter
	JoinEUI string `json:"join_eui,omitempty"` // optional; default "0000000000000000"

	// --- ABP branch fields (activation_mode == "ABP") ---------------------
	DevAddr  string `json:"dev_addr,omitempty"`  // 8 hex
	NwkSKey  string `json:"nwk_s_key,omitempty"` // 32 hex (LoRaWAN 1.0.x — wrapper copies to NwkSEnc/SNwkSInt/FNwkSInt)
	AppSKey  string `json:"app_s_key,omitempty"` // 32 hex
	FCntUp   uint32 `json:"fcnt_up,omitempty"`   // default 0
	FCntDown uint32 `json:"fcnt_down,omitempty"` // default 0 (mapped to NFCntDown)

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

// listDevices is the Phase 3 D-12..D-18 server-side filter/sort/paginated
// list of devices. It REPLACES the Phase 2 minimal `LIMIT 100 OFFSET 0`
// implementation; the wire shape changes from a bare JSON array to a paged
// envelope `{total_count, page_count, page, per_page, rows: [...]}` so the
// frontend can render the page/total badges directly.
//
// Query params (all optional; defaults match D-14 — sort=-last_seen):
//
//	site=<uuid>     repeat to multi-select (D-12 site multi-select)
//	status=         '' | active | inactive | never_joined (D-12 activation status)
//	last_seen=      '' | 24h | 7d | 30d | all (D-12 last-seen window)
//	q=              substring on name OR dev_eui (D-12 text search)
//	sort=           name | -name | dev_eui | -dev_eui | site | -site |
//	                last_seen | -last_seen | created_at | -created_at
//	page=           1-based; defaults to 1
//	per_page=       25 | 50 | 100; defaults to 50; any other value → 400
//
// DEV-09 invariant: the per-row JSON has NO `app_key` / `nwk_s_key` /
// `app_s_key` / `nwk_key` keys — the device schema simply has no such
// columns (structural enforcement per Phase 2 D-22). Only the dedicated
// reveal endpoint (POST /api/devices/{eui}/keys) surfaces key material.
func listDevices(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, errResp := parseListDevicesParams(r)
		if errResp != nil {
			writeJSON(w, errResp.status, errResp.body)
			return
		}

		q := sqlc.New(deps.Pool)
		total, err := q.CountDevicesFiltered(r.Context(), sqlc.CountDevicesFilteredParams{
			Column1: params.siteIDs,
			Column2: params.status,
			Column3: params.lastSeenCutoff,
			Column4: params.textQ,
		})
		if err != nil {
			internalError(deps.Log, w, "count devices filtered", err)
			return
		}
		rows, err := q.ListDevicesFiltered(r.Context(), sqlc.ListDevicesFilteredParams{
			Column1: params.siteIDs,
			Column2: params.status,
			Column3: params.lastSeenCutoff,
			Column4: params.textQ,
			Column5: params.sortCol,
			Column6: params.sortDesc,
			Limit:   params.perPage,
			Offset:  (params.page - 1) * params.perPage,
		})
		if err != nil {
			internalError(deps.Log, w, "list devices filtered", err)
			return
		}

		pageCount := int32(0)
		if total > 0 {
			pageCount = int32(math.Ceil(float64(total) / float64(params.perPage)))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"total_count": total,
			"page_count":  pageCount,
			"page":        params.page,
			"per_page":    params.perPage,
			"rows":        filteredDevicesToJSON(rows),
		})
	}
}

// listDevicesParams holds the validated/normalised query params for the
// Phase 3 listDevices handler. siteIDs is the multi-select; an empty slice
// means "no site filter". textQ / status are empty strings when absent.
type listDevicesParams struct {
	siteIDs        []pgtype.UUID
	status         string
	lastSeenCutoff pgtype.Timestamptz
	textQ          string
	sortCol        string
	sortDesc       bool
	page           int32
	perPage        int32
}

type listDevicesErr struct {
	status int
	body   errorResp
}

// parseListDevicesParams parses and validates the query-string for D-12..D-18.
// Returns a non-nil error response value when validation fails (400).
func parseListDevicesParams(r *http.Request) (listDevicesParams, *listDevicesErr) {
	qv := r.URL.Query()

	// per_page restricted to {25,50,100}. D-13.
	perPage := int32(50)
	if raw := qv.Get("per_page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || (n != 25 && n != 50 && n != 100) {
			return listDevicesParams{}, &listDevicesErr{
				status: http.StatusBadRequest,
				body: errorResp{
					Error:  "invalid_per_page",
					Detail: "per_page must be one of 25, 50, 100",
				},
			}
		}
		perPage = int32(n)
	}

	// page 1-based.
	page := int32(1)
	if raw := qv.Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return listDevicesParams{}, &listDevicesErr{
				status: http.StatusBadRequest,
				body:   errorResp{Error: "invalid_page", Detail: "page must be >= 1"},
			}
		}
		page = int32(n)
	}

	// sort — default '-last_seen'.
	sortCol, sortDesc, ok := parseSortParam(qv.Get("sort"))
	if !ok {
		return listDevicesParams{}, &listDevicesErr{
			status: http.StatusBadRequest,
			body: errorResp{
				Error:  "invalid_sort",
				Detail: "sort must be one of name|dev_eui|site|last_seen|created_at (prefix '-' for desc)",
			},
		}
	}

	// status — allowed set.
	status := qv.Get("status")
	switch status {
	case "", "active", "inactive", "never_joined":
		// ok
	default:
		return listDevicesParams{}, &listDevicesErr{
			status: http.StatusBadRequest,
			body: errorResp{
				Error:  "invalid_status",
				Detail: "status must be one of active|inactive|never_joined",
			},
		}
	}

	// last_seen window → timestamp cutoff.
	lastSeenCutoff := pgtype.Timestamptz{} // Valid=false → NULL
	switch qv.Get("last_seen") {
	case "", "all":
		// no filter
	case "24h":
		lastSeenCutoff = pgtype.Timestamptz{Time: time.Now().UTC().Add(-24 * time.Hour), Valid: true}
	case "7d":
		lastSeenCutoff = pgtype.Timestamptz{Time: time.Now().UTC().Add(-7 * 24 * time.Hour), Valid: true}
	case "30d":
		lastSeenCutoff = pgtype.Timestamptz{Time: time.Now().UTC().Add(-30 * 24 * time.Hour), Valid: true}
	default:
		return listDevicesParams{}, &listDevicesErr{
			status: http.StatusBadRequest,
			body: errorResp{
				Error:  "invalid_last_seen",
				Detail: "last_seen must be one of 24h|7d|30d|all",
			},
		}
	}

	// site multi-select.
	rawSites := qv["site"]
	siteIDs := make([]pgtype.UUID, 0, len(rawSites))
	for _, s := range rawSites {
		if s == "" {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return listDevicesParams{}, &listDevicesErr{
				status: http.StatusBadRequest,
				body:   errorResp{Error: "invalid_site_id", Detail: s},
			}
		}
		siteIDs = append(siteIDs, pgUUID(id))
	}

	textQ := strings.TrimSpace(qv.Get("q"))

	return listDevicesParams{
		siteIDs:        siteIDs,
		status:         status,
		lastSeenCutoff: lastSeenCutoff,
		textQ:          textQ,
		sortCol:        sortCol,
		sortDesc:       sortDesc,
		page:           page,
		perPage:        perPage,
	}, nil
}

// parseSortParam normalises the `sort` query parameter into (column, desc, ok).
// Empty input maps to the default '-last_seen'. Unknown column names → ok=false.
func parseSortParam(raw string) (col string, desc bool, ok bool) {
	if raw == "" {
		return "last_seen", true, true
	}
	desc = false
	col = raw
	if strings.HasPrefix(raw, "-") {
		desc = true
		col = raw[1:]
	}
	switch col {
	case "name", "dev_eui", "site", "last_seen", "created_at":
		return col, desc, true
	}
	return "", false, false
}

// filteredDevicesToJSON projects the filter/sort row shape to the API JSON
// envelope used by the Devices list page. The current_site_{id,name} fields
// are only set when the device has an active binding; null otherwise.
//
// DEV-09: no key material appears anywhere in the projection — only the
// reveal endpoint surfaces secrets.
func filteredDevicesToJSON(rows []sqlc.ListDevicesFilteredRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"id":                uuidString(r.ID),
			"dev_eui":           r.DevEui,
			"name":              r.Name,
			"device_profile_id": uuidString(r.DeviceProfileID),
			"join_eui":          derefString(r.JoinEui),
			"description":       derefString(r.Description),
			"last_seen_at":      timestamptzText(r.LastSeenAt),
			"decommissioned_at": timestamptzText(r.DecommissionedAt),
			"created_at":        timestamptzText(r.CreatedAt),
			"updated_at":        timestamptzText(r.UpdatedAt),
			"current_site_id":   uuidStringOrNil(r.CurrentSiteID),
			"current_site_name": derefString(r.CurrentSiteName),
		})
	}
	return out
}

func uuidStringOrNil(u pgtype.UUID) any {
	if !u.Valid {
		return nil
	}
	return uuid.UUID(u.Bytes).String()
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
		if !isHex(in.DevEUI, 16) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_dev_eui",
				Detail: "dev_eui must be 16 lowercase hex chars"})
			return
		}

		// 1a. activation_mode (Phase 3 D-19). Empty defaults to OTAA for
		// backwards-compat with Phase 2 callers (the Phase 2 dialog never sent
		// activation_mode; we route those to OTAA without breaking them).
		in.ActivationMode = strings.ToUpper(strings.TrimSpace(in.ActivationMode))
		if in.ActivationMode == "" {
			in.ActivationMode = "OTAA"
		}
		switch in.ActivationMode {
		case "OTAA", "ABP":
			// ok
		default:
			writeJSON(w, http.StatusBadRequest, errorResp{
				Error:  "invalid_activation_mode",
				Detail: "activation_mode must be OTAA or ABP",
			})
			return
		}

		// 1b. mode-specific field validation. Each branch normalises and
		// length-checks ONLY its own fields; the opposite branch's fields are
		// left untouched (and not echoed in the response).
		switch in.ActivationMode {
		case "OTAA":
			in.AppKey = strings.ToLower(strings.TrimSpace(in.AppKey))
			in.JoinEUI = strings.ToLower(strings.TrimSpace(in.JoinEUI))
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
		case "ABP":
			in.DevAddr = strings.ToLower(strings.TrimSpace(in.DevAddr))
			in.NwkSKey = strings.ToLower(strings.TrimSpace(in.NwkSKey))
			in.AppSKey = strings.ToLower(strings.TrimSpace(in.AppSKey))
			if !isHex(in.DevAddr, 8) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_dev_addr",
					Detail: "dev_addr must be 8 lowercase hex chars"})
				return
			}
			if !isHex(in.NwkSKey, 32) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_nwk_s_key",
					Detail: "nwk_s_key must be 32 lowercase hex chars"})
				return
			}
			if !isHex(in.AppSKey, 32) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_app_s_key",
					Detail: "app_s_key must be 32 lowercase hex chars"})
				return
			}
			// JoinEUI is NOT used for ABP (CS Activate doesn't accept it). We
			// still write a default into the Shifter PG column for query
			// consistency — this is metadata, not a secret.
			if in.JoinEUI == "" {
				in.JoinEUI = "0000000000000000"
			}
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
		csInput := chirpstack.CreateDeviceInput{
			DevEUI:          in.DevEUI,
			ApplicationID:   appID,
			DeviceProfileID: csProfileIDStr,
			Name:            in.Name,
			Description:     in.Description,
			JoinEUI:         in.JoinEUI,
		}
		if in.ActivationMode == "OTAA" {
			csInput.AppKey = in.AppKey // forwarded to CS; NEVER persisted in Shifter
		}
		if err := deps.CS.CreateDevice(r.Context(), csInput); err != nil {
			deps.Log.Error("addDevice: CS CreateDevice", "err", err, "dev_eui", in.DevEUI)
			writeJSON(w, http.StatusBadGateway, errorResp{
				Error: "cs_create_failed", Detail: err.Error(),
			})
			return
		}
		csCreated := true

		// 4b. Mode-specific CS provisioning (Phase 3 D-19 branch).
		//   - OTAA: CreateDeviceKeys — uploads operator's AppKey, CS derives
		//     session keys at join time. CS itself rolls back its own device
		//     on keys failure (CreateDeviceWithKeys does this; calling pair
		//     manually here so the Postgres mutations can interleave).
		//   - ABP : ActivateDevice — uploads operator-typed session (DevAddr,
		//     NwkSKey, AppSKey) + frame counters. Same rollback shape.
		switch in.ActivationMode {
		case "OTAA":
			if err := deps.CS.CreateDeviceKeys(r.Context(), in.DevEUI, in.AppKey); err != nil {
				cleanupCS(deps, in.DevEUI)
				deps.Log.Error("addDevice: CS CreateDeviceKeys", "err", err, "dev_eui", in.DevEUI)
				writeJSON(w, http.StatusBadGateway, errorResp{
					Error: "cs_keys_failed", Detail: err.Error(),
				})
				return
			}
		case "ABP":
			if err := deps.CS.ActivateDevice(r.Context(), chirpstack.ActivateDeviceInput{
				DevEUI:   in.DevEUI,
				DevAddr:  in.DevAddr,
				NwkSKey:  in.NwkSKey,
				AppSKey:  in.AppSKey,
				FCntUp:   in.FCntUp,
				FCntDown: in.FCntDown,
			}); err != nil {
				cleanupCS(deps, in.DevEUI)
				deps.Log.Error("addDevice: CS ActivateDevice", "err", err, "dev_eui", in.DevEUI)
				writeJSON(w, http.StatusBadGateway, errorResp{
					Error: "cs_activate_failed", Detail: err.Error(),
				})
				return
			}
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
		//
		// DEV-09 / T-3-70: the audit `after` payload records the activation
		// mode but NEVER any key material. The mode itself is reconstructable
		// metadata (already on the CS side); the keys are only ever surfaced
		// via the dedicated reveal endpoint.
		after := map[string]any{
			"dev_eui":           in.DevEUI,
			"name":              in.Name,
			"device_profile_id": profileID.String(),
			"join_eui":          in.JoinEUI,
			"description":       in.Description,
			"activation_mode":   in.ActivationMode,
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

		// Success — never delete CS at this point. Echo the operator-typed
		// keys back to the client for the dialog's D-21 success state. Shifter
		// did NOT persist them; the client renders Copy buttons once and then
		// the dialog tears down (gcTime:0). To re-reveal later, the operator
		// calls POST /api/devices/:eui/keys (separate authz boundary).
		resp := deviceToJSON(dev)
		resp["activation_mode"] = in.ActivationMode
		switch in.ActivationMode {
		case "OTAA":
			resp["app_key"] = in.AppKey
			resp["nwk_key"] = in.AppKey // LoRaWAN 1.0.x convention: NwkKey = AppKey
			resp["join_eui"] = in.JoinEUI
		case "ABP":
			resp["dev_addr"] = in.DevAddr
			resp["nwk_s_key"] = in.NwkSKey
			resp["app_s_key"] = in.AppSKey
			resp["f_cnt_up"] = in.FCntUp
			resp["f_cnt_down"] = in.FCntDown
		}
		writeJSON(w, http.StatusCreated, resp)
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

// BulkDecommissionRequest is the JSON body of POST /api/devices/bulk-decommission.
type BulkDecommissionRequest struct {
	DeviceIDs []string `json:"device_ids"`
	Reason    string   `json:"reason"`
}

// bulkDecommissionDevices handles POST /api/devices/bulk-decommission (D-17).
//
// Per-row Serializable tx: failure of one device does NOT roll back rows
// already committed. Each device's CS+PG side effects (CS DeleteDevice +
// q.DecommissionDevice + close active binding if any + audit row) live in
// their own short-lived transaction so a partial-success summary is honest.
//
// CS DeleteDevice runs on a fresh context (Pitfall 02-05) and CS-not-found is
// folded into "ok" — the bulk path is idempotent against an already-cleaned
// CS side. PG-level failures (not_found, already_decommissioned, conflict)
// produce a per-row "failed" outcome but never abort the loop.
//
// Cap: max 200 device_ids per request to bound work and DoS surface
// (T-3-56). Over-cap → 400.
//
// Audit: every successful row gets an audit_log entry with
// action='decommission' and notes='bulk decommission: <reason>' so the
// per-row trail is identical in shape to single-device decommission, plus
// `request_id` from chi middleware groups every row of one bulk call.
func bulkDecommissionDevices(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionDeviceDecommission)
		if !ok {
			return
		}
		var body BulkDecommissionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		if len(body.DeviceIDs) == 0 {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "no_device_ids"})
			return
		}
		if len(body.DeviceIDs) > 200 {
			writeJSON(w, http.StatusBadRequest, errorResp{
				Error:  "too_many_devices",
				Detail: "max 200 devices per request",
			})
			return
		}

		outcomes := make([]map[string]any, 0, len(body.DeviceIDs))
		succeeded, failed := 0, 0
		userUUID := mustParseUUID(user.ID)
		requestID := middleware.GetReqID(r.Context())
		notes := "bulk decommission"
		if strings.TrimSpace(body.Reason) != "" {
			notes = "bulk decommission: " + strings.TrimSpace(body.Reason)
		}

		for _, rawID := range body.DeviceIDs {
			id, err := uuid.Parse(rawID)
			if err != nil {
				outcomes = append(outcomes, map[string]any{
					"id": rawID, "status": "failed", "reason": "invalid_id",
				})
				failed++
				continue
			}
			outcome, errRow := bulkDecommissionOne(r.Context(), deps, id, userUUID, requestID, notes)
			outcomes = append(outcomes, outcome)
			if errRow != nil {
				failed++
			} else {
				succeeded++
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"succeeded": succeeded,
			"failed":    failed,
			"outcomes":  outcomes,
		})
	}
}

// bulkDecommissionOne runs a single device's Per-row Serializable tx for the
// bulk endpoint. Returns the per-row outcome map AND any error encountered
// (so the caller can tally succeeded/failed). The map is the same shape used
// in the bulk response.
func bulkDecommissionOne(ctx context.Context, deps Deps, id uuid.UUID, userUUID uuid.UUID, requestID, notes string) (map[string]any, error) {
	tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return map[string]any{"id": id.String(), "status": "failed", "reason": "tx_begin"}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlc.New(tx)

	dev, err := q.GetDevice(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]any{"id": id.String(), "status": "failed", "reason": "not_found"}, err
	}
	if err != nil {
		return map[string]any{"id": id.String(), "status": "failed", "reason": "lookup"}, err
	}
	if dev.DecommissionedAt.Valid {
		return map[string]any{"id": id.String(), "status": "failed", "reason": "already_decommissioned"}, errors.New("already_decommissioned")
	}

	// Close active binding if any — mirrors single decommissionDevice flow.
	var bindingID pgtype.UUID
	var bindingFound bool
	err = tx.QueryRow(ctx,
		`SELECT id FROM binding WHERE device_id = $1 AND valid_to IS NULL LIMIT 1`,
		dev.ID,
	).Scan(&bindingID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// no active binding — fine.
	case err != nil:
		return map[string]any{"id": id.String(), "status": "failed", "reason": "binding_lookup"}, err
	default:
		bindingFound = true
	}
	if bindingFound {
		if _, err := q.CloseBinding(ctx, sqlc.CloseBindingParams{
			ID:      bindingID,
			ValidTo: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}); err != nil {
			return map[string]any{"id": id.String(), "status": "failed", "reason": "binding_close"}, err
		}
	}

	// CS DeleteDevice best-effort with fresh ctx (Pitfall 02-05). CS-not-found
	// is folded into success so re-running a bulk-decommission against an
	// already-cleaned CS side is idempotent.
	if deps.CS != nil {
		csCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if cerr := deps.CS.DeleteDevice(csCtx, dev.DevEui); cerr != nil &&
			!errors.Is(cerr, chirpstack.ErrNotFound) && deps.Log != nil {
			// Log but do NOT roll back — operator's intent is "retire this
			// device in Shifter"; CS may already be gone or unreachable.
			deps.Log.Warn("bulk decommission: CS DeleteDevice", "err", cerr, "dev_eui", dev.DevEui)
		}
		cancel()
	}

	decommissioned, err := q.DecommissionDevice(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]any{"id": id.String(), "status": "failed", "reason": "already_decommissioned"}, err
	}
	if err != nil {
		return map[string]any{"id": id.String(), "status": "failed", "reason": "decommission"}, err
	}

	after := map[string]any{
		"decommissioned_at": timestamptzText(decommissioned.DecommissionedAt),
		"closed_binding":    bindingFound,
	}
	if bindingFound {
		after["binding_id"] = uuid.UUID(bindingID.Bytes).String()
	}
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     userUUID,
		Action:     audit.ActionDecommission,
		EntityType: audit.EntityTypeDevice,
		EntityID:   uuid.UUID(decommissioned.ID.Bytes),
		Before:     map[string]any{"decommissioned_at": nil},
		After:      after,
		Notes:      notes,
		RequestID:  requestID,
	}); err != nil {
		return map[string]any{"id": id.String(), "status": "failed", "reason": "audit"}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return map[string]any{"id": id.String(), "status": "failed", "reason": "commit"}, err
	}
	return map[string]any{"id": id.String(), "status": "decommissioned"}, nil
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
