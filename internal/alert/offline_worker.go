package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/shifter-io/shifter/internal/audit"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// OfflineArgs is the River job-args type for ALERT-02 + ALERT-03 (the
// offline_device + offline_gateway rule kinds share one worker because
// gateway-down suppression requires evaluating both in the same cycle —
// D-14).
//
// RESEARCH §Decision C cadence: 2 minutes.
type OfflineArgs struct{}

// Kind returns the unique River job kind. Must match the
// alertWorkerKindFromJobKind switch in degraded.go.
func (OfflineArgs) Kind() string { return "alert_offline" }

// InsertOpts caps retries at 3 (D-22) so a permanently-broken worker hits
// JobStateDiscarded and trips the degraded flag.
func (OfflineArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// OfflineWorker evaluates offline_device + offline_gateway rules with
// D-14 gateway-down suppression and D-15 hysteresis.
//
// One cycle:
//  1. Load active offline_device + offline_gateway rules. If none, skip.
//  2. One SQL query returns offline candidates plus per-row gateway_offline.
//  3. For each candidate:
//       - if gateway_offline=TRUE: suppress device alert, add to the
//         per-gateway count.
//       - else: fire offline_device alert (idempotent + cooldown-aware).
//  4. For each downed gateway: fire ONE offline_gateway alert carrying
//     the suppressed-device count in the payload.
//  5. Hysteresis clears: any firing offline_device alert whose
//     last_seen_at is back inside 2× expected_interval clears.
type OfflineWorker struct {
	river.WorkerDefaults[OfflineArgs]

	Eng        EvaluateContext
	Rules      *RuleStore
	Alerts     *AlertStore
	WorkerStat *WorkerStateStore
}

// Work runs one offline-evaluation cycle.
func (w *OfflineWorker) Work(ctx context.Context, _ *river.Job[OfflineArgs]) error {
	start := time.Now()
	state := &RunState{}

	defer func() {
		state.DurationMS = int32(time.Since(start).Milliseconds())
		if w.WorkerStat == nil {
			return
		}
		tx, err := w.Eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			return
		}
		_ = w.WorkerStat.UpsertWorkerState(ctx, tx, "offline", state)
		_ = tx.Commit(ctx)
	}()

	deviceRules, err := w.Rules.ListActiveRulesByKind(ctx, "offline_device")
	if err != nil {
		state.RecordErr(err)
		return err
	}
	gatewayRules, err := w.Rules.ListActiveRulesByKind(ctx, "offline_gateway")
	if err != nil {
		state.RecordErr(err)
		return err
	}
	if len(deviceRules) == 0 && len(gatewayRules) == 0 {
		return nil
	}

	candidates, err := w.Eng.Queries.ListOfflineDevicesWithGatewayStatus(ctx)
	if err != nil {
		state.RecordErr(err)
		return err
	}

	installName := readInstallDisplayName(ctx, w.Eng)
	now := time.Now().UTC()

	// D-14 suppression bookkeeping — for each downed gateway, the list of
	// candidates behind it. The slice is used to populate
	// suppresses_n_devices on the single gateway-offline alert.
	suppressed := map[uuid.UUID][]sqlc.ListOfflineDevicesWithGatewayStatusRow{}

	for _, c := range candidates {
		state.RulesEvaluated++

		if c.GatewayOffline != nil && *c.GatewayOffline && c.GatewayID.Valid {
			// D-14: suppress device alert; tally for gateway-down alert.
			gwID := uuid.UUID(c.GatewayID.Bytes)
			suppressed[gwID] = append(suppressed[gwID], c)
			continue
		}

		for _, rule := range deviceRules {
			if !scopeMatchesDevice(rule, c) {
				continue
			}
			if !rule.CooledDown(now) {
				continue
			}
			deviceID := uuid.UUID(c.DeviceID.Bytes)
			existing, _ := w.Alerts.ListFiringByRuleTarget(ctx, rule.ID, deviceID)
			if existing != nil {
				continue // already firing — idempotent
			}
			if err := w.fireDeviceOffline(ctx, rule, c, installName); err != nil {
				if errors.Is(err, ErrDuplicateFire) {
					continue
				}
				state.RecordErr(err)
				continue
			}
			state.FiresEmitted++
		}
	}

	// Emit one offline_gateway alert per downed gateway.
	for gwID, sup := range suppressed {
		for _, rule := range gatewayRules {
			if !scopeMatchesGateway(rule, gwID) {
				continue
			}
			if !rule.CooledDown(now) {
				continue
			}
			existing, _ := w.Alerts.ListFiringByRuleTarget(ctx, rule.ID, gwID)
			if existing != nil {
				continue // already firing — first-emitted count persists
			}
			gwLabel := ""
			if len(sup) > 0 && sup[0].GatewayLabel != nil {
				gwLabel = *sup[0].GatewayLabel
			}
			if err := w.fireGatewayOffline(ctx, rule, gwID, gwLabel, int32(len(sup)), installName); err != nil {
				if errors.Is(err, ErrDuplicateFire) {
					continue
				}
				state.RecordErr(err)
				continue
			}
			state.FiresEmitted++
		}
	}

	// Hysteresis clears (D-15: now() - last_seen_at < 2× expected_interval_s).
	clears, err := w.Eng.Queries.ListHysteresisClearOffline(ctx)
	if err != nil {
		state.RecordErr(err)
		return nil // partial success — we already fired what we could
	}
	for _, c := range clears {
		existing, _ := w.Alerts.ListFiringByRuleTarget(ctx,
			uuid.UUID(c.RuleID.Bytes), uuid.UUID(c.DeviceID.Bytes))
		if existing == nil {
			continue
		}
		if err := autoClearAlert(ctx, w.Eng, w.Alerts, *existing); err != nil {
			state.RecordErr(err)
			continue
		}
		state.Cleared++
	}

	return nil
}

// scopeMatchesDevice returns true when the rule's scope is broad enough to
// cover this offline candidate. offline_device rules typically apply
// globally; per-device or per-site scopes are also allowed.
func scopeMatchesDevice(rule RuleRecord, c sqlc.ListOfflineDevicesWithGatewayStatusRow) bool {
	switch rule.ScopeKind {
	case "global":
		return true
	case "device":
		if rule.ScopeID == nil {
			return false
		}
		return *rule.ScopeID == uuid.UUID(c.DeviceID.Bytes)
	case "metering_point":
		// offline_device rules can be scoped to an MP — they fire when
		// the device currently bound to that MP is offline. We don't
		// resolve the binding here; the SQL would need a JOIN through
		// binding. v1 silently no-ops MP-scoped offline rules.
		return false
	case "site":
		// Site-scoped offline rules: would need a JOIN through binding
		// → metering_point → site. v1 silently no-ops; operators get
		// the same coverage via global offline rules.
		return false
	case "gateway":
		// Gateway scope on offline_device makes no semantic sense (the
		// rule is "is this DEVICE offline?"); operator should use
		// offline_gateway instead.
		return false
	default:
		return false
	}
}

// scopeMatchesGateway returns true when the rule's scope covers this
// downed gateway.
func scopeMatchesGateway(rule RuleRecord, gatewayID uuid.UUID) bool {
	switch rule.ScopeKind {
	case "global":
		return true
	case "gateway":
		if rule.ScopeID == nil {
			return false
		}
		return *rule.ScopeID == gatewayID
	default:
		return false
	}
}

// fireDeviceOffline opens a tx, inserts the offline_device alert + audit
// row + TouchLastFiredAt in the same tx (D-23).
func (w *OfflineWorker) fireDeviceOffline(ctx context.Context, rule RuleRecord,
	c sqlc.ListOfflineDevicesWithGatewayStatusRow, installName string) error {
	tx, err := w.Eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	deviceID := uuid.UUID(c.DeviceID.Bytes)
	last := c.LastUplinkAt.Time
	seconds := int32(time.Since(last).Seconds())
	interval := c.ExpectedIntervalS
	payload, err := BuildPayload(BuildPayloadInput{
		RuleID:            rule.ID,
		RuleKind:          "offline_device",
		Severity:          rule.Severity,
		Target:            PayloadTarget{EntityType: "device", EntityID: deviceID, Label: c.DeviceLabel},
		Value:             float64(seconds),
		Threshold:         float64(3 * interval),
		Comparison:        "gt",
		Unit:              "seconds_since_last_uplink",
		FiredAt:           time.Now().UTC(),
		InstallName:       installName,
		LastUplinkAt:      &last,
		ExpectedIntervalS: &interval,
	})
	if err != nil {
		return fmt.Errorf("alert: build offline_device payload: %w", err)
	}

	row, err := w.Alerts.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           rule.ID,
		RuleKind:         "offline_device",
		Severity:         rule.Severity,
		Payload:          payload,
		TargetEntityType: "device",
		TargetEntityID:   deviceID,
	})
	if err != nil {
		return err
	}
	afterMap, _ := jsonbToMap(payload)
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		Action:     audit.ActionAlertFired,
		EntityType: audit.EntityTypeAlert,
		EntityID:   row.ID,
		After:      afterMap,
	}); err != nil {
		return fmt.Errorf("alert: audit offline_device fire: %w", err)
	}
	if err := w.Rules.TouchLastFiredAt(ctx, tx, rule.ID); err != nil {
		return fmt.Errorf("alert: touch offline_device rule: %w", err)
	}
	return tx.Commit(ctx)
}

// fireGatewayOffline opens a tx, inserts the offline_gateway alert with
// the suppresses_n_devices count in the payload (D-14).
func (w *OfflineWorker) fireGatewayOffline(ctx context.Context, rule RuleRecord,
	gatewayID uuid.UUID, gatewayLabel string, suppressedN int32, installName string) error {
	tx, err := w.Eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	payload, err := BuildPayload(BuildPayloadInput{
		RuleID:             rule.ID,
		RuleKind:           "offline_gateway",
		Severity:           rule.Severity,
		Target:             PayloadTarget{EntityType: "gateway", EntityID: gatewayID, Label: gatewayLabel},
		Value:              float64(suppressedN),
		Threshold:          0,
		Comparison:         "gt",
		Unit:               "devices",
		FiredAt:            time.Now().UTC(),
		InstallName:        installName,
		SuppressesNDevices: &suppressedN,
	})
	if err != nil {
		return fmt.Errorf("alert: build offline_gateway payload: %w", err)
	}

	row, err := w.Alerts.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           rule.ID,
		RuleKind:         "offline_gateway",
		Severity:         rule.Severity,
		Payload:          payload,
		TargetEntityType: "gateway",
		TargetEntityID:   gatewayID,
	})
	if err != nil {
		return err
	}
	afterMap, _ := jsonbToMap(payload)
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		Action:     audit.ActionAlertFired,
		EntityType: audit.EntityTypeAlert,
		EntityID:   row.ID,
		After:      afterMap,
	}); err != nil {
		return fmt.Errorf("alert: audit offline_gateway fire: %w", err)
	}
	if err := w.Rules.TouchLastFiredAt(ctx, tx, rule.ID); err != nil {
		return fmt.Errorf("alert: touch offline_gateway rule: %w", err)
	}
	return tx.Commit(ctx)
}
