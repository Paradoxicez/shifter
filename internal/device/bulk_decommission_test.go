package device

// Phase 3 Plan 03-06 / Task 2 — POST /api/devices/bulk-decommission (D-17).
//
// Per-row Serializable tx semantics: each device-id either:
//   - succeeds: CS DeleteDevice (best-effort, CS-not-found OK), q.DecommissionDevice,
//     audit row, tx commit.
//   - fails:    the per-row tx rolls back, sibling rows continue. The response
//     reports {succeeded, failed, outcomes:[{id, status, reason?}]}.
//
// Tests verify the four contract points:
//   1. Partial-success: missing id + valid ids in one batch → succeeded=2,
//      failed=1, the failing row carries reason="not_found".
//   2. Atomic per-row: a failed row does NOT roll back already-successful
//      siblings (sibling row's decommissioned_at IS NOT NULL after the call).
//   3. Max-batch cap: 201 ids → 400 with detail "max 200 devices per request".
//   4. Viewer 403: viewer attempt rejected; NO device decommissioned.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// bulkResponse matches the JSON shape emitted by bulkDecommissionDevices.
type bulkResponse struct {
	Succeeded int              `json:"succeeded"`
	Failed    int              `json:"failed"`
	Outcomes  []map[string]any `json:"outcomes"`
}

// TestBulkDecommissionDevices_PartialSuccess — 2 valid + 1 missing → 2 ok, 1 failed.
func TestBulkDecommissionDevices_PartialSuccess(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	d1 := seedDevice(t, f, "ba00000000000001", "bulk-1", time.Now().UTC())
	d2 := seedDevice(t, f, "ba00000000000002", "bulk-2", time.Now().UTC())
	missing := uuid.NewString() // never inserted

	res := f.doJSON(t, "POST", "/api/devices/bulk-decommission", BulkDecommissionRequest{
		DeviceIDs: []string{d1, missing, d2},
		Reason:    "fleet sunset",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var body bulkResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, 2, body.Succeeded)
	require.Equal(t, 1, body.Failed)
	require.Len(t, body.Outcomes, 3)

	// Verify the missing id was the one marked failed.
	for _, oc := range body.Outcomes {
		if oc["id"] == missing {
			require.Equal(t, "failed", oc["status"])
			require.Equal(t, "not_found", oc["reason"])
		} else {
			require.Equal(t, "decommissioned", oc["status"])
		}
	}

	// Both valid devices were actually decommissioned in PG.
	for _, id := range []string{d1, d2} {
		var has bool
		require.NoError(t, f.pool.QueryRow(context.Background(),
			`SELECT decommissioned_at IS NOT NULL FROM device WHERE id = $1::uuid`, id,
		).Scan(&has))
		require.True(t, has, "device %s must be decommissioned", id)
	}

	// Audit rows: one per successful decommission, action='decommission',
	// notes prefixed with "bulk decommission".
	for _, id := range []string{d1, d2} {
		var notes string
		require.NoError(t, f.pool.QueryRow(context.Background(),
			`SELECT notes FROM audit_log WHERE entity_id = $1::uuid AND action = 'decommission' ORDER BY time DESC LIMIT 1`,
			id,
		).Scan(&notes))
		require.Contains(t, notes, "bulk decommission")
	}
}

// TestBulkDecommissionDevices_Atomic — failure of one row does NOT roll
// back the rows already committed in earlier iterations. We seed 3 devices
// where the middle one is already decommissioned (so the 2nd row fails with
// reason "already_decommissioned"), and verify the 1st AND 3rd rows are
// still decommissioned after the call.
func TestBulkDecommissionDevices_Atomic(t *testing.T) {
	f := newDeviceFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	d1 := seedDevice(t, f, "bb00000000000001", "atom-1", time.Now().UTC())
	d2 := seedDevice(t, f, "bb00000000000002", "atom-2", time.Now().UTC())
	d3 := seedDevice(t, f, "bb00000000000003", "atom-3", time.Now().UTC())

	// Pre-decommission d2 so its bulk row fails.
	_, err := f.pool.Exec(ctx,
		`UPDATE device SET decommissioned_at = now() WHERE id = $1::uuid`, d2)
	require.NoError(t, err)

	res := f.doJSON(t, "POST", "/api/devices/bulk-decommission", BulkDecommissionRequest{
		DeviceIDs: []string{d1, d2, d3},
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var body bulkResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, 2, body.Succeeded)
	require.Equal(t, 1, body.Failed)

	// d1 and d3 must be decommissioned (their per-row tx committed).
	for _, id := range []string{d1, d3} {
		var has bool
		require.NoError(t, f.pool.QueryRow(ctx,
			`SELECT decommissioned_at IS NOT NULL FROM device WHERE id = $1::uuid`, id,
		).Scan(&has))
		require.True(t, has, "Per-row Serializable: sibling commit must persist (id=%s)", id)
	}
}

// TestBulkDecommissionDevices_MaxBatch200 — 201 ids → 400.
func TestBulkDecommissionDevices_MaxBatch200(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	ids := make([]string, 201)
	for i := range ids {
		ids[i] = uuid.NewString()
	}
	res := f.doJSON(t, "POST", "/api/devices/bulk-decommission", BulkDecommissionRequest{
		DeviceIDs: ids,
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "too_many_devices", body["error"])
	require.Contains(t, body["detail"], "max 200")
}

// TestBulkDecommissionDevices_Viewer403 — viewer POST → 403; nothing is
// decommissioned.
func TestBulkDecommissionDevices_Viewer403(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")
	d1 := seedDevice(t, f, "bc00000000000001", "viewer-target", time.Now().UTC())

	f.seedRole(t, "viewer")
	res := f.doJSON(t, "POST", "/api/devices/bulk-decommission", BulkDecommissionRequest{
		DeviceIDs: []string{d1},
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	// Device not decommissioned.
	var has bool
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT decommissioned_at IS NOT NULL FROM device WHERE id = $1::uuid`, d1,
	).Scan(&has))
	require.False(t, has, "viewer reject must not decommission")
}
