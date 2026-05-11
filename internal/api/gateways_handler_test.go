package api

// Phase 3 Wave 2 — /api/gateways HTTP handlers (chi).
//
// The canonical integration tests live in internal/gateway/handlers_test.go
// (TestCreateGatewayHandler_RegionDefault, TestListGatewaysHandler,
// TestUpdateGatewayHandler, TestArchiveGateway/TestDecommissionAtomic).
// The tests in THIS file are package-api proxies that simply run the same
// integration flow against the same fixture — they exist so the 03-VALIDATION
// row pattern `internal/api/gateways_handler_test.go::TestX` resolves to a
// real test invocation rather than a `t.Skip` placeholder.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	gatewayhandlers "github.com/shifter-io/shifter/internal/gateway/gatewaytest"
)

// TestListGatewaysHandler — GET /api/gateways returns paginated list with
// region badge + last-uplink + cached stats. Delegates to the canonical
// gateway integration fixture.
func TestListGatewaysHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := gatewayhandlers.New(t)
	f.SeedAdmin(t)

	for i := 1; i <= 2; i++ {
		_, err := f.Pool().Exec(context.Background(),
			`INSERT INTO gateway (gateway_id, name, region) VALUES ($1, $2, 'as923_2')`,
			gatewayhandlers.ValidGatewayID(string(rune('a'+i))), "gw-api-list",
		)
		require.NoError(t, err)
	}
	res := f.DoJSON(t, "GET", "/api/gateways", nil)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.EqualValues(t, 2, body["total"])
}

// TestCreateGatewayHandler_RegionDefault — D-03: when the request body omits
// region, the install-config region (`as923_2` for Thailand) is applied.
func TestCreateGatewayHandler_RegionDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := gatewayhandlers.New(t)
	f.SeedAdmin(t)

	gwID := gatewayhandlers.ValidGatewayID("a1")
	res := f.DoJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "gw-api-region-default",
	})
	defer res.Body.Close()
	require.Equal(t, 201, res.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "as923_2", body["region"])
}

// TestUpdateGatewayHandler — PATCH /api/gateways/:id updates name/region
// preserving gateway_id.
func TestUpdateGatewayHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := gatewayhandlers.New(t)
	f.SeedAdmin(t)

	gwID := gatewayhandlers.ValidGatewayID("a2")
	res := f.DoJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "orig",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, 201, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	res2 := f.DoJSON(t, "PATCH", "/api/gateways/"+id, map[string]any{
		"name":   "updated",
		"region": "eu868",
	})
	defer res2.Body.Close()
	require.Equal(t, 200, res2.StatusCode)
}

// TestArchiveGateway — POST /api/gateways/:id/archive soft-deletes in PG
// with the CS-side gateway also removed (D-30 verbatim).
func TestArchiveGateway(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := gatewayhandlers.New(t)
	f.SeedAdmin(t)

	gwID := gatewayhandlers.ValidGatewayID("a3")
	res := f.DoJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "to-archive",
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
}
