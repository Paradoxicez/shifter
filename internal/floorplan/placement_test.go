package floorplan_test

// Plan 05-07 Task 1: placement CRUD integration tests.
//
// These tests use the same testcontainer Postgres + httptest setup as
// handlers_test.go (setupFixture) to exercise the placement endpoints
// end-to-end against a real DB with full migrations applied.
//
// Coverage (7 named test cases per plan spec):
//   1. TestPlacementCRUD          — POST/PATCH/DELETE happy path + DB row + audit entries
//   2. TestPlacement_RejectsFractionOutOfRange — x_frac=1.5 → 422
//   3. TestPlacement_RejectsSiteMismatch       — device on Site A, plan on Site B → 409
//   4. TestPlacement_RejectsUnboundDevice      — device with no active binding → 422
//   5. TestPlacement_UpsertMovesPin            — first POST pins to plan1, second POST to plan2
//   6. TestListPlacements_DenormalizedFields   — response includes device_name, utility_class, etc.
//   7. TestPlacement_DBCheckFiresEvenIfHandlerSkipped — direct DB defense-in-depth

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedProfile inserts a bare-minimum device_profile row (or reuses axioma_w1
// from migration seeds). Returns the profile ID.
func seedProfile(t *testing.T, f *fixture) string {
	t.Helper()
	ctx := context.Background()
	var id string
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT id::text FROM device_profile WHERE slug = 'axioma_w1' LIMIT 1`,
	).Scan(&id))
	return id
}

// seedDeviceRow inserts a device row directly (bypassing CS). Returns device ID.
// dev_eui must be exactly 16 lowercase hex chars per device_dev_eui_hex16 CHECK.
func seedDeviceRow(t *testing.T, f *fixture, profileID string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	// Generate a valid 16-lowercase-hex-char EUI using a fresh UUID's first 8 bytes.
	rawID := uuid.New()
	devEUI := hex.EncodeToString(rawID[:8]) // always exactly 16 hex chars, lowercase
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ($1, $2, $3::uuid) RETURNING id::text`,
		devEUI, "Dev-"+devEUI[:8], profileID,
	).Scan(&id))
	return id
}

// seedMP inserts a metering_point under siteID. Returns MP ID.
func seedMPRow(t *testing.T, f *fixture, siteID uuid.UUID) string {
	t.Helper()
	ctx := context.Background()
	var id string
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, $2, 'water') RETURNING id::text`,
		siteID, "MP "+uuid.NewString()[:6],
	).Scan(&id))
	return id
}

// bindDevice creates an active binding between deviceID and mpID.
func bindDevice(t *testing.T, f *fixture, deviceID, mpID string) {
	t.Helper()
	_, err := f.deps.Pool.Exec(context.Background(),
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1::uuid, $2::uuid, now(), 0)`, mpID, deviceID,
	)
	require.NoError(t, err)
}

// uploadPlan uploads a small PNG via the API and returns the plan ID.
func uploadPlan(t *testing.T, f *fixture, siteID uuid.UUID, label string) string {
	t.Helper()
	body, ct := buildMultipart(t, smallPNG(t), label, "0", "plan.png")
	req, _ := http.NewRequest(http.MethodPost,
		f.server.URL+"/api/sites/"+siteID.String()+"/floor-plans", body)
	req.Header.Set("Content-Type", ct)
	resp, err := f.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var plan map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&plan))
	return plan["ID"].(map[string]any)["Bytes"].(string)
}

// doPlacementJSON performs a JSON request to a placement endpoint.
func doPlacementJSON(t *testing.T, f *fixture, method, path string, body any) *http.Response {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, f.server.URL+path, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	require.NoError(t, err)
	return resp
}

// planIDFromUpload uploads a floor plan image and returns its DB uuid.UUID.
// Uses the next available sort_order for the site to avoid UNIQUE(site_id, sort_order) conflicts.
func planIDFromUpload(t *testing.T, f *fixture, siteID uuid.UUID, label string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	// Get the next sort_order for this site (max + 1, or 0 if none exist).
	var maxOrder int
	_ = f.deps.Pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(sort_order), -1) FROM floor_plan WHERE site_id = $1`, siteID,
	).Scan(&maxOrder)
	nextOrder := maxOrder + 1

	body, ct := buildMultipart(t, smallPNG(t), label, fmt.Sprintf("%d", nextOrder), "plan.png")
	req, _ := http.NewRequest(http.MethodPost,
		f.server.URL+"/api/sites/"+siteID.String()+"/floor-plans", body)
	req.Header.Set("Content-Type", ct)
	resp, err := f.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Query the DB for the just-inserted plan ID (label is unique per test).
	var planID string
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT id::text FROM floor_plan WHERE site_id = $1 AND label = $2 LIMIT 1`,
		siteID, label,
	).Scan(&planID))
	id, err := uuid.Parse(planID)
	require.NoError(t, err)
	return id
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestPlacementCRUD — POST/PATCH/DELETE happy path.
// Verifies: DB row exists after POST, nudge updates coords, DELETE removes row,
// audit entries written inside same tx.
func TestPlacementCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := setupFixture(t)
	ctx := context.Background()

	profileID := seedProfile(t, f)
	deviceID := seedDeviceRow(t, f, profileID)
	mpID := seedMPRow(t, f, f.siteID)
	bindDevice(t, f, deviceID, mpID)
	planID := planIDFromUpload(t, f, f.siteID, "CRUD Plan")

	// POST placement → 201.
	pinResp := doPlacementJSON(t, f, http.MethodPost,
		"/api/floor-plans/"+planID.String()+"/placements",
		map[string]any{"device_id": deviceID, "x_frac": 0.5, "y_frac": 0.5},
	)
	defer pinResp.Body.Close()
	require.Equal(t, http.StatusCreated, pinResp.StatusCode)

	// DB row must exist.
	var xFrac float32
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT x_frac FROM device_floor_plan_placement WHERE device_id = $1::uuid`, deviceID,
	).Scan(&xFrac))
	require.InDelta(t, 0.5, xFrac, 0.001)

	// Audit entry: placement.pin.
	var action string
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT action FROM audit_log WHERE entity_id = $1::uuid AND action = 'placement.pin'`, deviceID,
	).Scan(&action))
	require.Equal(t, "placement.pin", action)

	// PATCH nudge → 200.
	nudgeResp := doPlacementJSON(t, f, http.MethodPatch,
		"/api/floor-plans/"+planID.String()+"/placements/"+deviceID,
		map[string]any{"x_frac": 0.8, "y_frac": 0.2},
	)
	defer nudgeResp.Body.Close()
	require.Equal(t, http.StatusOK, nudgeResp.StatusCode)

	// Coords updated in DB.
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT x_frac FROM device_floor_plan_placement WHERE device_id = $1::uuid`, deviceID,
	).Scan(&xFrac))
	require.InDelta(t, 0.8, xFrac, 0.001)

	// Audit entry: placement.nudge.
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT action FROM audit_log WHERE entity_id = $1::uuid AND action = 'placement.nudge'`, deviceID,
	).Scan(&action))
	require.Equal(t, "placement.nudge", action)

	// DELETE → 204.
	delResp := doPlacementJSON(t, f, http.MethodDelete,
		"/api/floor-plans/"+planID.String()+"/placements/"+deviceID, nil,
	)
	defer delResp.Body.Close()
	require.Equal(t, http.StatusNoContent, delResp.StatusCode)

	// Row must be gone.
	var count int
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT count(*) FROM device_floor_plan_placement WHERE device_id = $1::uuid`, deviceID,
	).Scan(&count))
	require.Equal(t, 0, count)

	// Audit entry: placement.remove.
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT action FROM audit_log WHERE entity_id = $1::uuid AND action = 'placement.remove'`, deviceID,
	).Scan(&action))
	require.Equal(t, "placement.remove", action)
}

// TestPlacement_RejectsFractionOutOfRange — x_frac=1.5 → 422.
func TestPlacement_RejectsFractionOutOfRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := setupFixture(t)

	profileID := seedProfile(t, f)
	deviceID := seedDeviceRow(t, f, profileID)
	mpID := seedMPRow(t, f, f.siteID)
	bindDevice(t, f, deviceID, mpID)
	planID := planIDFromUpload(t, f, f.siteID, "OOR Plan")

	resp := doPlacementJSON(t, f, http.MethodPost,
		"/api/floor-plans/"+planID.String()+"/placements",
		map[string]any{"device_id": deviceID, "x_frac": 1.5, "y_frac": 0.5},
	)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "fraction_out_of_range", body["error"])
}

// TestPlacement_RejectsSiteMismatch — device bound to Site A, plan belongs to
// Site B → 409 with body containing "site_mismatch".
func TestPlacement_RejectsSiteMismatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := setupFixture(t)
	ctx := context.Background()

	// Create a second site (Site B).
	var siteBID string
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('Site B', 'UTC') RETURNING id::text`,
	).Scan(&siteBID))
	siteB, _ := uuid.Parse(siteBID)

	// Upload a plan to Site B.
	planID := planIDFromUpload(t, f, siteB, "Site B Plan")

	// Seed a device bound to Site A (f.siteID).
	profileID := seedProfile(t, f)
	deviceID := seedDeviceRow(t, f, profileID)
	mpID := seedMPRow(t, f, f.siteID) // bound to Site A
	bindDevice(t, f, deviceID, mpID)

	// POST to Site B's plan with Site A's device → 409 site_mismatch.
	resp := doPlacementJSON(t, f, http.MethodPost,
		"/api/floor-plans/"+planID.String()+"/placements",
		map[string]any{"device_id": deviceID, "x_frac": 0.5, "y_frac": 0.5},
	)
	defer resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "site_mismatch", body["error"])
}

// TestPlacement_RejectsUnboundDevice — device exists but has no active binding → 422.
func TestPlacement_RejectsUnboundDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := setupFixture(t)

	profileID := seedProfile(t, f)
	// Device with NO binding.
	deviceID := seedDeviceRow(t, f, profileID)
	planID := planIDFromUpload(t, f, f.siteID, "Unbound Plan")

	resp := doPlacementJSON(t, f, http.MethodPost,
		"/api/floor-plans/"+planID.String()+"/placements",
		map[string]any{"device_id": deviceID, "x_frac": 0.5, "y_frac": 0.5},
	)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "device_unbound", body["error"])
}

// TestPlacement_UpsertMovesPin — POST to plan1 pins device; POST to plan2 moves
// pin (UPSERT on device_id PK). Only one row in placement table, floor_plan_id=plan2.
// Second POST uses action=placement.nudge in audit (re-pin of existing placement).
func TestPlacement_UpsertMovesPin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := setupFixture(t)
	ctx := context.Background()

	profileID := seedProfile(t, f)
	deviceID := seedDeviceRow(t, f, profileID)
	mpID := seedMPRow(t, f, f.siteID)
	bindDevice(t, f, deviceID, mpID)

	plan1ID := planIDFromUpload(t, f, f.siteID, "Plan 1")
	plan2ID := planIDFromUpload(t, f, f.siteID, "Plan 2")

	// First POST: pin to plan1.
	resp1 := doPlacementJSON(t, f, http.MethodPost,
		"/api/floor-plans/"+plan1ID.String()+"/placements",
		map[string]any{"device_id": deviceID, "x_frac": 0.1, "y_frac": 0.1},
	)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	// Second POST: move to plan2.
	resp2 := doPlacementJSON(t, f, http.MethodPost,
		"/api/floor-plans/"+plan2ID.String()+"/placements",
		map[string]any{"device_id": deviceID, "x_frac": 0.9, "y_frac": 0.9},
	)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	// Only one row must exist; it belongs to plan2.
	var rowCount int
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT count(*) FROM device_floor_plan_placement WHERE device_id = $1::uuid`, deviceID,
	).Scan(&rowCount))
	require.Equal(t, 1, rowCount, "UPSERT must keep exactly one row per device")

	var floorPlanID pgtype.UUID
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT floor_plan_id FROM device_floor_plan_placement WHERE device_id = $1::uuid`, deviceID,
	).Scan(&floorPlanID))
	require.Equal(t, plan2ID, uuid.UUID(floorPlanID.Bytes), "device must now be on plan2")

	// Second audit entry must use action=placement.nudge (re-pin).
	var nudgeCount int
	require.NoError(t, f.deps.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_id = $1::uuid AND action = 'placement.nudge'`, deviceID,
	).Scan(&nudgeCount))
	require.Equal(t, 1, nudgeCount, "second POST must produce a placement.nudge audit entry")
}

// TestListPlacements_DenormalizedFields — GET /api/floor-plans/:id/placements
// returns rows including DeviceName, UtilityClass, LastSeenAt, ExpectedIntervalS,
// BatteryPct, Rssi (may be null when no measurement exists).
func TestListPlacements_DenormalizedFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := setupFixture(t)

	profileID := seedProfile(t, f)
	deviceID := seedDeviceRow(t, f, profileID)
	mpID := seedMPRow(t, f, f.siteID)
	bindDevice(t, f, deviceID, mpID)
	planID := planIDFromUpload(t, f, f.siteID, "List Plan")

	// Pin the device.
	pinResp := doPlacementJSON(t, f, http.MethodPost,
		"/api/floor-plans/"+planID.String()+"/placements",
		map[string]any{"device_id": deviceID, "x_frac": 0.3, "y_frac": 0.7},
	)
	defer pinResp.Body.Close()
	require.Equal(t, http.StatusCreated, pinResp.StatusCode)

	// List placements.
	listResp, err := f.client.Get(f.server.URL + "/api/floor-plans/" + planID.String() + "/placements")
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var rows []map[string]any
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&rows))
	require.Len(t, rows, 1)

	row := rows[0]
	// DeviceName must be present and non-empty.
	require.NotEmpty(t, row["DeviceName"], "DeviceName must be present in list response")
	// UtilityClass field must be present (may be null for unbound device, but here bound to water MP).
	_, hasUtility := row["UtilityClass"]
	require.True(t, hasUtility, "UtilityClass must be present in list response")
	// ExpectedIntervalS must be present.
	_, hasInterval := row["ExpectedIntervalS"]
	require.True(t, hasInterval, "ExpectedIntervalS must be present in list response")
	// BatteryPct and Rssi may be null (no measurement seeded) but field must exist.
	_, hasBattery := row["BatteryPct"]
	require.True(t, hasBattery, "BatteryPct must be present in list response")
	_, hasRssi := row["Rssi"]
	require.True(t, hasRssi, "Rssi must be present in list response")
}

// TestPlacement_DBCheckFiresEvenIfHandlerSkipped — defense-in-depth:
// direct INSERT bypassing the handler with x_frac=1.5 must be rejected by the
// DB CHECK constraint (23514).
func TestPlacement_DBCheckFiresEvenIfHandlerSkipped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := setupFixture(t)
	ctx := context.Background()

	profileID := seedProfile(t, f)
	deviceID := seedDeviceRow(t, f, profileID)
	planID := planIDFromUpload(t, f, f.siteID, "Check Plan")

	_, err := f.deps.Pool.Exec(ctx,
		`INSERT INTO device_floor_plan_placement (device_id, floor_plan_id, x_frac, y_frac)
		 VALUES ($1::uuid, $2::uuid, 1.5, 0.5)`,
		deviceID, planID,
	)
	require.Error(t, err, "DB CHECK constraint must reject x_frac=1.5")
	require.Contains(t, err.Error(), "23514", "error must be a CHECK violation (code 23514)")
}
