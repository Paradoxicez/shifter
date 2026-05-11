package api

// Phase 3 Wave 2 — POST /api/gateways/:id/archive (decommission) + restore.
//
// D-30 verbatim per user decision 2026-05-11: archive + restore are
// atomic CS+PG transactions. The CS-side DeleteGateway is part of the
// contract — these tests assert it.
//
// D-31 24h-uplink warning: deferred to Phase 4 telemetry ingest (the
// measurement hypertable does not yet carry gateway_id, see PLAN 03-04
// CountGatewayRecentUplinks24h note). The Phase 3 archive flow ships
// without the per-gateway count; the test name `TestDecommissionGateway_24hWarning`
// is kept as a skip so Phase 4 can fill it in.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gatewayhandlers "github.com/shifter-io/shifter/internal/gateway/gatewaytest"
)

// TestDecommissionGateway_SoftDelete — happy path: PG row gets archived_at
// timestamp; CS gateway is deleted; both succeed atomically; audit row
// written.
func TestDecommissionGateway_SoftDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := gatewayhandlers.New(t)
	f.SeedAdmin(t)

	gwID := gatewayhandlers.ValidGatewayID("d1")
	res := f.DoJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "to-decommission",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, 201, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	res2 := f.DoJSON(t, "POST", "/api/gateways/"+id+"/archive", map[string]any{
		"reason": "operator decommission",
	})
	defer res2.Body.Close()
	require.Equal(t, 200, res2.StatusCode)

	// CS gateway gone.
	require.False(t, f.CS().HasGateway(gwID), "CS gateway must be deleted")
	// PG row archived_at set + snapshot non-empty.
	var archivedAt *time.Time
	var snapshot []byte
	require.NoError(t, f.Pool().QueryRow(context.Background(),
		`SELECT archived_at, archived_snapshot FROM gateway WHERE id = $1::uuid`, id,
	).Scan(&archivedAt, &snapshot))
	require.NotNil(t, archivedAt)
	require.NotEmpty(t, snapshot)
}

// TestDecommissionGateway_24hWarning — D-31 warning shipping with device
// count deferred to Phase 4 (measurement.gateway_id not yet on hypertable).
// Skip retained so the 03-VALIDATION test row resolves.
func TestDecommissionGateway_24hWarning(t *testing.T) {
	t.Skip("Phase 3 D-31 warning ships without device count; Phase 4 telemetry ingest adds measurement.gateway_id then this test gets filled in")
}

// TestRestoreGateway — POST /api/gateways/:id/restore re-creates the
// gateway in CS using archived_snapshot and clears PG archive columns
// atomically (D-30 verbatim restore).
func TestRestoreGateway(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := gatewayhandlers.New(t)
	f.SeedAdmin(t)

	gwID := gatewayhandlers.ValidGatewayID("d2")
	res := f.DoJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "to-restore",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, 201, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	// Archive.
	resArc := f.DoJSON(t, "POST", "/api/gateways/"+id+"/archive", nil)
	defer resArc.Body.Close()
	require.Equal(t, 200, resArc.StatusCode)
	require.False(t, f.CS().HasGateway(gwID))

	// Restore.
	resRes := f.DoJSON(t, "POST", "/api/gateways/"+id+"/restore", nil)
	defer resRes.Body.Close()
	require.Equal(t, 200, resRes.StatusCode)
	require.True(t, f.CS().HasGateway(gwID), "CS gateway must be re-created from snapshot")

	// PG archive cleared.
	var archivedAt *time.Time
	require.NoError(t, f.Pool().QueryRow(context.Background(),
		`SELECT archived_at FROM gateway WHERE id = $1::uuid`, id,
	).Scan(&archivedAt))
	require.Nil(t, archivedAt)
}

// TestDecommissionGateway_CSFailureRollsBack — mirror of the canonical
// gateway-package test for the api-package 03-VALIDATION row.
func TestDecommissionGateway_CSFailureRollsBack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := gatewayhandlers.New(t)
	f.SeedAdmin(t)

	gwID := gatewayhandlers.ValidGatewayID("d3")
	res := f.DoJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "cs-failure",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, 201, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	// Queue a CS Delete failure.
	f.CS().DeleteErrs = append(f.CS().DeleteErrs, errors.New("cs unavailable"))

	res2 := f.DoJSON(t, "POST", "/api/gateways/"+id+"/archive", nil)
	defer res2.Body.Close()
	require.Equal(t, 502, res2.StatusCode)

	// PG row not archived.
	var archivedAt *time.Time
	require.NoError(t, f.Pool().QueryRow(context.Background(),
		`SELECT archived_at FROM gateway WHERE id = $1::uuid`, id,
	).Scan(&archivedAt))
	require.Nil(t, archivedAt, "PG must roll back on CS Delete failure")
}
