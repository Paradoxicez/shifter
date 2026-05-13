// Package doctor — diagnostic bundle for operator support (Plan 06-11 / D-50).
//
// The `shifter doctor` CLI command calls Doctor.SnapshotBundle to collect a
// redacted JSON snapshot of the install's current state. The bundle is safe to
// email to support because all email addresses are masked (MaskEmail /
// RedactJSON) and no secrets (passwords, API tokens, session keys) are
// included.
//
// D-50 deferral: Docker log tail is NOT included in v1. The binary does not
// take a dependency on the Docker SDK. Operators are instructed to attach
// `docker compose logs --tail=200 shifter` output manually.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/version"
)

// Bundle is the top-level diagnostic snapshot. Every field that may contain
// PII is masked by MarshalRedacted before the bundle is written to stdout or
// file.
type Bundle struct {
	GeneratedAt        time.Time            `json:"generated_at"`
	Shifter            ShifterInfo          `json:"shifter"`
	ConfigCheck        any                  `json:"config_check"`
	HealthDetailed     any                  `json:"health_detailed"`
	AlertWorkers       []AlertWorkerRow     `json:"alert_workers"`
	LastBackup         *LastBackupRow       `json:"last_backup"`
	ChirpstackGRPCPing GRPCPingResult       `json:"chirpstack_grpc_ping"`
	RecentAudit        []AuditRow           `json:"recent_audit"`
	Logs               []string             `json:"logs"`
	ProbeResults       map[string]ProbeResult `json:"probe_results,omitempty"`
}

// ShifterInfo contains build and runtime metadata.
type ShifterInfo struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	UptimeSeconds int    `json:"uptime_seconds"`
}

// AlertWorkerRow mirrors one alert_worker_state row for the bundle.
type AlertWorkerRow struct {
	Kind           string    `json:"kind"`
	LastRunAt      time.Time `json:"last_run_at"`
	RulesEvaluated int       `json:"rules_evaluated"`
	FiresEmitted   int       `json:"fires_emitted"`
	Cleared        int       `json:"cleared"`
	DurationMs     int       `json:"duration_ms"`
	Degraded       bool      `json:"degraded"`
	LastError      string    `json:"last_error,omitempty"`
}

// LastBackupRow mirrors the most-recent backup_run row for the bundle.
type LastBackupRow struct {
	FileName   string    `json:"file_name"`
	StartedAt  time.Time `json:"started_at"`
	AgeSeconds int64     `json:"age_seconds"`
	Status     string    `json:"status"`
	SHA256     string    `json:"sha256"`
}

// GRPCPingResult records whether the ChirpStack gRPC endpoint was reachable.
type GRPCPingResult struct {
	Reachable bool   `json:"reachable"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

// AuditRow is a condensed audit_log row suitable for the bundle.
// The UserEmail field will be masked by RedactJSON.
type AuditRow struct {
	ID         string `json:"id"`
	Time       string `json:"time"`
	Action     string `json:"action"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	UserEmail  string `json:"user_email,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// Doctor assembles the diagnostic bundle from live DB queries and config
// probes. It is constructed by the CLI command and calls SnapshotBundle once.
//
// NOTE: Doctor does NOT depend on the HTTP server being running. It calls the
// same underlying probe functions (DB pool, ChirpStack gRPC dial) directly.
type Doctor struct {
	Pool      *pgxpool.Pool
	CSDialer  func(ctx context.Context) GRPCPingResult
	StartedAt time.Time // process start time for uptime calculation
}

// SnapshotBundle collects all diagnostic data and returns the bundle. The
// returned bundle has NOT been redacted yet — call MarshalRedacted on the
// result before writing to stdout or file.
func (d *Doctor) SnapshotBundle(ctx context.Context) (*Bundle, error) {
	b := &Bundle{
		GeneratedAt: time.Now().UTC(),
	}

	// Shifter info.
	b.Shifter = d.gatherShifterInfo(ctx)

	// Config check — summarise what's reachable.
	b.ConfigCheck = d.gatherConfigCheck(ctx)

	// Health detailed — same query shape as /health/detailed handler.
	b.HealthDetailed = d.gatherHealthDetailed(ctx)

	// Alert workers — loaded separately for convenience in the bundle shape.
	b.AlertWorkers = d.loadAlertWorkers(ctx)

	// Last backup.
	b.LastBackup = d.loadLastBackup(ctx)

	// ChirpStack gRPC ping.
	if d.CSDialer != nil {
		b.ChirpstackGRPCPing = d.CSDialer(ctx)
	} else {
		b.ChirpstackGRPCPing = GRPCPingResult{Reachable: false, Error: "no CS dialer configured"}
	}

	// Recent audit rows (last 100, newest first).
	b.RecentAudit = d.loadRecentAudit(ctx, 100)

	// Logs: Docker socket deferred per D-50 / RESEARCH Open Question #3.
	b.Logs = loadLogs()

	return b, nil
}

// MarshalRedacted serializes the bundle as indented JSON and runs the email
// regex pass over the output bytes so all email-shaped tokens are masked.
func (b *Bundle) MarshalRedacted() ([]byte, error) {
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("doctor: marshal bundle: %w", err)
	}
	return RedactJSON(raw), nil
}

// gatherShifterInfo queries schema_migrations for the current schema version
// and builds the ShifterInfo struct.
func (d *Doctor) gatherShifterInfo(ctx context.Context) ShifterInfo {
	info := version.Info()

	var schemaVersion int
	_ = d.Pool.QueryRow(ctx,
		`SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`,
	).Scan(&schemaVersion)

	uptimeSecs := 0
	if !d.StartedAt.IsZero() {
		uptimeSecs = int(time.Since(d.StartedAt).Seconds())
	}

	return ShifterInfo{
		Version:       info.Version,
		SchemaVersion: schemaVersion,
		UptimeSeconds: uptimeSecs,
	}
}

// gatherConfigCheck returns a summary of config reachability without leaking
// secret values. Returns a map with a "db" boolean key.
func (d *Doctor) gatherConfigCheck(ctx context.Context) map[string]any {
	return map[string]any{
		"db": d.Pool.Ping(ctx) == nil,
	}
}

// gatherHealthDetailed returns the same structure as /health/detailed by
// querying directly (no HTTP server dependency).
func (d *Doctor) gatherHealthDetailed(ctx context.Context) map[string]any {
	dbOK := d.Pool.Ping(ctx) == nil
	status := "ok"
	if !dbOK {
		status = "degraded"
	}

	workers := d.loadAlertWorkers(ctx)
	for _, aw := range workers {
		if aw.Degraded {
			status = "degraded"
		}
	}

	lb := d.loadLastBackup(ctx)
	if lb != nil {
		var critHours int
		_ = d.Pool.QueryRow(ctx,
			`SELECT COALESCE(backup_crit_threshold_hours, 0) FROM retention_config WHERE id = 1`,
		).Scan(&critHours)
		if critHours > 0 && lb.AgeSeconds > int64(critHours)*3600 {
			status = "degraded"
		}
	}

	return map[string]any{
		"status":        status,
		"checks":        map[string]any{"db": dbOK},
		"alert_workers": workers,
		"last_backup":   lb,
	}
}

// loadAlertWorkers returns the alert_worker_state rows for the bundle.
func (d *Doctor) loadAlertWorkers(ctx context.Context) []AlertWorkerRow {
	rows, err := d.Pool.Query(ctx, `
		SELECT worker_kind, last_run_at, rules_evaluated, fires_emitted,
		       cleared, duration_ms, degraded, COALESCE(last_error, '')
		FROM alert_worker_state
		ORDER BY worker_kind`)
	if err != nil {
		return []AlertWorkerRow{}
	}
	defer rows.Close()

	var out []AlertWorkerRow
	for rows.Next() {
		var r AlertWorkerRow
		if err := rows.Scan(&r.Kind, &r.LastRunAt, &r.RulesEvaluated, &r.FiresEmitted,
			&r.Cleared, &r.DurationMs, &r.Degraded, &r.LastError); err != nil {
			continue
		}
		out = append(out, r)
	}
	if out == nil {
		out = []AlertWorkerRow{}
	}
	return out
}

// loadLastBackup returns the most-recent backup_run row, or nil if no backups.
func (d *Doctor) loadLastBackup(ctx context.Context) *LastBackupRow {
	var r LastBackupRow
	err := d.Pool.QueryRow(ctx, `
		SELECT COALESCE(file_name,''), started_at,
		       EXTRACT(EPOCH FROM (now()-started_at))::BIGINT,
		       status, COALESCE(sha256,'')
		FROM backup_run
		ORDER BY started_at DESC LIMIT 1`,
	).Scan(&r.FileName, &r.StartedAt, &r.AgeSeconds, &r.Status, &r.SHA256)
	if err != nil {
		return nil
	}
	return &r
}

// loadRecentAudit returns the most recent limit audit rows (newest first).
// The UserEmail field will be masked by RedactJSON at marshal time.
func (d *Doctor) loadRecentAudit(ctx context.Context, limit int) []AuditRow {
	rows, err := d.Pool.Query(ctx, `
		SELECT al.id::text, al.time::text, al.action, al.entity_type,
		       al.entity_id::text,
		       COALESCE(u.email, ''),
		       COALESCE(al.notes, '')
		FROM audit_log al
		LEFT JOIN "user" u ON u.id = al.user_id
		ORDER BY al.time DESC, al.id DESC
		LIMIT $1`, limit)
	if err != nil {
		return []AuditRow{}
	}
	defer rows.Close()

	var out []AuditRow
	for rows.Next() {
		var r AuditRow
		if err := rows.Scan(&r.ID, &r.Time, &r.Action, &r.EntityType,
			&r.EntityID, &r.UserEmail, &r.Notes); err != nil {
			continue
		}
		out = append(out, r)
	}
	if out == nil {
		out = []AuditRow{}
	}
	return out
}

// loadLogs returns the static fallback note. Docker socket log tail is
// deferred to v1.x per RESEARCH Open Question #3 / D-50. The bundle's logs
// field documents this for operators who attach the bundle to a support ticket.
func loadLogs() []string {
	return []string{
		"<docker socket not mounted — run `docker compose logs --tail=200 shifter` from host>",
	}
}
