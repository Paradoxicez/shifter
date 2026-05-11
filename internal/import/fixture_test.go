package importpkg

// Phase 3 Plan 03-05 — shared test bench for dryrun / commit / job_ttl
// integration tests. Spins up a testcontainer Postgres + applies all
// migrations + seeds a synced device profile + a site, mirroring the
// internal/device/handlers_test.go layout.

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// importFixture is the shared bench. Tests that don't need CS / commit only
// touch the read-only Queries via fixture.q.
type importFixture struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries

	cs   *fakeImportCS
	boot *fakeImportBootstrap

	adminID   uuid.UUID
	siteID    uuid.UUID
	siteName  string
	profileID uuid.UUID
}

func newImportFixture(t *testing.T) *importFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, discardLogger()))

	var adminID, siteID, profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-import@example.com', 'Admin Import', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))

	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('site-A', 'Asia/Bangkok') RETURNING id::text`,
	).Scan(&siteID))

	// Patch the seeded axioma_w1 profile with a fake cs_profile_id so the
	// commit path passes the "profile synced to CS" check.
	require.NoError(t, pool.QueryRow(ctx,
		`UPDATE device_profile SET cs_profile_id = $1::uuid, codec_js_synced_at = now()
		 WHERE slug = 'axioma_w1' RETURNING id::text`, uuid.NewString(),
	).Scan(&profileID))

	return &importFixture{
		pool:      pool,
		q:         sqlc.New(pool),
		cs:        newFakeImportCS(),
		boot:      &fakeImportBootstrap{tenantID: uuid.NewString(), appID: uuid.NewString()},
		adminID:   uuid.MustParse(adminID),
		siteID:    uuid.MustParse(siteID),
		siteName:  "site-A",
		profileID: uuid.MustParse(profileID),
	}
}

// seedDevice inserts a Shifter device row directly (no CS call). Used to
// exercise the already_exists outcome path in dryrun.
func (f *importFixture) seedDevice(t *testing.T, devEUI string) {
	t.Helper()
	ctx := context.Background()
	_, err := f.pool.Exec(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ($1, $2, $3)`,
		devEUI, "preexisting", f.profileID,
	)
	require.NoError(t, err)
}

// fakeImportCS — narrow CS contract for commit tests. Per-call error
// injection is FIFO.
type fakeImportCS struct {
	mu sync.Mutex

	createCalls   atomic.Int64
	keysCalls     atomic.Int64
	activateCalls atomic.Int64
	deleteCalls   atomic.Int64

	createErrs []error
	keysErrs   []error
	activeErrs []error
	deleteErrs []error

	createdEUIs []string
	deletedEUIs []string
}

func newFakeImportCS() *fakeImportCS {
	return &fakeImportCS{}
}

func (f *fakeImportCS) CreateDevice(_ context.Context, in chirpstack.CreateDeviceInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if len(f.createErrs) > 0 {
		err := f.createErrs[0]
		f.createErrs = f.createErrs[1:]
		if err != nil {
			return err
		}
	}
	f.createdEUIs = append(f.createdEUIs, in.DevEUI)
	return nil
}

func (f *fakeImportCS) CreateDeviceKeys(_ context.Context, _ string, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keysCalls.Add(1)
	if len(f.keysErrs) > 0 {
		err := f.keysErrs[0]
		f.keysErrs = f.keysErrs[1:]
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeImportCS) ActivateDevice(_ context.Context, _ chirpstack.ActivateDeviceInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activateCalls.Add(1)
	if len(f.activeErrs) > 0 {
		err := f.activeErrs[0]
		f.activeErrs = f.activeErrs[1:]
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeImportCS) DeleteDevice(_ context.Context, devEUI string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls.Add(1)
	f.deletedEUIs = append(f.deletedEUIs, devEUI)
	if len(f.deleteErrs) > 0 {
		err := f.deleteErrs[0]
		f.deleteErrs = f.deleteErrs[1:]
		return err
	}
	return nil
}

type fakeImportBootstrap struct {
	tenantID string
	appID    string
	err      error
}

func (b *fakeImportBootstrap) EnsureTenantAndApplication(_ context.Context) (string, string, error) {
	if b.err != nil {
		return "", "", b.err
	}
	return b.tenantID, b.appID, nil
}

// rowsFromHappy parses the HappyFiveRows fixture, sets every row's site_id
// to the fixture's seeded site, returns the slice.
func rowsFromFixture(t *testing.T, f *importFixture, xlsx []byte) []ParsedRow {
	t.Helper()
	rows, err := ParseXLSX(bytesReader(xlsx))
	require.NoError(t, err)
	for i := range rows {
		// testsupport fixtures set site_id="site-A" which the dryrun
		// resolves via siteByName. No rewriting needed.
		_ = i
	}
	return rows
}

// buildBigXLSX returns an XLSX with the canonical header + n data rows.
// Used by upload-row-cap tests; the content of each row doesn't matter
// because the parser counts rows before the dry-run consumes them.
func buildBigXLSX(t *testing.T, n int) []byte {
	t.Helper()
	headers := []string{
		"dev_eui", "name", "device_profile", "site_id", "activation_mode",
		"join_eui", "app_key", "dev_addr", "f_cnt_up", "f_cnt_down",
	}
	var buf bytes.Buffer
	buf.WriteString(strings.Join(headers, ",") + "\n")
	// Each data row is a single comma-separated line; the parser counts by
	// row, not by content validity. We use CSV here because building a
	// 5001-row XLSX via excelize would be slow; the row-cap check fires the
	// same way for both formats.
	for i := 0; i < n; i++ {
		buf.WriteString("70b3d59999000000,d,axioma_w1,site-A,OTAA,0000000000000000,00112233445566778899aabbccddeeff,,,\n")
	}
	return buf.Bytes()
}
