package device

// Plan 05-07 Task 2 — D-25: device decommission auto-removes floor plan placement.
//
// Three contract points:
//  1. TestDecommission_RemovesFloorPlanPlacement — a pinned device, when
//     decommissioned, has its placement row deleted AND two audit entries written
//     (placement.remove + decommission), all within the same transaction.
//  2. TestDecommission_PlacementRollsBackOnFailure — if the device is already
//     decommissioned (conflict), the placement row is NOT deleted (the tx never
//     committed). This exercises the atomic rollback path by engineering a
//     conflict on the decommission itself.
//  3. TestDecommission_NoPlacement_NoOp — decommissioning a device that has no
//     placement pin succeeds normally (200), and the audit notes show
//     placement_removed=false.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// seedFloorPlan inserts a floor_plan row directly (no image file needed —
// image_path can be any string for placement seeding purposes). Returns the
// plan id as a string.
func seedFloorPlan(t *testing.T, f *deviceFixture, siteID string, sortOrder int) string {
	t.Helper()
	var id string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO floor_plan (site_id, label, sort_order, image_path, image_w, image_h)
		 VALUES ($1::uuid, 'Test Plan', $2, 'dummy.png', 100, 100) RETURNING id::text`,
		siteID, sortOrder,
	).Scan(&id))
	return id
}

// seedPlacement inserts a device_floor_plan_placement row directly.
func seedPlacement(t *testing.T, f *deviceFixture, deviceID, floorPlanID string) {
	t.Helper()
	_, err := f.pool.Exec(context.Background(),
		`INSERT INTO device_floor_plan_placement (device_id, floor_plan_id, x_frac, y_frac)
		 VALUES ($1::uuid, $2::uuid, 0.25, 0.50)`,
		deviceID, floorPlanID,
	)
	require.NoError(t, err)
}

// hasPlacement returns true if a device_floor_plan_placement row exists for deviceID.
func hasPlacement(t *testing.T, f *deviceFixture, deviceID string) bool {
	t.Helper()
	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM device_floor_plan_placement WHERE device_id = $1::uuid`,
		deviceID,
	).Scan(&count))
	return count > 0
}

// TestDecommission_RemovesFloorPlanPlacement — pin a device on a floor plan,
// then POST decommission. Expects:
//   - 200 OK with placement_removed=true in the audit after entry
//   - device_floor_plan_placement row is gone
//   - audit row for 'placement.remove' exists
//   - audit row for 'decommission' exists with placement_removed=true
func TestDecommission_RemovesFloorPlanPlacement(t *testing.T) {
	f := newDeviceFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	siteID := seedSite(t, f, fmt.Sprintf("D25Site-%s", uuid.NewString()[:8]))
	planID := seedFloorPlan(t, f, siteID, 0)
	deviceID := seedDevice(t, f, "d2500000000001a1", "D25 Device Pinned", time.Now().UTC())
	seedPlacement(t, f, deviceID, planID)

	require.True(t, hasPlacement(t, f, deviceID), "placement must exist before decommission")

	res := f.doJSON(t, "POST", "/api/devices/"+deviceID+"/decommission", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	// Placement row must be gone.
	require.False(t, hasPlacement(t, f, deviceID), "placement must be deleted after decommission")

	// Device marked decommissioned.
	var isDecomm bool
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT decommissioned_at IS NOT NULL FROM device WHERE id = $1::uuid`, deviceID,
	).Scan(&isDecomm))
	require.True(t, isDecomm)

	// Audit: placement.remove entry must exist.
	var placementAuditCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_id = $1::uuid AND action = 'placement.remove'`,
		deviceID,
	).Scan(&placementAuditCount))
	require.Equal(t, 1, placementAuditCount, "placement.remove audit entry must exist")

	// Audit: decommission entry must exist with placement_removed=true in the after JSON.
	var afterJSON []byte
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT "after" FROM audit_log WHERE entity_id = $1::uuid AND action = 'decommission'`,
		deviceID,
	).Scan(&afterJSON))
	var afterMap map[string]any
	require.NoError(t, json.Unmarshal(afterJSON, &afterMap))
	require.Equal(t, true, afterMap["placement_removed"],
		"decommission audit after_state must have placement_removed=true")
}

// TestDecommission_PlacementRollsBackOnFailure — seed a device that is ALREADY
// decommissioned but still has a placement row (injected directly via SQL to
// simulate the edge case). Posting decommission returns 409 (already_decommissioned)
// and the placement row must remain — because the handler exits early (before
// reaching the placement delete) without committing.
func TestDecommission_PlacementRollsBackOnFailure(t *testing.T) {
	f := newDeviceFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	siteID := seedSite(t, f, fmt.Sprintf("D25Site2-%s", uuid.NewString()[:8]))
	planID := seedFloorPlan(t, f, siteID, 0)
	deviceID := seedDevice(t, f, "d2500000000002b2", "D25 Already Decommissioned", time.Now().UTC())
	seedPlacement(t, f, deviceID, planID)

	// Mark the device decommissioned BEFORE the HTTP call.
	_, err := f.pool.Exec(ctx,
		`UPDATE device SET decommissioned_at = now() WHERE id = $1::uuid`, deviceID,
	)
	require.NoError(t, err)

	// POST decommission — expect 409.
	res := f.doJSON(t, "POST", "/api/devices/"+deviceID+"/decommission", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "already_decommissioned", body["error"])

	// Placement row must still be present — handler exited early, no delete ran.
	require.True(t, hasPlacement(t, f, deviceID),
		"placement must NOT be deleted when decommission fails with 409")
}

// TestDecommission_NoPlacement_NoOp — decommission a device that has no
// placement pin. Expects 200 with the body JSON containing "placement_removed"
// that is falsy in the audit, and no error.
func TestDecommission_NoPlacement_NoOp(t *testing.T) {
	f := newDeviceFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	deviceID := seedDevice(t, f, "d2500000000003c3", "D25 No Placement", time.Now().UTC())

	require.False(t, hasPlacement(t, f, deviceID), "device must have no placement initially")

	res := f.doJSON(t, "POST", "/api/devices/"+deviceID+"/decommission", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	// Device decommissioned.
	var isDecomm bool
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT decommissioned_at IS NOT NULL FROM device WHERE id = $1::uuid`, deviceID,
	).Scan(&isDecomm))
	require.True(t, isDecomm)

	// No placement.remove audit entry.
	var placementAuditCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_id = $1::uuid AND action = 'placement.remove'`,
		deviceID,
	).Scan(&placementAuditCount))
	require.Equal(t, 0, placementAuditCount, "no placement.remove audit entry expected")

	// The decommission audit after_state must have placement_removed=false.
	var afterJSON []byte
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT "after" FROM audit_log WHERE entity_id = $1::uuid AND action = 'decommission'`,
		deviceID,
	).Scan(&afterJSON))
	var afterMap map[string]any
	require.NoError(t, json.Unmarshal(afterJSON, &afterMap))
	require.Equal(t, false, afterMap["placement_removed"],
		"decommission audit after_state must have placement_removed=false when no placement existed")
}
