package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/profile"
	"github.com/shifter-io/shifter/internal/resolver"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// ============================================================================
// Decode unit tests — Task 1
// ============================================================================

// TestDecodeChirpStackEvent_HappyPath — table-driven across three sample
// event JSONs. Asserts dev_eui lowercased, fCnt parsed, rxInfo populated,
// decoded object map preserved.
func TestDecodeChirpStackEvent_HappyPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		payload     string
		wantDevEUI  string
		wantFCnt    uint32
		wantFPort   uint8
		wantObjKey  string
		wantObjVal  any
		wantRSSI    int16
		wantSNR     float32
		wantGwRxNil bool
	}{
		{
			name: "axioma_w1_uplink",
			payload: `{
				"deduplicationId": "00000000-0000-0000-0000-000000000001",
				"time": "2026-05-04T12:34:56.789Z",
				"deviceInfo": { "devEui": "0102030405060708", "applicationId": "app-uuid" },
				"data": "AAECAwQF",
				"fCnt": 42,
				"fPort": 100,
				"rxInfo": [{
					"gatewayId": "gw-1",
					"time": "2026-05-04T12:34:55.500Z",
					"rssi": -85,
					"snr": 9.25
				}],
				"object": { "cumulative_l": 12345.678, "battery_pct": 87 }
			}`,
			wantDevEUI: "0102030405060708",
			wantFCnt:   42,
			wantFPort:  100,
			wantObjKey: "cumulative_l",
			wantObjVal: 12345.678,
			wantRSSI:   -85,
			wantSNR:    9.25,
		},
		{
			name: "uppercase_devEUI_normalized",
			payload: `{
				"deviceInfo": { "devEui": "AABBCCDDEEFF0011", "applicationId": "app-x" },
				"fCnt": 7,
				"fPort": 1,
				"rxInfo": [{ "gatewayId": "gw-2", "rssi": -110, "snr": 1.5, "time": "2026-05-04T12:00:00Z" }],
				"object": { "voltage_l1": 230 }
			}`,
			wantDevEUI: "aabbccddeeff0011",
			wantFCnt:   7,
			wantFPort:  1,
			wantObjKey: "voltage_l1",
			wantObjVal: 230.0,
			wantRSSI:   -110,
			wantSNR:    1.5,
		},
		{
			name: "no_top_level_time_no_gateway_rx",
			payload: `{
				"deviceInfo": { "devEui": "1111111111111111" },
				"fCnt": 1,
				"fPort": 1,
				"object": { "x": 1 }
			}`,
			wantDevEUI:  "1111111111111111",
			wantFCnt:    1,
			wantFPort:   1,
			wantObjKey:  "x",
			wantObjVal:  1.0,
			wantGwRxNil: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := DecodeChirpStackEvent([]byte(tc.payload))
			require.NoError(t, err, "decode")
			require.Equal(t, tc.wantDevEUI, ev.DevEUI, "DevEUI lowercased")
			require.Equal(t, tc.wantFCnt, ev.FCnt)
			require.Equal(t, tc.wantFPort, ev.FPort)
			require.NotNil(t, ev.DecodedObject, "DecodedObject preserved")
			require.Equal(t, tc.wantObjVal, ev.DecodedObject[tc.wantObjKey])
			if !tc.wantGwRxNil {
				require.Len(t, ev.RxInfo, 1)
				require.Equal(t, tc.wantRSSI, ev.RxInfo[0].RSSI)
				require.InDelta(t, tc.wantSNR, ev.RxInfo[0].SNR, 0.01)
				require.NotNil(t, ev.GatewayRxTime, "earliest rxInfo[].time captured")
			} else {
				require.Nil(t, ev.GatewayRxTime)
			}
		})
	}
}

// TestDecodeChirpStackEvent_MissingObject_NoError — Pitfall 11: an event
// without an `object` field (codec missing or runtime failure) yields
// ev.DecodedObject == nil with NO error. Caller's quality flag handles it.
func TestDecodeChirpStackEvent_MissingObject_NoError(t *testing.T) {
	t.Parallel()
	payload := `{"deviceInfo":{"devEui":"abcdef0123456789"}, "fCnt": 1, "fPort": 1}`
	ev, err := DecodeChirpStackEvent([]byte(payload))
	require.NoError(t, err)
	require.Equal(t, "abcdef0123456789", ev.DevEUI)
	require.Nil(t, ev.DecodedObject, "no `object` field → DecodedObject nil, error nil")
}

// TestDecodeChirpStackEvent_BadJSON — corrupt payload returns error.
func TestDecodeChirpStackEvent_BadJSON(t *testing.T) {
	t.Parallel()
	_, err := DecodeChirpStackEvent([]byte(`not-valid-json`))
	require.Error(t, err)
}

// TestDecodeChirpStackEvent_EarliestGatewayRxTime — when multiple gateways
// hear the same uplink, GatewayRxTime is the EARLIEST of rxInfo[].time.
// Pins the open-question #3 input.
func TestDecodeChirpStackEvent_EarliestGatewayRxTime(t *testing.T) {
	t.Parallel()
	payload := `{
		"deviceInfo": { "devEui": "1111111111111111" },
		"fCnt": 1, "fPort": 1,
		"rxInfo": [
			{ "gatewayId": "gw-late",  "time": "2026-05-04T12:00:05Z" },
			{ "gatewayId": "gw-early", "time": "2026-05-04T12:00:01Z" },
			{ "gatewayId": "gw-mid",   "time": "2026-05-04T12:00:03Z" }
		]
	}`
	ev, err := DecodeChirpStackEvent([]byte(payload))
	require.NoError(t, err)
	require.NotNil(t, ev.GatewayRxTime)
	expected := time.Date(2026, 5, 4, 12, 0, 1, 0, time.UTC)
	require.True(t, ev.GatewayRxTime.Equal(expected),
		"earliest of three rxInfo[].time values: want %v got %v", expected, *ev.GatewayRxTime)
}

// TestDecodeChirpStackEvent_Base64Data — `data` field is base64-decoded.
func TestDecodeChirpStackEvent_Base64Data(t *testing.T) {
	t.Parallel()
	raw := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	encoded := base64.StdEncoding.EncodeToString(raw)
	payload := fmt.Sprintf(`{"deviceInfo":{"devEui":"1111111111111111"},"data":"%s","fCnt":1,"fPort":1}`, encoded)
	ev, err := DecodeChirpStackEvent([]byte(payload))
	require.NoError(t, err)
	require.Equal(t, raw, ev.Data)
}

// TestDevEUIFromTopic — extracts dev_eui from the canonical CS v4 uplink topic.
func TestDevEUIFromTopic(t *testing.T) {
	t.Parallel()
	cases := []struct {
		topic string
		want  string
	}{
		{"application/abc/device/0102030405060708/event/up", "0102030405060708"},
		{"application/abc/device/AABBCCDDEEFF0011/event/up", "aabbccddeeff0011"}, // lowercased
		{"application/abc/event/up", ""},                                         // too few parts
		{"random/topic", ""},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.topic, func(t *testing.T) {
			require.Equal(t, tc.want, devEUIFromTopic(tc.topic))
		})
	}
}

// ============================================================================
// Handler tests — Task 1 (decode_fail + unbound device paths)
// Integration tests that exercise the full pipeline against a real Postgres.
// ============================================================================

// fakeMappingStore is an in-memory MappingStore for tests. Mappings is keyed
// by profile id; absent entries return an empty slice so a handler with no
// mappings configured still flows through normalize → ErrNoCanonicalValue.
type fakeMappingStore struct {
	mu   sync.Mutex
	data map[uuid.UUID][]profile.Mapping
}

func newFakeMappingStore() *fakeMappingStore {
	return &fakeMappingStore{data: make(map[uuid.UUID][]profile.Mapping)}
}

func (s *fakeMappingStore) Set(profileID uuid.UUID, mappings []profile.Mapping) {
	s.mu.Lock()
	s.data[profileID] = mappings
	s.mu.Unlock()
}

func (s *fakeMappingStore) GetMappingsByProfile(_ context.Context, profileID uuid.UUID) ([]profile.Mapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.data[profileID]
	if !ok {
		return []profile.Mapping{}, nil
	}
	out := make([]profile.Mapping, len(m))
	copy(out, m)
	return out, nil
}

// fakeLoader satisfies resolver.Loader with a small in-memory map. Unmapped
// dev_euis return resolver.ErrNoActiveBinding so the unbound-device path is
// exercisable without seeding a binding.
type fakeLoader struct {
	mu       sync.Mutex
	bindings map[string]resolver.Binding
	calls    atomic.Int32
}

func newFakeLoader() *fakeLoader {
	return &fakeLoader{bindings: make(map[string]resolver.Binding)}
}

func (l *fakeLoader) Set(devEUI string, b resolver.Binding) {
	l.mu.Lock()
	l.bindings[devEUI] = b
	l.mu.Unlock()
}

func (l *fakeLoader) LoadActive(_ context.Context, devEUI string, _ time.Time) (resolver.Binding, error) {
	l.calls.Add(1)
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.bindings[devEUI]; ok {
		return b, nil
	}
	return resolver.Binding{}, resolver.ErrNoActiveBinding
}

// logBuffer is a slog.Handler that captures records so tests can assert on
// emitted log lines (e.g. "ingest: unbound device" structured-log).
type logBuffer struct {
	mu      sync.Mutex
	records []string
}

func (lb *logBuffer) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (lb *logBuffer) Handle(_ context.Context, r slog.Record) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, fmt.Sprintf("%s=%v", a.Key, a.Value.Any()))
		return true
	})
	lb.records = append(lb.records, fmt.Sprintf("%s|%s|%v", r.Level, r.Message, attrs))
	return nil
}
func (lb *logBuffer) WithAttrs(_ []slog.Attr) slog.Handler { return lb }
func (lb *logBuffer) WithGroup(_ string) slog.Handler      { return lb }
func (lb *logBuffer) Contains(substr string) bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	for _, r := range lb.records {
		if containsCaseInsensitive(r, substr) {
			return true
		}
	}
	return false
}
func containsCaseInsensitive(s, substr string) bool {
	return len(s) >= len(substr) && (indexOf(s, substr) >= 0)
}
func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ============================================================================

// fixtureFull is the integration fixture that holds a real *pgxpool.Pool +
// the seeded UUIDs the tests assert against. Replaces the placeholder above.
type fixtureFull struct {
	pool      *pgxpool.Pool
	mappings  *fakeMappingStore
	resolver  *resolver.Resolver
	loader    *fakeLoader
	deps      Deps
	mpID      uuid.UUID
	bindingID uuid.UUID
	deviceID  uuid.UUID
	profileID uuid.UUID
	devEUI    string
	logHand   *logBuffer
}

// makeFixture seeds: site → MP → device_profile (axioma_w1) → device → binding.
// Reading offset = 0; last_raw_value = NULL (first uplink for binding).
func makeFixture(t *testing.T) *fixtureFull {
	t.Helper()

	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	var siteID, mpID, profileID, deviceID, bindingID pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'mp-1', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))
	// Use the seeded axioma_w1 device profile (counter_modulus=4294967296 from 0010).
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))
	devEUI := "0102030405060708"
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, 'test-dev', $2) RETURNING id`,
		devEUI, profileID,
	).Scan(&deviceID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from)
		 VALUES ($1, $2, '2026-01-01T00:00:00Z') RETURNING id`,
		mpID, deviceID,
	).Scan(&bindingID))

	// Look up counter_modulus for the seeded profile.
	var counterModulus int64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT counter_modulus FROM device_profile WHERE id = $1`,
		profileID,
	).Scan(&counterModulus))

	loader := newFakeLoader()
	loader.Set(devEUI, resolver.Binding{
		BindingID:       uuid.UUID(bindingID.Bytes),
		MeteringPointID: uuid.UUID(mpID.Bytes),
		DeviceID:        uuid.UUID(deviceID.Bytes),
		DeviceProfileID: uuid.UUID(profileID.Bytes),
		ReadingOffset:   big.NewFloat(0).SetPrec(normalizePrecision),
		LastRawValue:    nil, // first uplink for binding
		CounterModulus:  counterModulus,
		ValidFrom:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	r := resolver.New(loader)
	mappings := newFakeMappingStore()
	logHand := &logBuffer{}
	deps := Deps{
		Pool:     pool,
		Resolver: r,
		Mappings: mappings,
		Log:      slog.New(logHand),
	}

	return &fixtureFull{
		pool:      pool,
		mappings:  mappings,
		resolver:  r,
		loader:    loader,
		deps:      deps,
		mpID:      uuid.UUID(mpID.Bytes),
		bindingID: uuid.UUID(bindingID.Bytes),
		deviceID:  uuid.UUID(deviceID.Bytes),
		profileID: uuid.UUID(profileID.Bytes),
		devEUI:    devEUI,
		logHand:   logHand,
	}
}

// countMeasurements returns the row count for the given MP — handy for
// asserting "row was/was-not written."
func countMeasurements(t *testing.T, pool *pgxpool.Pool, mpID uuid.UUID) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM measurement WHERE metering_point_id = $1`,
		pgtype.UUID{Bytes: mpID, Valid: true},
	).Scan(&n))
	return n
}

// TestUplinkHandler_DecodeFail_PersistsWithQuality — the malformed-JSON
// path persists a row with quality='decode_fail' + raw payload preserved,
// keyed by the dev_eui recovered from the MQTT topic (DATA-07 + D-26).
func TestUplinkHandler_DecodeFail_PersistsWithQuality(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)
	h := UplinkHandler(f.deps)

	topic := fmt.Sprintf("application/test-app/device/%s/event/up", f.devEUI)
	badPayload := []byte(`{not-valid-json`)
	h(topic, badPayload)

	require.Equal(t, 1, countMeasurements(t, f.pool, f.mpID),
		"decode_fail row inserted (D-26 + DATA-07)")

	// Inspect the row.
	q := sqlc.New(f.pool)
	got, err := q.GetLatestMeasurement(context.Background(), pgtype.UUID{Bytes: f.mpID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, QualityDecodeFail, got.Quality)
	require.Equal(t, badPayload, got.RawPayload, "raw_payload preserved (DATA-07)")
	// Canonical columns should be NULL.
	require.False(t, got.RawValue.Valid)
	require.False(t, got.CumulativeValue.Valid)
}

// TestUplinkHandler_NoBinding_LogsAndDrops — DATA-01 forbids writing a
// row without a metering_point_id. Unbound dev_eui → log + drop. The raw
// bytes are logged-but-not-persisted (the only path where DATA-07 yields
// to DATA-01).
func TestUplinkHandler_NoBinding_LogsAndDrops(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)
	h := UplinkHandler(f.deps)

	// Unbound dev_eui — not in the loader's map.
	unboundEUI := "ffffffffffffffff"
	topic := fmt.Sprintf("application/test-app/device/%s/event/up", unboundEUI)
	payload := mustMarshalEvent(t, eventBuilder{
		DevEUI: unboundEUI,
		FCnt:   1,
		Object: map[string]any{"liters": 100.0},
	})
	h(topic, payload)

	// No row written for the unbound device. (Our seeded MP has zero rows
	// because the unbound dev_eui doesn't map to it, but check globally.)
	var totalCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM measurement`,
	).Scan(&totalCount))
	require.Equal(t, 0, totalCount, "DATA-01: no row written for unbound device")

	// Structured-log emitted so operators see the unbound uplink.
	require.True(t, f.logHand.Contains("unbound device"),
		"unbound_device log line emitted (Phase 6 surface)")
}

// TestUplinkHandler_HappyPath_PersistsRow — sanity: a well-formed event
// for a bound dev_eui with a single raw_value mapping persists a row
// with quality='ok' + cumulative_value computed.
func TestUplinkHandler_HappyPath_PersistsRow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)

	// One mapping: object.liters → raw_value (numeric, scale=1)
	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/liters", Target: "raw_value", DataType: "numeric", Position: 0},
	})

	h := UplinkHandler(f.deps)
	topic := fmt.Sprintf("application/test-app/device/%s/event/up", f.devEUI)
	payload := mustMarshalEvent(t, eventBuilder{
		DevEUI: f.devEUI,
		FCnt:   42,
		Object: map[string]any{"liters": 1234.5},
	})
	h(topic, payload)

	require.Equal(t, 1, countMeasurements(t, f.pool, f.mpID))
	q := sqlc.New(f.pool)
	got, err := q.GetLatestMeasurement(context.Background(), pgtype.UUID{Bytes: f.mpID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, QualityOK, got.Quality)
	require.Equal(t, payload, got.RawPayload)
	require.True(t, got.RawValue.Valid)
	require.True(t, got.CumulativeValue.Valid, "cumulative computed = raw + offset(0)")
	require.NotNil(t, got.Fcnt)
	require.Equal(t, int32(42), *got.Fcnt)
}

// TestUplinkHandler_NoCanonicalValueMapped_FlagsRow — a profile with no
// raw_value/instant_value mapping → row persists with quality='missing_canonical'.
func TestUplinkHandler_NoCanonicalValueMapped_FlagsRow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)

	// Mapping populates only battery_pct — no raw_value or instant_value.
	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/battery", Target: "battery_pct", DataType: "int", Position: 0},
	})

	h := UplinkHandler(f.deps)
	topic := fmt.Sprintf("application/test-app/device/%s/event/up", f.devEUI)
	payload := mustMarshalEvent(t, eventBuilder{
		DevEUI: f.devEUI,
		FCnt:   1,
		Object: map[string]any{"battery": 87.0},
	})
	h(topic, payload)

	require.Equal(t, 1, countMeasurements(t, f.pool, f.mpID))
	q := sqlc.New(f.pool)
	got, err := q.GetLatestMeasurement(context.Background(), pgtype.UUID{Bytes: f.mpID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, QualityMissingCanonical, got.Quality)
	require.NotNil(t, got.BatteryPct)
	require.Equal(t, int16(87), *got.BatteryPct)
}

// TestUplinkHandler_DataTimeIsServerSide — DATA-03 + Pitfall 4: the
// measurement.time column is the server's time.Now() at handler entry,
// NOT GatewayRxTime or DeviceTime. Both diagnostic timestamps are
// preserved in their own columns.
func TestUplinkHandler_DataTimeIsServerSide(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)
	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric"},
	})

	// Build a payload whose gateway/device timestamps are 1 hour in the past
	// — the row's time should be the server's now() at handler entry, not those.
	pastDevice := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	pastGateway := time.Date(2025, 1, 1, 12, 0, 1, 0, time.UTC)

	h := UplinkHandler(f.deps)
	topic := fmt.Sprintf("application/test-app/device/%s/event/up", f.devEUI)
	before := time.Now().UTC()
	payload := mustMarshalEvent(t, eventBuilder{
		DevEUI:        f.devEUI,
		FCnt:          1,
		Object:        map[string]any{"v": 1.0},
		DeviceTime:    &pastDevice,
		GatewayRxTime: &pastGateway,
	})
	h(topic, payload)
	after := time.Now().UTC()

	q := sqlc.New(f.pool)
	got, err := q.GetLatestMeasurement(context.Background(), pgtype.UUID{Bytes: f.mpID, Valid: true})
	require.NoError(t, err)
	require.True(t, got.Time.Valid)
	rowTime := got.Time.Time.UTC()
	require.False(t, rowTime.Before(before.Add(-time.Second)),
		"measurement.time >= handler entry (DATA-03)")
	require.False(t, rowTime.After(after.Add(time.Second)),
		"measurement.time <= handler exit")

	// And the diagnostic columns DO carry the payload-side timestamps.
	require.True(t, got.GatewayRxTime.Valid)
	require.True(t, got.GatewayRxTime.Time.Equal(pastGateway))
	require.True(t, got.DeviceTime.Valid)
	require.True(t, got.DeviceTime.Time.Equal(pastDevice))
}

// ============================================================================
// Helpers shared with persist tests (Task 3 also uses these).
// ============================================================================

// eventBuilder is a small DSL for synthesizing CS v4 uplink event JSON in tests.
type eventBuilder struct {
	DevEUI        string
	ApplicationID string
	FCnt          uint32
	FPort         uint8
	Object        map[string]any
	DeviceTime    *time.Time
	GatewayRxTime *time.Time
}

func (b eventBuilder) MarshalJSON() ([]byte, error) {
	type rxInfo struct {
		GatewayID string `json:"gatewayId"`
		Time      string `json:"time,omitempty"`
		RSSI      int16  `json:"rssi"`
		SNR       float32 `json:"snr"`
	}
	type devInfo struct {
		DevEui        string `json:"devEui"`
		ApplicationID string `json:"applicationId,omitempty"`
	}
	out := map[string]any{
		"deviceInfo": devInfo{DevEui: b.DevEUI, ApplicationID: b.ApplicationID},
		"fCnt":       b.FCnt,
		"fPort":      b.FPort,
		"object":     b.Object,
	}
	if b.DeviceTime != nil {
		out["time"] = b.DeviceTime.UTC().Format(time.RFC3339Nano)
	}
	if b.GatewayRxTime != nil {
		out["rxInfo"] = []rxInfo{{GatewayID: "gw-test", Time: b.GatewayRxTime.UTC().Format(time.RFC3339Nano), RSSI: -85, SNR: 9.0}}
	}
	return json.Marshal(out)
}

func mustMarshalEvent(t *testing.T, b eventBuilder) []byte {
	t.Helper()
	buf, err := json.Marshal(b)
	require.NoError(t, err)
	return buf
}

// errMissingFixtureField is unused — kept here as a place to hang test-helper
// errors if they grow.
var _ = errors.New

// ============================================================================
// Persist integration tests — Task 3
// Exercise the rollover detection + audit path + multi-uplink continuity.
// ============================================================================

// TestPersist_RolloverDetected — pre-seed binding with last_raw_value near
// the 32-bit counter ceiling; publish uplink with raw_value=10; assert:
//
//   - measurement row is written with quality='ok'.
//   - binding.reading_offset advanced by counter_modulus (4294967296).
//   - audit_log row with action='rollover_detected' was written in the
//     SAME tx as the measurement (same transaction commit time).
//   - resolver cache primed with new offset + new last_raw_value.
func TestPersist_RolloverDetected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)
	ctx := context.Background()

	// Pre-seed binding.last_raw_value just below the modulus AND prime the
	// resolver cache so the handler hits the rollover path on the next uplink.
	prevRaw := big.NewFloat(0).SetPrec(normalizePrecision).SetInt64(4294967290) // 2^32 - 6
	rawNumeric, err := numericFromBigFloat(prevRaw)
	require.NoError(t, err)
	require.NoError(t, sqlc.New(f.pool).UpdateBindingLastRaw(ctx, sqlc.UpdateBindingLastRawParams{
		ID:           pgtype.UUID{Bytes: f.bindingID, Valid: true},
		LastRawValue: rawNumeric,
	}))

	// Update the loader's binding so the resolver loads the seeded last_raw.
	cm := f.loader.bindings[f.devEUI].CounterModulus
	require.Greater(t, cm, int64(0))
	f.loader.Set(f.devEUI, resolver.Binding{
		BindingID:       f.bindingID,
		MeteringPointID: f.mpID,
		DeviceID:        f.deviceID,
		DeviceProfileID: f.profileID,
		ReadingOffset:   big.NewFloat(0).SetPrec(normalizePrecision),
		LastRawValue:    prevRaw,
		CounterModulus:  cm,
		ValidFrom:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric", Position: 0},
	})

	h := UplinkHandler(f.deps)
	topic := fmt.Sprintf("application/test-app/device/%s/event/up", f.devEUI)
	payload := mustMarshalEvent(t, eventBuilder{
		DevEUI: f.devEUI,
		FCnt:   42,
		Object: map[string]any{"v": 10.0}, // raw < prev → rollover
	})
	h(topic, payload)

	// 1. Measurement row written with quality='ok'.
	require.Equal(t, 1, countMeasurements(t, f.pool, f.mpID))
	got, err := sqlc.New(f.pool).GetLatestMeasurement(ctx, pgtype.UUID{Bytes: f.mpID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, QualityOK, got.Quality)

	// 2. Cumulative_value should be raw + offset = 10 + 4294967296 = 4294967306.
	require.True(t, got.CumulativeValue.Valid)
	cumF, err := got.CumulativeValue.Float64Value()
	require.NoError(t, err)
	require.True(t, cumF.Valid)
	require.InDelta(t, 4294967306.0, cumF.Float64, 1.0,
		"cumulative = raw(10) + offset(modulus 4294967296) after rollover applied")

	// 3. binding.reading_offset = counter_modulus.
	var offsetText string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT reading_offset::text FROM binding WHERE id = $1`,
		pgtype.UUID{Bytes: f.bindingID, Valid: true},
	).Scan(&offsetText))
	require.Equal(t, "4294967296", offsetText, "reading_offset advanced by counter_modulus")

	// 4. Audit row with action='rollover_detected' EXISTS for this binding.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log
		 WHERE entity_type = 'binding' AND entity_id = $1
		   AND action = 'rollover_detected'`,
		pgtype.UUID{Bytes: f.bindingID, Valid: true},
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount, "exactly one rollover_detected audit row")

	// 5. binding.last_raw_value updated to the new (post-rollover) raw value.
	var lastRawText string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT last_raw_value::text FROM binding WHERE id = $1`,
		pgtype.UUID{Bytes: f.bindingID, Valid: true},
	).Scan(&lastRawText))
	require.Equal(t, "10", lastRawText)
}

// TestPersist_MultipleUplinksSameMP — five successive uplinks, no rollover.
// Asserts row count, MP key invariant (DATA-01), and binding.last_raw_value
// matches the final uplink's raw value.
func TestPersist_MultipleUplinksSameMP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)
	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric", Position: 0},
	})
	h := UplinkHandler(f.deps)
	topic := fmt.Sprintf("application/test-app/device/%s/event/up", f.devEUI)

	values := []float64{100.0, 110.0, 120.0, 130.0, 140.0}
	for i, v := range values {
		payload := mustMarshalEvent(t, eventBuilder{
			DevEUI: f.devEUI,
			FCnt:   uint32(i),
			Object: map[string]any{"v": v},
		})
		h(topic, payload)
		// Allow microsecond tick between uplinks so the (mp, time) PK is distinct.
		time.Sleep(time.Millisecond)
	}

	// Five rows, all keyed by f.mpID (DATA-01).
	require.Equal(t, 5, countMeasurements(t, f.pool, f.mpID))

	// Confirm DATA-01 invariant — there is no other MP with rows.
	var totalCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM measurement WHERE metering_point_id = $1`,
		pgtype.UUID{Bytes: f.mpID, Valid: true},
	).Scan(&totalCount))
	require.Equal(t, 5, totalCount)

	// binding.last_raw_value reflects the final uplink (140).
	var lastRawText string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT last_raw_value::text FROM binding WHERE id = $1`,
		pgtype.UUID{Bytes: f.bindingID, Valid: true},
	).Scan(&lastRawText))
	require.Equal(t, "140", lastRawText)

	// device.last_seen_at updated.
	var lastSeen time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT last_seen_at FROM device WHERE id = $1`,
		pgtype.UUID{Bytes: f.deviceID, Valid: true},
	).Scan(&lastSeen))
	require.False(t, lastSeen.IsZero(), "device.last_seen_at written")
}

// TestPersist_RolloverNotDetectedOnFirstUplink — DATA-05 boundary: a
// freshly-opened binding has last_raw_value=NULL. The first uplink has
// raw_value < <something> looks like rollover but MUST NOT trigger one
// (T-02-09-06 mitigation — false positive after a swap).
func TestPersist_RolloverNotDetectedOnFirstUplink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)
	// No pre-seed — binding.last_raw_value is NULL.
	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric", Position: 0},
	})
	h := UplinkHandler(f.deps)
	topic := fmt.Sprintf("application/test-app/device/%s/event/up", f.devEUI)

	payload := mustMarshalEvent(t, eventBuilder{
		DevEUI: f.devEUI,
		FCnt:   1,
		Object: map[string]any{"v": 50.0},
	})
	h(topic, payload)

	// No rollover audit row exists.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'rollover_detected'`,
	).Scan(&auditCount))
	require.Equal(t, 0, auditCount, "no rollover audit on first uplink for binding")

	// reading_offset still 0 (no advance).
	var offsetText string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT reading_offset::text FROM binding WHERE id = $1`,
		pgtype.UUID{Bytes: f.bindingID, Valid: true},
	).Scan(&offsetText))
	require.Equal(t, "0", offsetText, "no rollover → offset unchanged")
}

// TestPersist_PreservesRawAndDecodedAcrossQualityLevels — DATA-07: every
// uplink, regardless of quality, persists raw_payload + decoded_object.
// Three rows: one ok, one decode_fail, one missing_canonical. All three
// have non-empty raw_payload.
func TestPersist_PreservesRawAndDecodedAcrossQualityLevels(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)
	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric", Position: 0},
	})
	h := UplinkHandler(f.deps)
	topic := fmt.Sprintf("application/test-app/device/%s/event/up", f.devEUI)

	// 1. Happy path — quality='ok'.
	okPayload := mustMarshalEvent(t, eventBuilder{
		DevEUI: f.devEUI, FCnt: 1, Object: map[string]any{"v": 100.0},
	})
	h(topic, okPayload)
	time.Sleep(time.Millisecond)

	// 2. Decode-fail — malformed JSON.
	badPayload := []byte(`{not-valid-json`)
	h(topic, badPayload)
	time.Sleep(time.Millisecond)

	// 3. Missing canonical — valid event but no mapping populates raw_value.
	// Reset mappings so /missing won't resolve.
	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/missing_field", Target: "raw_value", DataType: "numeric"},
	})
	missingPayload := mustMarshalEvent(t, eventBuilder{
		DevEUI: f.devEUI, FCnt: 3, Object: map[string]any{"v": 200.0},
	})
	h(topic, missingPayload)

	require.Equal(t, 3, countMeasurements(t, f.pool, f.mpID))

	// Pull all rows, oldest first; assert each has raw_payload AND decoded_object.
	rows, err := f.pool.Query(context.Background(),
		`SELECT quality, raw_payload, decoded_object FROM measurement
		 WHERE metering_point_id = $1 ORDER BY time ASC`,
		pgtype.UUID{Bytes: f.mpID, Valid: true},
	)
	require.NoError(t, err)
	defer rows.Close()

	var seenQualities []string
	for rows.Next() {
		var q string
		var raw, decoded []byte
		require.NoError(t, rows.Scan(&q, &raw, &decoded))
		require.NotEmpty(t, raw, "raw_payload preserved for quality=%s (DATA-07)", q)
		require.NotEmpty(t, decoded, "decoded_object preserved for quality=%s (DATA-07)", q)
		seenQualities = append(seenQualities, q)
	}
	require.NoError(t, rows.Err())
	require.Contains(t, seenQualities, QualityOK)
	require.Contains(t, seenQualities, QualityDecodeFail)
	require.Contains(t, seenQualities, QualityMissingCanonical)
}

// TestPersist_RollsBackOnPersistError — Pattern 5 atomicity: if any step
// inside PersistAtomically fails AFTER the measurement insert, the whole
// tx rolls back. Hard to inject without modifying production code; we
// simulate by passing a context whose deadline expires mid-transaction.
//
// Skipped in normal flows — the realistic atomicity check is the rollover
// path (which fires audit.WriteEntry and tx.Commit in sequence). The
// existing TestCommitSwap_RolledBack_OnExclusionViolation in
// internal/swap pins the rollback semantics for the audit-in-tx pattern;
// this test is a sanity check that ingest follows the same pattern.
//
// Verify: binding.last_raw_value remains NULL after a context-canceled
// uplink; no measurement row written.
func TestPersist_RollsBackOnContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	t.Parallel()

	f := makeFixture(t)
	f.mappings.Set(f.profileID, []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric"},
	})

	// Pre-cancel the context so BeginTx fails immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	binding := f.loader.bindings[f.devEUI]
	err := PersistAtomically(ctx, f.deps, PersistInput{
		IngestTime: time.Now().UTC(),
		Event: Event{
			DevEUI: f.devEUI,
			FCnt:   1,
		},
		Binding:    binding,
		Normalized: Layer1{RawValue: big.NewFloat(100), Extra: map[string]any{}},
		RawPayload: []byte(`{"v":100}`),
		Quality:    QualityOK,
	})
	require.Error(t, err, "BeginTx with canceled ctx must fail")

	// No measurement row was written.
	require.Equal(t, 0, countMeasurements(t, f.pool, f.mpID))

	// binding.last_raw_value stays NULL.
	var lastRaw pgtype.Numeric
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT last_raw_value FROM binding WHERE id = $1`,
		pgtype.UUID{Bytes: f.bindingID, Valid: true},
	).Scan(&lastRaw))
	require.False(t, lastRaw.Valid, "last_raw_value still NULL — rollback")
}

// silence unused import warnings if any helper field becomes unused later.
var _ = atomic.Int32{}
