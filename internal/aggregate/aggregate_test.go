package aggregate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestCAGGHierarchy verifies all four CAGGs exist with correct materialized_only flags.
// - hourly: materialized_only = false (real-time ON per D-11)
// - daily: materialized_only = false (real-time ON per D-11)
// - monthly: materialized_only = true (real-time OFF per D-11)
// - yearly: materialized_only = true (real-time OFF per D-11)
func TestCAGGHierarchy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	type caggRow struct {
		ViewName        string
		MaterializedOnly bool
	}

	rows, err := pool.Query(ctx,
		`SELECT view_name, materialized_only
		 FROM timescaledb_information.continuous_aggregates
		 ORDER BY view_name`)
	require.NoError(t, err)
	defer rows.Close()

	var caggs []caggRow
	for rows.Next() {
		var r caggRow
		require.NoError(t, rows.Scan(&r.ViewName, &r.MaterializedOnly))
		caggs = append(caggs, r)
	}
	require.NoError(t, rows.Err())

	require.Len(t, caggs, 4, "expected exactly 4 CAGGs: hourly/daily/monthly/yearly")

	// Build a map for easy lookup.
	caggMap := make(map[string]bool)
	for _, c := range caggs {
		caggMap[c.ViewName] = c.MaterializedOnly
	}

	// Verify each CAGG exists.
	for _, name := range []string{"measurement_hourly", "measurement_daily", "measurement_monthly", "measurement_yearly"} {
		_, ok := caggMap[name]
		require.True(t, ok, "CAGG %q must exist in continuous_aggregates", name)
	}

	// D-11: real-time ON for hourly + daily (materialized_only = false).
	require.False(t, caggMap["measurement_hourly"], "measurement_hourly must have materialized_only=false (D-11 real-time ON)")
	require.False(t, caggMap["measurement_daily"], "measurement_daily must have materialized_only=false (D-11 real-time ON)")

	// D-11: real-time OFF for monthly + yearly (materialized_only = true).
	require.True(t, caggMap["measurement_monthly"], "measurement_monthly must have materialized_only=true (D-11 real-time OFF)")
	require.True(t, caggMap["measurement_yearly"], "measurement_yearly must have materialized_only=true (D-11 real-time OFF)")
}

// TestRefreshPolicyParams verifies CAGG refresh policy offset values meet D-11 / DATA-12.
// - end_offset >= 2 × max(expected_interval_s) for each CAGG
// - start_offset for hourly <= raw retention (90d)
func TestRefreshPolicyParams(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Get max(expected_interval_s) across all active device profiles.
	var maxIntervalS float64
	err := pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(expected_interval_s), 3600) FROM device_profile WHERE archived_at IS NULL`,
	).Scan(&maxIntervalS)
	require.NoError(t, err)

	// Get raw retention window (in seconds) from TimescaleDB jobs.
	// The raw measurement retention policy was added by 0025_cagg_hourly.up.sql.
	var rawRetentionSeconds float64
	err = pool.QueryRow(ctx, `
		SELECT EXTRACT(EPOCH FROM (config->>'drop_after')::interval)
		FROM timescaledb_information.jobs
		WHERE proc_name = 'policy_retention'
		  AND hypertable_name = 'measurement'
	`).Scan(&rawRetentionSeconds)
	require.NoError(t, err, "raw measurement retention policy must exist")
	require.Greater(t, rawRetentionSeconds, 0.0, "raw retention must be a positive interval")

	// For each CAGG, verify end_offset >= 2 × maxIntervalS.
	// We also verify start_offset <= raw retention for the hourly CAGG (Pitfall #2).
	type policyRow struct {
		ViewName string
		Config   []byte
	}

	rows, err := pool.Query(ctx, `
		SELECT hypertable_name, config
		FROM timescaledb_information.jobs
		WHERE proc_name = 'policy_refresh_continuous_aggregate'
		ORDER BY hypertable_name
	`)
	require.NoError(t, err)
	defer rows.Close()

	var policies []policyRow
	for rows.Next() {
		var r policyRow
		require.NoError(t, rows.Scan(&r.ViewName, &r.Config))
		policies = append(policies, r)
	}
	require.NoError(t, rows.Err())
	require.Len(t, policies, 4, "expected 4 refresh policies (one per CAGG)")

	for _, p := range policies {
		var cfg map[string]interface{}
		require.NoError(t, json.Unmarshal(p.Config, &cfg))

		// Parse end_offset — stored as {"microseconds": N} or as interval string.
		endOffsetSeconds := extractIntervalSeconds(t, cfg, "end_offset")
		require.GreaterOrEqual(t, endOffsetSeconds, 2*maxIntervalS,
			"CAGG %q: end_offset (%.0fs) must be >= 2 × max(expected_interval_s) (%.0fs) per DATA-12",
			p.ViewName, endOffsetSeconds, maxIntervalS)

		// For the hourly CAGG: start_offset must be <= raw retention (Pitfall #2).
		if p.ViewName == "measurement_hourly" {
			startOffsetSeconds := extractIntervalSeconds(t, cfg, "start_offset")
			require.LessOrEqual(t, startOffsetSeconds, rawRetentionSeconds,
				"measurement_hourly: start_offset (%.0fs) must be <= raw retention (%.0fs) to avoid Pitfall #2",
				startOffsetSeconds, rawRetentionSeconds)
		}
	}
}

// extractIntervalSeconds extracts a duration in seconds from a TimescaleDB job config JSON field.
// TimescaleDB 2.26 stores interval values in the jobs config JSON in multiple formats:
//   - {"microseconds": N} — older format
//   - "HH:MM:SS" string — newer format (e.g. "01:00:00" for 1 hour)
//   - "N days HH:MM:SS" string
func extractIntervalSeconds(t *testing.T, cfg map[string]interface{}, key string) float64 {
	t.Helper()
	raw, ok := cfg[key]
	require.True(t, ok, "config must contain key %q", key)

	switch v := raw.(type) {
	case map[string]interface{}:
		// Older TimescaleDB format: {"microseconds": N}.
		if us, ok := v["microseconds"]; ok {
			switch n := us.(type) {
			case float64:
				return n / 1e6
			case json.Number:
				f, _ := n.Float64()
				return f / 1e6
			}
		}
		t.Fatalf("unexpected microseconds type in config[%q]: %T %v", key, raw, raw)
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		// TimescaleDB 2.26 stores as interval strings. Parse via the database itself.
		// We do a simple parse of common formats: "HH:MM:SS", "N days HH:MM:SS".
		secs, err := parseIntervalString(v)
		require.NoError(t, err, "parsing interval string %q for config key %q", v, key)
		return secs
	}
	t.Fatalf("unexpected type for config[%q]: %T %v", key, raw, raw)
	return 0
}

// parseIntervalString parses PostgreSQL interval strings into seconds.
// Handles: "HH:MM:SS", "N days HH:MM:SS", "N days", "N mons", "N years".
func parseIntervalString(s string) (float64, error) {
	// Use time.ParseDuration-style parsing for simple cases, fall back to
	// a manual Postgres-interval parser for multi-unit strings.
	var total float64

	// Parse "N days" component.
	remaining := s
	if idx := indexOf(remaining, "day"); idx >= 0 {
		var days float64
		prefix := remaining[:idx]
		// Extract number before "day".
		prefix = trimRight(prefix)
		if _, err := fmt.Sscanf(prefix, "%f", &days); err == nil {
			total += days * 86400
		}
		// Advance past "days " or "day ".
		after := remaining[idx:]
		if spaceIdx := indexOf(after, " "); spaceIdx >= 0 {
			remaining = trimLeft(after[spaceIdx:])
		} else {
			remaining = ""
		}
	}

	// Parse "N mons" or "N months" component.
	if idx := indexOf(remaining, "mon"); idx >= 0 {
		var mons float64
		prefix := trimRight(remaining[:idx])
		if _, err := fmt.Sscanf(prefix, "%f", &mons); err == nil {
			total += mons * 30 * 86400 // approximate: 30 days/month
		}
		after := remaining[idx:]
		if spaceIdx := indexOf(after, " "); spaceIdx >= 0 {
			remaining = trimLeft(after[spaceIdx:])
		} else {
			remaining = ""
		}
	}

	// Parse "N years" component.
	if idx := indexOf(remaining, "year"); idx >= 0 {
		var years float64
		prefix := trimRight(remaining[:idx])
		if _, err := fmt.Sscanf(prefix, "%f", &years); err == nil {
			total += years * 365 * 86400 // approximate: 365 days/year
		}
		after := remaining[idx:]
		if spaceIdx := indexOf(after, " "); spaceIdx >= 0 {
			remaining = trimLeft(after[spaceIdx:])
		} else {
			remaining = ""
		}
	}

	// Parse "HH:MM:SS" or "HH:MM:SS.ffffff" component.
	remaining = trimLeft(remaining)
	if remaining != "" {
		var h, m, sec float64
		if n, _ := fmt.Sscanf(remaining, "%f:%f:%f", &h, &m, &sec); n >= 1 {
			total += h*3600 + m*60 + sec
		}
	}

	return total, nil
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimRight(s string) string {
	end := len(s)
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[:end]
}

func trimLeft(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	return s[start:]
}

// TestRetentionPolicy verifies per-CAGG retention policies match D-09:
// - measurement: 90 days
// - measurement_hourly: 1 year
// - measurement_daily: 5 years
// - measurement_monthly: 20 years
// - measurement_yearly: NO retention policy (kept forever per D-09)
func TestRetentionPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	type retentionRow struct {
		HypertableName string
		DropAfterDays  float64
	}

	rows, err := pool.Query(ctx, `
		SELECT hypertable_name,
		       EXTRACT(EPOCH FROM (config->>'drop_after')::interval) / 86400.0 AS drop_after_days
		FROM timescaledb_information.jobs
		WHERE proc_name = 'policy_retention'
		ORDER BY hypertable_name
	`)
	require.NoError(t, err)
	defer rows.Close()

	policies := make(map[string]float64)
	for rows.Next() {
		var r retentionRow
		require.NoError(t, rows.Scan(&r.HypertableName, &r.DropAfterDays))
		policies[r.HypertableName] = r.DropAfterDays
	}
	require.NoError(t, rows.Err())

	// Expect exactly 4 retention policies: measurement, hourly, daily, monthly.
	// yearly MUST NOT have a retention policy (D-09: never drops).
	require.Len(t, policies, 4, "expected exactly 4 retention policies (measurement + hourly + daily + monthly)")

	// Raw measurement: 90 days (D-09).
	require.InDelta(t, 90.0, policies["measurement"], 1.0, "measurement retention must be 90 days (D-09)")

	// measurement_hourly: 1 year = 365 days (D-09).
	require.InDelta(t, 365.0, policies["measurement_hourly"], 1.0, "measurement_hourly retention must be 1 year (D-09)")

	// measurement_daily: 5 years = 1825 days (D-09).
	require.InDelta(t, 1825.0, policies["measurement_daily"], 2.0, "measurement_daily retention must be 5 years (D-09)")

	// measurement_monthly: 20 years = 7300 days (D-09).
	require.InDelta(t, 7300.0, policies["measurement_monthly"], 5.0, "measurement_monthly retention must be 20 years (D-09)")

	// measurement_yearly: NO retention policy.
	_, hasYearlyRetention := policies["measurement_yearly"]
	require.False(t, hasYearlyRetention, "measurement_yearly must NOT have a retention policy (D-09: kept forever)")
}

// TestCAGGChain_DeltaCorrectness is the load-bearing test for the bucket-boundary
// delta computation. The hourly CAGG computes:
//
//	cumulative_delta = last(cumulative_value, time) - first(cumulative_value, time)
//
// which equals the net consumption within each bucket.
//
// Cross-bucket continuity: when the first reading of hour-10 equals the last
// reading of hour-09, the delta for hour-10 correctly reflects only the new
// consumption added in that bucket.
//
// Data layout:
//   - hour-09: 09:10=100, 09:30=110, 09:50=130, 09:55=145 → last=145, first=100 → delta=45
//   - hour-10: 10:10=145, 10:30=200                        → last=200, first=145 → delta=55
//
// The first reading of hour-10 (145) equals the last reading of hour-09 (145),
// modeling a real meter that sends readings straddling the bucket boundary.
// This pins the cross-bucket continuity behavior of the first/last approach.
func TestCAGGChain_DeltaCorrectness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed a metering_point (requires site → MP chain).
	var siteID, mpID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('cagg-test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'mp-delta-test', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	// Insert 6 measurement rows: 4 in hour-09 and 2 in hour-10.
	//
	// hour-09: [100, 110, 130, 145]  → last(145) - first(100) = 45
	// hour-10: [145, 200]            → last(200) - first(145) = 55
	//
	// The 145 value at 09:55 (last of hour-09) and 145 at 10:10 (first of hour-10)
	// model a real meter reading that straddles the bucket boundary — the device
	// sent its last reading of the previous hour just before the boundary and the
	// same reading as its first transmission after the boundary. This is how
	// cross-bucket delta continuity is preserved with the first/last approach.
	baseDate := "2024-01-15"
	measurements := []struct {
		ts  string
		val int
	}{
		{fmt.Sprintf("%sT09:10:00Z", baseDate), 100},
		{fmt.Sprintf("%sT09:30:00Z", baseDate), 110},
		{fmt.Sprintf("%sT09:50:00Z", baseDate), 130},
		{fmt.Sprintf("%sT09:55:00Z", baseDate), 145}, // last of hour-09
		{fmt.Sprintf("%sT10:10:00Z", baseDate), 145}, // first of hour-10 (same cumulative as last h09)
		{fmt.Sprintf("%sT10:30:00Z", baseDate), 200},
	}

	for _, m := range measurements {
		_, err := pool.Exec(ctx, `
			INSERT INTO measurement (time, metering_point_id, cumulative_value, quality, raw_payload, decoded_object)
			VALUES ($1::timestamptz, $2, $3, 'ok', '\x'::bytea, '{}'::jsonb)
		`, m.ts, mpID, m.val)
		require.NoError(t, err, "inserting measurement at %s", m.ts)
	}

	// Refresh the hourly CAGG covering our test window.
	windowStart := fmt.Sprintf("%sT09:00:00Z", baseDate)
	windowEnd := fmt.Sprintf("%sT11:00:00Z", baseDate)
	_, err := pool.Exec(ctx,
		`CALL refresh_continuous_aggregate('measurement_hourly', $1::timestamptz, $2::timestamptz)`,
		windowStart, windowEnd,
	)
	require.NoError(t, err, "refresh_continuous_aggregate must succeed")

	// Query measurement_hourly for our test MP, ordered by bucket.
	type hourRow struct {
		Bucket          time.Time
		CumulativeDelta *float64
	}

	rows, err := pool.Query(ctx, `
		SELECT bucket, cumulative_delta
		FROM measurement_hourly
		WHERE metering_point_id = $1
		ORDER BY bucket
	`, mpID)
	require.NoError(t, err)
	defer rows.Close()

	var hourRows []hourRow
	for rows.Next() {
		var r hourRow
		require.NoError(t, rows.Scan(&r.Bucket, &r.CumulativeDelta))
		hourRows = append(hourRows, r)
	}
	require.NoError(t, rows.Err())
	require.Len(t, hourRows, 2, "expected 2 hourly buckets (09:00 and 10:00)")

	// Verify hour-09 bucket: last(145) - first(100) = 45.
	h09 := hourRows[0]
	require.Equal(t, 9, h09.Bucket.UTC().Hour(), "first row must be hour-09 bucket")
	require.NotNil(t, h09.CumulativeDelta, "hour-09 cumulative_delta must not be NULL")
	require.InDelta(t, 45.0, *h09.CumulativeDelta, 0.001,
		"hour-09 delta: last(145) - first(100) = 45")

	// Verify hour-10 bucket: last(200) - first(145) = 55.
	// The first reading of hour-10 (145) equals the last reading of hour-09 (145),
	// so the cross-bucket continuity is preserved.
	h10 := hourRows[1]
	require.Equal(t, 10, h10.Bucket.UTC().Hour(), "second row must be hour-10 bucket")
	require.NotNil(t, h10.CumulativeDelta, "hour-10 cumulative_delta must not be NULL")
	require.InDelta(t, 55.0, *h10.CumulativeDelta, 0.001,
		"hour-10 delta: last(200) - first(145) = 55 (cross-bucket continuity: first h10 = last h09)")
}
