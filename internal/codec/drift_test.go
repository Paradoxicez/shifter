package codec_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/codec"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/profile/codecs"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// seedDriftProfile inserts a minimal device_profile row for drift check tests.
// Returns the inserted profile UUID string.
func seedDriftProfile(t *testing.T, pool *pgxpool.Pool, slug, codecJS string, catalogSource string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO device_profile (
			slug, name, vendor, family, capabilities, counter_modulus,
			codec_js, region, mac_version,
			catalog_source, catalog_source_version, customer_edited,
			battery_curve, expected_uplink_interval_seconds,
			offline_threshold_multiplier, anomaly_compatibility
		) VALUES (
			$1, 'Test Profile', 'TestVendor', 'TestFamily', ARRAY['cumulative'], 4294967296,
			$2, NULL, 'LORAWAN_1_0_3',
			$3, '1.0.0', FALSE,
			'linear_pct', 3600, 3.0, 'full'
		) RETURNING id`,
		slug, codecJS, catalogSource,
	).Scan(&id)
	require.NoError(t, err, "seedDriftProfile insert failed")
	return id
}

// TestDriftCheck_HashesMatch_NoChange verifies that when codec_js already
// matches the embedded source, customer_edited stays FALSE.
func TestDriftCheck_HashesMatch_NoChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Use the embedded Axioma codec as-is — hashes should match.
	slug := "axioma_w1"
	embeddedJS := codecs.CodecBySlug(slug)
	require.NotEmpty(t, embeddedJS)

	seedDriftProfile(t, pool, slug+"-match", embeddedJS, slug)

	err := codec.RunCatalogDriftCheck(ctx, pool)
	require.NoError(t, err)

	var customerEdited bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT customer_edited FROM device_profile WHERE slug = $1`, slug+"-match",
	).Scan(&customerEdited))
	require.False(t, customerEdited, "matching hashes should not flip customer_edited")
}

// TestDriftCheck_HashesDiffer_MarksCustomerEdited verifies that when codec_js
// has been modified by an operator, customer_edited is flipped to TRUE and
// codec_js is NOT overwritten.
func TestDriftCheck_HashesDiffer_MarksCustomerEdited(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	slug := "axioma_w1"
	operatorEdit := "// operator edit — custom logic\nfunction Decode(bytes, port) { return {}; }"

	seedDriftProfile(t, pool, slug+"-edited", operatorEdit, slug)

	err := codec.RunCatalogDriftCheck(ctx, pool)
	require.NoError(t, err)

	var customerEdited bool
	var codecJS string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT customer_edited, codec_js FROM device_profile WHERE slug = $1`, slug+"-edited",
	).Scan(&customerEdited, &codecJS))
	require.True(t, customerEdited, "differing hashes must flip customer_edited to TRUE")
	require.Equal(t, operatorEdit, codecJS, "operator codec_js must NOT be overwritten")
}

// TestDriftCheck_PlaceholderMarker_OverwritesCodec verifies that when codec_js
// starts with the migration placeholder marker, the drift check overwrites it
// with the embedded source and does NOT flip customer_edited.
func TestDriftCheck_PlaceholderMarker_OverwritesCodec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	slug := "itron_kinmy_lora"
	placeholder := "// placeholder — replaced at boot by RunCatalogSeedSync (plan 07-03)"

	seedDriftProfile(t, pool, slug+"-placeholder", placeholder, slug)

	err := codec.RunCatalogDriftCheck(ctx, pool)
	require.NoError(t, err)

	var customerEdited bool
	var codecJS string
	var syncedAt *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT customer_edited, codec_js, codec_js_synced_at::text FROM device_profile WHERE slug = $1`,
		slug+"-placeholder",
	).Scan(&customerEdited, &codecJS, &syncedAt))
	require.False(t, customerEdited, "placeholder overwrite must NOT flip customer_edited")
	require.Equal(t, codecs.CodecBySlug(slug), codecJS, "codec_js must be replaced with embedded source")
	require.Nil(t, syncedAt, "codec_js_synced_at must be NULL after overwrite so seed re-pushes to ChirpStack")
}
