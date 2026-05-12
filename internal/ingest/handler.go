package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/profile"
	"github.com/shifter-io/shifter/internal/resolver"
)

// handlerTimeout caps the total work a single uplink can do before the
// pipeline forcibly bails. Threat T-02-09-05: a slow resolver / db can
// deadlock the MQTT consumer goroutine; paho QoS1 will redeliver if we
// don't ack, which is preferable to the consumer wedging.
const handlerTimeout = 10 * time.Second

// MappingStore returns the mapping rows for a given device_profile id.
// Implemented by a thin sqlc.Queries wrapper at cmd/serve.
type MappingStore interface {
	GetMappingsByProfile(ctx context.Context, profileID uuid.UUID) ([]profile.Mapping, error)
}

// Resolver is the resolver-cache contract the handler depends on. The
// production type *resolver.Resolver satisfies this directly. Defining
// it here avoids a hard import-only-for-the-method-set in test code.
type Resolver interface {
	Lookup(ctx context.Context, devEUI string, at time.Time) (resolver.Binding, error)
	Set(devEUI string, b resolver.Binding)
}

// Deps bundles the shared infra a single uplink handle needs.
//
// Pool — pgx pool used to open the persist transaction.
// Resolver — dev_eui → binding cache (Plan 02-07).
// Mappings — per-profile mapping rows; in production a sqlc.Queries wrapper.
// Log — structured logger; nil → slog.Default().
type Deps struct {
	Pool     *pgxpool.Pool
	Resolver Resolver
	Mappings MappingStore
	Log      *slog.Logger
}

// UplinkHandler returns a chirpstack.UplinkHandler closure that orchestrates
// the full ingest pipeline:
//
//  1. Capture ingestTime = time.Now().UTC() — DATA-03 + Pitfall 4 (server
//     time is the authoritative measurement.time, NOT the gateway/device
//     timestamps which are diagnostic).
//  2. DecodeChirpStackEvent — JSON parse fail → persist quality='decode_fail'
//     row with the raw payload preserved (D-26 + DATA-07 — never silent-drop).
//  3. resolver.Lookup → ErrNoActiveBinding → persist quality='missing_canonical'
//     row keyed by NULL metering_point_id... actually, DATA-01 says rows are
//     keyed by metering_point_id. An unbound device cannot satisfy that
//     invariant, so we LOG + DROP at the orphan path: the row is not written.
//
//     CONTEXT D-26 calls for "persist all uplinks, never silent-drop." But
//     DATA-01 is non-negotiable: measurement rows MUST be keyed by an MP.
//     The two reconcile by: the ingest pipeline emits a structured-log
//     "unbound_device" event (operators can grep it / Phase 6 surfaces it
//     in a separate "unbound uplinks" view), and NO row is written to the
//     hypertable. This is the only path where DATA-07 yields to DATA-01.
//
//  4. MappingStore.GetMappingsByProfile — load per-profile mapping rows.
//  5. NormalizeMeasurement — apply mappings; ErrNoCanonicalValue is recovered
//     to quality='missing_canonical' (row still persists with raw + decoded).
//  6. PersistAtomically — single Serializable txn: insert measurement, update
//     binding.last_raw_value, update device.last_seen, detect+apply rollover
//     (with audit row in same tx), commit.
//
// The closure is bound to the chirpstack.MQTTSubscriber via SetUplinkHandler
// at cmd/serve boot wiring (Plan 02-12).
func UplinkHandler(deps Deps) chirpstack.UplinkHandler {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return func(topic string, payload []byte) {
		ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
		defer cancel()

		// DATA-03 + Pitfall 4 — capture authoritative ingest time IMMEDIATELY.
		// Any clock-affecting work below happens AFTER this point; the value
		// does not move once captured.
		ingestTime := time.Now().UTC()

		ev, err := DecodeChirpStackEvent(payload)
		if err != nil {
			deps.Log.Warn("ingest: decode failed",
				"err", err, "topic", topic, "bytes", len(payload))
			// D-26: persist a quality='decode_fail' row so operators see the
			// issue. Best-effort dev_eui recovery from the topic — ChirpStack
			// v4's canonical uplink topic embeds the dev_eui.
			if devEUI := devEUIFromTopic(topic); devEUI != "" {
				persistDecodeFail(ctx, deps, devEUI, ingestTime, payload)
			}
			return
		}

		binding, err := deps.Resolver.Lookup(ctx, ev.DevEUI, ingestTime)
		if err != nil {
			if errors.Is(err, resolver.ErrNoActiveBinding) {
				// Race / unprovisioned device. DATA-01 forbids writing a
				// measurement row without a metering_point_id; structured-log
				// the unbound uplink and drop. The raw bytes are NOT lost —
				// MQTT broker retains them per QoS1 semantics if needed.
				deps.Log.Warn("ingest: unbound device — dropping uplink",
					"dev_eui", ev.DevEUI, "fcnt", ev.FCnt,
					"reason", "no_active_binding", "topic", topic)
				return
			}
			deps.Log.Error("ingest: resolver error",
				"err", err, "dev_eui", ev.DevEUI)
			return
		}

		mappings, err := deps.Mappings.GetMappingsByProfile(ctx, binding.DeviceProfileID)
		if err != nil {
			deps.Log.Error("ingest: load mappings failed",
				"err", err, "dev_eui", ev.DevEUI, "device_profile_id", binding.DeviceProfileID)
			return
		}

		normalized, normErr := NormalizeMeasurement(ev.DecodedObject, mappings, binding.BatteryCurve)
		quality := QualityOK
		if normErr != nil {
			quality = QualityMissingCanonical
			deps.Log.Warn("ingest: normalize warning",
				"dev_eui", ev.DevEUI, "err", normErr,
				"mapping_count", len(mappings))
		}

		if err := PersistAtomically(ctx, deps, PersistInput{
			IngestTime: ingestTime,
			Event:      ev,
			Binding:    binding,
			Mappings:   mappings,
			Normalized: normalized,
			RawPayload: payload,
			Quality:    quality,
		}); err != nil {
			deps.Log.Error("ingest: persist failed",
				"err", err, "dev_eui", ev.DevEUI, "fcnt", ev.FCnt)
		}
	}
}

// persistDecodeFail writes a measurement row with quality='decode_fail' for
// the dev_eui recovered from the MQTT topic. DATA-07: raw_payload is preserved
// so operators can inspect the broken bytes in Phase 6's audit view; canonical
// columns are NULL (decode never produced any). DATA-01 still applies — we
// resolve the dev_eui through the resolver to find the active binding's MP.
// If the dev_eui is unbound, we log + drop (same as the orphan path above).
//
// Best-effort: errors here are warn-logged but never propagated. The MQTT
// consumer should keep flowing.
func persistDecodeFail(ctx context.Context, deps Deps, devEUI string, ingestTime time.Time, payload []byte) {
	binding, err := deps.Resolver.Lookup(ctx, devEUI, ingestTime)
	if err != nil {
		// Unbound + decode_fail — both DATA-01 violations. Drop with log.
		deps.Log.Warn("ingest: decode_fail on unbound device — dropping",
			"dev_eui", devEUI, "err", err)
		return
	}
	tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		deps.Log.Error("ingest: decode_fail begin tx", "err", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlc.New(tx)
	if err := q.AppendMeasurement(ctx, sqlc.AppendMeasurementParams{
		Time:            pgtype.Timestamptz{Time: ingestTime, Valid: true},
		MeteringPointID: pgtype.UUID{Bytes: binding.MeteringPointID, Valid: true},
		Extra:           []byte(`{}`),
		RawPayload:      payload,
		DecodedObject:   []byte(`{}`),
		Quality:         QualityDecodeFail,
		BindingID:       pgtype.UUID{Bytes: binding.BindingID, Valid: true},
	}); err != nil {
		deps.Log.Error("ingest: decode_fail persist", "err", err)
		return
	}
	// Update device.last_seen even on decode_fail — we DID hear from the
	// device, the codec just choked. Phase 4 "device offline" alerts depend
	// on this being current.
	if err := q.UpdateDeviceLastSeen(ctx, sqlc.UpdateDeviceLastSeenParams{
		ID:         pgtype.UUID{Bytes: binding.DeviceID, Valid: true},
		LastSeenAt: pgtype.Timestamptz{Time: ingestTime, Valid: true},
	}); err != nil {
		deps.Log.Error("ingest: decode_fail update last_seen", "err", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		deps.Log.Error("ingest: decode_fail commit", "err", err)
		return
	}
}

// SQLCMappingStore wraps a *pgxpool.Pool so it satisfies MappingStore.
// Used at cmd/serve boot wiring; tests substitute an in-memory fake.
type SQLCMappingStore struct {
	Pool *pgxpool.Pool
}

// GetMappingsByProfile fetches the per-profile mapping rows in position order.
// Returns an empty slice (not nil) on no-rows so callers can distinguish
// "profile has no mappings" from "loader failed."
func (s *SQLCMappingStore) GetMappingsByProfile(ctx context.Context, profileID uuid.UUID) ([]profile.Mapping, error) {
	if s.Pool == nil {
		return nil, fmt.Errorf("ingest: nil pool")
	}
	q := sqlc.New(s.Pool)
	rows, err := q.ListMappingsByProfile(ctx, pgtype.UUID{Bytes: profileID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("ingest: list mappings: %w", err)
	}
	out := make([]profile.Mapping, 0, len(rows))
	for _, r := range rows {
		// Mapping rows default to identity scale (1) when the column is
		// SQL NULL — distinct from BigFloatFromNumeric's generic 0 default.
		scale := big.NewFloat(1)
		if r.Scale.Valid {
			scale = BigFloatFromNumeric(r.Scale)
		}
		out = append(out, profile.Mapping{
			JSONPointer: r.JsonPointer,
			Target:      r.Target,
			Scale:       scale,
			DataType:    r.DataType,
			Position:    r.Position,
		})
	}
	return out, nil
}

// numericFromBigFloat encodes a *big.Float as pgtype.Numeric via the lossless
// decimal text form. Mirrors swap.numericFromBigFloat — separate copy because
// importing swap from ingest would create a cycle (swap doesn't import ingest
// today, but the layering rule is "infrastructure packages don't import each
// other").
func numericFromBigFloat(f *big.Float) (pgtype.Numeric, error) {
	if f == nil {
		return pgtype.Numeric{}, nil
	}
	var n pgtype.Numeric
	if err := n.Scan(f.Text('f', -1)); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("scan %q: %w", f.Text('f', -1), err)
	}
	return n, nil
}

// numericFromBigFloatNullable returns Valid=false when f is nil so the
// destination column lands as SQL NULL rather than 0.
func numericFromBigFloatNullable(f *big.Float) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{Valid: false}
	}
	n, err := numericFromBigFloat(f)
	if err != nil {
		return pgtype.Numeric{Valid: false}
	}
	return n
}

// pgInt2Nullable wraps a *int16 for sqlc.AppendMeasurementParams which uses
// *int16 directly — the helper is a no-op pass-through that keeps the persist
// call site aligned with the other Nullable helpers.
func pgInt2Nullable(p *int16) *int16 { return p }

// pgFloat4Nullable mirrors pgInt2Nullable for *float32.
func pgFloat4Nullable(p *float32) *float32 { return p }

// pgBoolNullable mirrors pgInt2Nullable for *bool.
func pgBoolNullable(p *bool) *bool { return p }

// pgTimestamptzNullable converts a *time.Time to a pgtype.Timestamptz with
// Valid=false on nil so the destination column lands as SQL NULL.
func pgTimestamptzNullable(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// jsonOrEmptyObject marshals m to JSON; returns "{}" on nil/error so the
// JSONB column always has a valid value (the schema column is NOT NULL).
func jsonOrEmptyObject(m any) []byte {
	if m == nil {
		return []byte(`{}`)
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) == 0 {
		return []byte(`{}`)
	}
	return b
}
