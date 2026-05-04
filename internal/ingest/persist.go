package ingest

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shifter-io/shifter/internal/audit"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/profile"
	"github.com/shifter-io/shifter/internal/resolver"
	"github.com/shifter-io/shifter/internal/swap"
)

// PersistInput is the value-shape PersistAtomically consumes. Built by the
// UplinkHandler closure after decode + resolve + normalize succeeds (or
// gracefully degrades to a non-OK quality flag).
type PersistInput struct {
	IngestTime time.Time
	Event      Event
	Binding    resolver.Binding
	Mappings   []profile.Mapping
	Normalized Layer1
	RawPayload []byte
	Quality    string
}

// PersistAtomically writes one measurement row + advances binding.last_raw_value
// + (if rollover detected) bumps binding.reading_offset and audits the event +
// updates device.last_seen, all inside a single pgx.Serializable transaction.
//
// Order of operations in the txn:
//
//  1. Rollover detection — only when both Normalized.RawValue AND
//     Binding.LastRawValue are non-nil. The first uplink for a freshly-opened
//     binding has Binding.LastRawValue == nil (D-05 boundary signal); rollover
//     detection is skipped to avoid a false positive on the new meter starting
//     fresh after a swap (T-02-09-06 mitigation).
//  2. Cumulative computation: cumulative_value = raw + currentOffset (where
//     currentOffset reflects rollover application from step 1).
//     If the profile's mapping explicitly populated CumulativeValue, that
//     value is preserved (codec-emitted cumulative wins over the computed
//     one — rare path, profile editor allowed).
//  3. AppendMeasurement (the only place ingest writes to measurement) with
//     all 20 column parameters populated. raw_payload + decoded_object are
//     ALWAYS persisted, including on quality != 'ok' rows (DATA-07 + D-26).
//  4. UpdateBindingLastRaw if RawValue non-nil — feeds the next uplink's
//     rollover detection without a JOIN to the hypertable.
//  5. UpdateDeviceLastSeen — Phase 4 "device offline" alerts depend on this.
//  6. Rollover audit row (if step 1 fired) via audit.WriteEntry inside the
//     same tx (D-23 + AUDIT-01).
//  7. tx.Commit — atomic across all six writes.
//  8. POST-COMMIT: defensively prime the resolver cache with the new
//     LastRawValue + ReadingOffset so the next uplink's rollover math has
//     the freshest binding state without a re-load. The 0017 NOTIFY trigger
//     does NOT fire on UpdateBindingLastRaw / AdvanceReadingOffset (Plan 02-07
//     decision), so without this Set the resolver would carry stale per-binding
//     state — every uplink would show "rollover" forever after one fires.
//
// Per D-26 + Pitfall 5: NEVER silent-drop. Even if Quality != 'ok' the row
// is inserted (raw_payload + decoded_object preserved). The dashboard
// surfaces these via the "X uplinks flagged" badge.
func PersistAtomically(ctx context.Context, deps Deps, in PersistInput) error {
	if deps.Pool == nil {
		return fmt.Errorf("ingest: nil pool")
	}

	tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("ingest: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlc.New(tx)

	// 1. Rollover detection.
	currentOffset := in.Binding.ReadingOffset
	if currentOffset == nil {
		currentOffset = big.NewFloat(0).SetPrec(normalizePrecision)
	}
	rolloverDetected := false
	if in.Normalized.RawValue != nil && in.Binding.LastRawValue != nil && in.Binding.CounterModulus > 0 {
		if swap.DetectRollover(in.Binding.LastRawValue, in.Normalized.RawValue) {
			currentOffset = swap.ApplyRollover(currentOffset, in.Binding.CounterModulus)
			rolloverDetected = true
			offsetDelta := big.NewFloat(0).SetPrec(normalizePrecision).SetInt64(in.Binding.CounterModulus)
			deltaNumeric, err := numericFromBigFloat(offsetDelta)
			if err != nil {
				return fmt.Errorf("ingest: encode rollover delta: %w", err)
			}
			if err := q.AdvanceReadingOffset(ctx, sqlc.AdvanceReadingOffsetParams{
				ID:            pgtype.UUID{Bytes: in.Binding.BindingID, Valid: true},
				ReadingOffset: deltaNumeric,
			}); err != nil {
				return fmt.Errorf("ingest: advance offset: %w", err)
			}
		}
	}

	// 2. Compute cumulative_value (codec-emitted CumulativeValue overrides).
	cumulative := in.Normalized.CumulativeValue
	if cumulative == nil && in.Normalized.RawValue != nil {
		cumulative = new(big.Float).SetPrec(normalizePrecision).Add(in.Normalized.RawValue, currentOffset)
	}

	// 3. AppendMeasurement — DATA-01 (mp-keyed) + DATA-03 (server time) +
	//    DATA-07 (raw + decoded preserved) + DATA-08 (wide+JSONB).
	decodedJSON := jsonOrEmptyObject(in.Event.DecodedObject)
	extraJSON := jsonOrEmptyObject(in.Normalized.Extra)

	fcnt := int32(in.Event.FCnt)
	params := sqlc.AppendMeasurementParams{
		Time:            pgtype.Timestamptz{Time: in.IngestTime, Valid: true},
		MeteringPointID: pgtype.UUID{Bytes: in.Binding.MeteringPointID, Valid: true},
		RawValue:        numericFromBigFloatNullable(in.Normalized.RawValue),
		CumulativeValue: numericFromBigFloatNullable(cumulative),
		InstantValue:    numericFromBigFloatNullable(in.Normalized.InstantValue),
		BatteryPct:      pgInt2Nullable(in.Normalized.BatteryPct),
		Rssi:            pgInt2Nullable(in.Normalized.RSSI),
		Snr:             pgFloat4Nullable(in.Normalized.SNR),
		TemperatureC:    pgFloat4Nullable(in.Normalized.TemperatureC),
		PressureKpa:     pgFloat4Nullable(in.Normalized.PressureKPa),
		LeakDetected:    pgBoolNullable(in.Normalized.LeakDetected),
		TamperDetected:  pgBoolNullable(in.Normalized.TamperDetected),
		Extra:           extraJSON,
		RawPayload:      in.RawPayload,
		DecodedObject:   decodedJSON,
		Quality:         in.Quality,
		Fcnt:            &fcnt,
		GatewayRxTime:   pgTimestamptzNullable(in.Event.GatewayRxTime),
		DeviceTime:      pgTimestamptzNullable(in.Event.DeviceTime),
		BindingID:       pgtype.UUID{Bytes: in.Binding.BindingID, Valid: true},
	}
	if err := q.AppendMeasurement(ctx, params); err != nil {
		return fmt.Errorf("ingest: append measurement: %w", err)
	}

	// 4. UpdateBindingLastRaw (only when we got an actual raw value).
	if in.Normalized.RawValue != nil {
		lastRaw, err := numericFromBigFloat(in.Normalized.RawValue)
		if err != nil {
			return fmt.Errorf("ingest: encode last_raw: %w", err)
		}
		if err := q.UpdateBindingLastRaw(ctx, sqlc.UpdateBindingLastRawParams{
			ID:           pgtype.UUID{Bytes: in.Binding.BindingID, Valid: true},
			LastRawValue: lastRaw,
		}); err != nil {
			return fmt.Errorf("ingest: update last_raw: %w", err)
		}
	}

	// 5. UpdateDeviceLastSeen.
	if err := q.UpdateDeviceLastSeen(ctx, sqlc.UpdateDeviceLastSeenParams{
		ID:         pgtype.UUID{Bytes: in.Binding.DeviceID, Valid: true},
		LastSeenAt: pgtype.Timestamptz{Time: in.IngestTime, Valid: true},
	}); err != nil {
		return fmt.Errorf("ingest: update last_seen: %w", err)
	}

	// 6. Rollover audit (D-05) inside the SAME tx (D-23).
	if rolloverDetected {
		prevRawText := ""
		if in.Binding.LastRawValue != nil {
			prevRawText = in.Binding.LastRawValue.Text('g', 32)
		}
		prevOffsetText := ""
		if in.Binding.ReadingOffset != nil {
			prevOffsetText = in.Binding.ReadingOffset.Text('g', 32)
		}
		currOffsetText := currentOffset.Text('g', 32)
		currRawText := in.Normalized.RawValue.Text('g', 32)

		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.Nil, // system-emitted
			Action:     audit.ActionRolloverDetected,
			EntityType: audit.EntityTypeBinding,
			EntityID:   in.Binding.BindingID,
			Before: map[string]any{
				"reading_offset":  prevOffsetText,
				"last_raw_value":  prevRawText,
				"counter_modulus": in.Binding.CounterModulus,
			},
			After: map[string]any{
				"reading_offset":    currOffsetText,
				"current_raw_value": currRawText,
				"counter_modulus":   in.Binding.CounterModulus,
				"dev_eui":           in.Event.DevEUI,
			},
		}); err != nil {
			return fmt.Errorf("ingest: write rollover audit: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ingest: commit: %w", err)
	}

	// 8. Defensive resolver cache update — keep next uplink's rollover math
	//    fresh without a re-load. The 0017 NOTIFY trigger does NOT fire on
	//    last_raw / offset updates (Plan 02-07 decision), so this Set is the
	//    only mechanism that propagates the post-uplink state.
	if deps.Resolver != nil {
		updated := in.Binding
		if in.Normalized.RawValue != nil {
			updated.LastRawValue = new(big.Float).SetPrec(normalizePrecision).Set(in.Normalized.RawValue)
		}
		updated.ReadingOffset = new(big.Float).SetPrec(normalizePrecision).Set(currentOffset)
		deps.Resolver.Set(in.Event.DevEUI, updated)
	}

	return nil
}
