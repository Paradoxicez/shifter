package alert

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestBuildPayload_CanonicalShape — the D-12 wire payload has EXACTLY the
// canonical top-level keys for a threshold fire, no extras, no missing fields,
// and the comparison string is the operator name ("gt"), not the symbol (">").
// Webhook deliverer V2-NOTIF-01 deserializes this exact shape.
func TestBuildPayload_CanonicalShape(t *testing.T) {
	ruleID := uuid.New()
	targetID := uuid.New()
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)

	bytes, err := BuildPayload(BuildPayloadInput{
		RuleID:   ruleID,
		RuleKind: "threshold_instantaneous",
		Severity: "critical",
		Target: PayloadTarget{
			EntityType: "metering_point",
			EntityID:   targetID,
			Label:      "MP-Lobby-Water",
		},
		Value:       42.7,
		Threshold:   40.0,
		Comparison:  "gt",
		Unit:        "m3/h",
		FiredAt:     now,
		InstallName: "Acme Water Co.",
	})
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(bytes, &wire))

	// Exact top-level keys for the canonical (non-offline, non-anomaly) shape.
	expectedKeys := []string{
		"rule_id", "rule_kind", "severity", "target",
		"value", "threshold", "comparison", "unit",
		"fired_at", "install",
	}
	for _, k := range expectedKeys {
		require.Contains(t, wire, k, "missing canonical key %q", k)
	}
	for k := range wire {
		require.Contains(t, expectedKeys, k, "unexpected top-level key %q", k)
	}

	require.Equal(t, "threshold_instantaneous", wire["rule_kind"])
	require.Equal(t, "critical", wire["severity"])
	require.InDelta(t, 42.7, wire["value"], 1e-9)
	require.InDelta(t, 40.0, wire["threshold"], 1e-9)
	// D-12: comparison stored as the operator name, never the symbol.
	require.Equal(t, "gt", wire["comparison"], "comparison must be operator name, not symbol")
	require.NotEqual(t, ">", wire["comparison"], "comparison must NOT be the symbol form")

	target := wire["target"].(map[string]any)
	require.Equal(t, "metering_point", target["entity_type"])
	require.Equal(t, "MP-Lobby-Water", target["label"])
	require.Equal(t, targetID.String(), target["entity_id"])

	install := wire["install"].(map[string]any)
	require.Equal(t, "Acme Water Co.", install["display_name"])

	// fired_at must round-trip to UTC RFC3339.
	require.Equal(t, "2026-05-12T10:00:00Z", wire["fired_at"])
}

// TestBuildPayload_OfflineDeviceExtensions — for rule_kind=offline_device the
// payload includes last_uplink_at and expected_interval_s.
func TestBuildPayload_OfflineDeviceExtensions(t *testing.T) {
	last := time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC)
	interval := int32(900)
	bytes, err := BuildPayload(BuildPayloadInput{
		RuleID:            uuid.New(),
		RuleKind:          "offline_device",
		Severity:          "warning",
		Target:            PayloadTarget{EntityType: "device", EntityID: uuid.New(), Label: "dev-1"},
		Value:             2700,
		Threshold:         float64(3 * interval),
		Comparison:        "gt",
		Unit:              "seconds_since_last_uplink",
		FiredAt:           time.Now().UTC(),
		InstallName:       "X",
		LastUplinkAt:      &last,
		ExpectedIntervalS: &interval,
	})
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(bytes, &wire))

	require.Contains(t, wire, "last_uplink_at")
	require.Contains(t, wire, "expected_interval_s")
	require.Equal(t, "2026-05-12T08:00:00Z", wire["last_uplink_at"])
	require.InDelta(t, 900.0, wire["expected_interval_s"], 1e-9)
}

// TestBuildPayload_OfflineGatewaySuppressionCount — for rule_kind=offline_gateway
// the payload carries the suppresses_n_devices count populated by the worker
// from the gateway-suppression map.
func TestBuildPayload_OfflineGatewaySuppressionCount(t *testing.T) {
	n := int32(12)
	bytes, err := BuildPayload(BuildPayloadInput{
		RuleID:             uuid.New(),
		RuleKind:           "offline_gateway",
		Severity:           "critical",
		Target:             PayloadTarget{EntityType: "gateway", EntityID: uuid.New(), Label: "GW-Roof"},
		Value:              0,
		Threshold:          0,
		Comparison:         "gt",
		Unit:               "devices",
		FiredAt:            time.Now().UTC(),
		InstallName:        "X",
		SuppressesNDevices: &n,
	})
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(bytes, &wire))
	require.Contains(t, wire, "suppresses_n_devices")
	require.InDelta(t, 12.0, wire["suppresses_n_devices"], 1e-9)
}

// TestQueries_ListOfflineDevicesWithGatewayStatus — seeded fixture: one device
// with last_seen 5×expected_interval ago + gateway last_seen also stale →
// returns row with gateway_offline = TRUE. Pins the eval-time D-14 query.
func TestQueries_ListOfflineDevicesWithGatewayStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed a vendor profile with a known expected_interval_s. The seeded
	// axioma_w1 profile shipped by migration 0010 already has
	// expected_interval_s = 3600 (after migration 0023 backfill); use it.
	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1' LIMIT 1`).Scan(&profileID))

	// Stale gateway: last_seen_at = 5 × 3600s = 5h ago.
	var gatewayID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO gateway (gateway_id, name, region, last_seen_at)
		 VALUES ('aabbccddeeff0011', 'GW-Stale', 'AS923_2', now() - INTERVAL '5 hours')
		 RETURNING id`).Scan(&gatewayID))

	// Stale device: last_seen_at = 5 × 3600s = 5h ago. Bound to the stale gateway.
	var deviceID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id, gateway_id, last_seen_at)
		 VALUES ('00112233445566aa', 'Dev-Stale', $1, $2, now() - INTERVAL '5 hours')
		 RETURNING id`, profileID, gatewayID).Scan(&deviceID))

	q := sqlc.New(pool)
	rows, err := q.ListOfflineDevicesWithGatewayStatus(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rows, "expected at least one offline candidate")

	// Find our device row.
	var found *sqlc.ListOfflineDevicesWithGatewayStatusRow
	for i := range rows {
		if rows[i].DeviceID == toPgUUID(t, deviceID) {
			found = &rows[i]
			break
		}
	}
	require.NotNil(t, found, "seeded stale device must appear in the result")
	require.NotNil(t, found.GatewayOffline, "gateway_offline must be non-NULL (gateway is bound)")
	require.True(t, *found.GatewayOffline, "gateway_offline must be TRUE when gateway last_seen is also 5× expected_interval old")
	require.Equal(t, int32(3600), found.ExpectedIntervalS)
}

func toPgUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	u, err := uuid.Parse(s)
	require.NoError(t, err)
	return pgtype.UUID{Bytes: u, Valid: true}
}
