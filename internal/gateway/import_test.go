package gateway_test

// Plan 07-13 Task 1 (TDD RED → GREEN): bulk gateway import service tests.
//
// Tests cover:
//   - Happy path 3-row validation
//   - Validation error (invalid EUI)
//   - Idempotent re-run (3 created → re-run → 3 skipped)
//   - Partial update (re-run with changed name → "updated")
//   - Per-row audit assertion (N audit rows written per commit)

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	gw "github.com/shifter-io/shifter/internal/gateway"
	"github.com/shifter-io/shifter/internal/testsupport"
)

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func setupImportTest(t *testing.T) (*pgxpool.Pool, *sqlc.Queries) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))
	return pool, sqlc.New(pool)
}

func makeImportSvc(pool *pgxpool.Pool) *gw.ImportService {
	return gw.NewImportService(pool)
}

// csvWith builds a CSV string with the canonical gateway_import header and the
// given rows (each row is a []string of [gateway_eui, name, description,
// latitude, longitude, region]).
func csvWith(rows [][]string) []byte {
	lines := []string{"gateway_eui,name,description,latitude,longitude,region"}
	for _, r := range rows {
		cols := make([]string, 6)
		copy(cols, r)
		lines = append(lines, strings.Join(cols, ","))
	}
	return []byte(strings.Join(lines, "\n"))
}

// TestGatewayImport_Validate_3RowsAllValid — happy path: 3 valid rows
// pass Validate without errors.
func TestGatewayImport_Validate_3RowsAllValid(t *testing.T) {
	pool, _ := setupImportTest(t)
	svc := makeImportSvc(pool)

	csv := csvWith([][]string{
		{"aabbccddeeff0001", "GW Alpha", "desc a", "13.0", "100.0", "as923_2"},
		{"aabbccddeeff0002", "GW Beta", "", "", "", "eu868"},
		{"aabbccddeeff0003", "GW Gamma", "", "13.5", "100.5", "as923_2"},
	})

	result, err := svc.Validate(context.Background(), csv)
	require.NoError(t, err)
	require.Equal(t, 3, result.ValidRows)
	require.Equal(t, 0, result.ErrorRows)
	require.Empty(t, result.Errors)
}

// TestGatewayImport_Validate_InvalidEUI — a row with an invalid EUI produces
// an error outcome; the other valid rows are not affected.
func TestGatewayImport_Validate_InvalidEUI(t *testing.T) {
	pool, _ := setupImportTest(t)
	svc := makeImportSvc(pool)

	csv := csvWith([][]string{
		{"not-a-valid-eui", "Bad GW", "", "", "", "as923_2"},
		{"aabbccddeeff0002", "Good GW", "", "", "", "as923_2"},
	})

	result, err := svc.Validate(context.Background(), csv)
	require.NoError(t, err)
	require.Equal(t, 1, result.ValidRows)
	require.Equal(t, 1, result.ErrorRows)
	require.Len(t, result.Errors, 1)
	require.Equal(t, "not-a-valid-eui", result.Errors[0].GatewayEUI)
	require.Equal(t, "error", result.Errors[0].Outcome)
	require.NotEmpty(t, result.Errors[0].ErrorMessage)
}

// TestGatewayImport_Commit_Idempotent — re-running the same CSV twice results
// in all rows being "skipped" on the second run (same EUI, same data → no
// effective change is idempotent; we treat same-data update as skipped).
func TestGatewayImport_Commit_Idempotent(t *testing.T) {
	pool, _ := setupImportTest(t)
	ctx := context.Background()
	svc := makeImportSvc(pool)

	// Seed an admin user for the actorID.
	var actorID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('import-test@example.com', 'Import Test', 'x', 'admin') RETURNING id::text`,
	).Scan(&actorID))

	csv := csvWith([][]string{
		{"aabbccddeeff0011", "GW One", "", "13.0", "100.0", "as923_2"},
		{"aabbccddeeff0012", "GW Two", "", "", "", "eu868"},
		{"aabbccddeeff0013", "GW Three", "", "", "", "as923_2"},
	})

	// First commit: all 3 created.
	result1, err := svc.Commit(ctx, csv, actorID)
	require.NoError(t, err)
	require.Equal(t, 3, result1.Created)
	require.Equal(t, 0, result1.Updated)
	require.Equal(t, 0, result1.Skipped)

	// Second commit with exact same CSV: all 3 skipped (idempotent).
	result2, err := svc.Commit(ctx, csv, actorID)
	require.NoError(t, err)
	require.Equal(t, 0, result2.Created)
	require.Equal(t, 0, result2.Updated)
	require.Equal(t, 3, result2.Skipped)
}

// TestGatewayImport_Commit_PartialUpdate — same EUIs but different name field
// on re-run produces "updated" outcome (not skipped).
func TestGatewayImport_Commit_PartialUpdate(t *testing.T) {
	pool, _ := setupImportTest(t)
	ctx := context.Background()
	svc := makeImportSvc(pool)

	var actorID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('import-update@example.com', 'Import Update', 'x', 'admin') RETURNING id::text`,
	).Scan(&actorID))

	euiA := "aabbccddeeff0021"
	euiB := "aabbccddeeff0022"

	csv1 := csvWith([][]string{
		{euiA, "Original Name A", "", "", "", "as923_2"},
		{euiB, "Original Name B", "", "", "", "eu868"},
	})

	_, err := svc.Commit(ctx, csv1, actorID)
	require.NoError(t, err)

	// Re-run with changed names.
	csv2 := csvWith([][]string{
		{euiA, "Updated Name A", "", "", "", "as923_2"},
		{euiB, "Updated Name B", "", "", "", "eu868"},
	})

	result2, err := svc.Commit(ctx, csv2, actorID)
	require.NoError(t, err)
	require.Equal(t, 0, result2.Created)
	require.Equal(t, 2, result2.Updated)
	require.Equal(t, 0, result2.Skipped)
}

// TestGatewayImport_Commit_AuditPerRow — commit of N gateways writes N audit
// rows with action='gateway.bulk_imported'.
func TestGatewayImport_Commit_AuditPerRow(t *testing.T) {
	pool, _ := setupImportTest(t)
	ctx := context.Background()
	svc := makeImportSvc(pool)

	var actorID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('import-audit@example.com', 'Import Audit', 'x', 'admin') RETURNING id::text`,
	).Scan(&actorID))

	csv := csvWith([][]string{
		{"aabbccddeeff0031", "GW Audit A", "", "", "", "as923_2"},
		{"aabbccddeeff0032", "GW Audit B", "", "", "", "as923_2"},
		{"aabbccddeeff0033", "GW Audit C", "", "", "", "as923_2"},
	})

	result, err := svc.Commit(ctx, csv, actorID)
	require.NoError(t, err)
	require.Equal(t, 3, result.Created)

	// Assert exactly 3 audit rows with action='gateway.bulk_imported'.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = $1`,
		audit.AuditActionGatewayBulkImported,
	).Scan(&auditCount))
	require.Equal(t, 3, auditCount, "expected 1 audit row per imported gateway")
}
