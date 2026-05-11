package device

// Phase 3 Plan 03-06 / Task 2 — Devices list (D-12..D-18) integration tests.
//
// These tests stand up the same testcontainer Postgres + httptest server
// fixture as handlers_test.go (newDeviceFixture) and exercise the new
// server-side filter/sort/page handler end-to-end. Each test seeds a small
// set of devices (mostly via direct INSERT to avoid CS bootstrapping per
// row) and asserts the response envelope + row count.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// seedDevice inserts a device row directly (skipping the atomic CS+PG add
// flow) so list-filter tests can fabricate device fleets cheaply. last_seen
// is optional; pass time.Time{} for "never joined".
func seedDevice(t *testing.T, f *deviceFixture, devEUI, name string, lastSeen time.Time) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if lastSeen.IsZero() {
		require.NoError(t, f.pool.QueryRow(ctx,
			`INSERT INTO device (dev_eui, name, device_profile_id)
			 VALUES ($1, $2, $3::uuid) RETURNING id::text`,
			devEUI, name, f.profileID,
		).Scan(&id))
	} else {
		require.NoError(t, f.pool.QueryRow(ctx,
			`INSERT INTO device (dev_eui, name, device_profile_id, last_seen_at)
			 VALUES ($1, $2, $3::uuid, $4) RETURNING id::text`,
			devEUI, name, f.profileID, lastSeen,
		).Scan(&id))
	}
	return id
}

// seedSite inserts a site row and returns its id.
func seedSite(t *testing.T, f *deviceFixture, name string) string {
	t.Helper()
	var id string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO site (name, timezone) VALUES ($1, 'Asia/Bangkok') RETURNING id::text`,
		name,
	).Scan(&id))
	return id
}

// seedMP creates a metering point under siteID and returns its id.
func seedMP(t *testing.T, f *deviceFixture, siteID, name string) string {
	t.Helper()
	var id string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1::uuid, $2, 'water') RETURNING id::text`, siteID, name,
	).Scan(&id))
	return id
}

// bindDeviceToMP opens an active binding device→MP at now().
func bindDeviceToMP(t *testing.T, f *deviceFixture, deviceID, mpID string) {
	t.Helper()
	_, err := f.pool.Exec(context.Background(),
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1::uuid, $2::uuid, now(), 0)`, mpID, deviceID,
	)
	require.NoError(t, err)
}

// listEnvelope is the shape we expect from GET /api/devices on Phase 3.
type listEnvelope struct {
	TotalCount int64                    `json:"total_count"`
	PageCount  int32                    `json:"page_count"`
	Page       int32                    `json:"page"`
	PerPage    int32                    `json:"per_page"`
	Rows       []map[string]any         `json:"rows"`
}

func decodeListEnvelope(t *testing.T, res *http.Response) listEnvelope {
	t.Helper()
	var env listEnvelope
	require.NoError(t, json.NewDecoder(res.Body).Decode(&env))
	return env
}

// TestListDevicesFiltered_Site — seeds 3 devices, each bound to its own
// site; ?site=siteA returns 1, ?site=A&site=B returns 2.
func TestListDevicesFiltered_Site(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	siteA := seedSite(t, f, "Site A")
	siteB := seedSite(t, f, "Site B")
	siteC := seedSite(t, f, "Site C")
	mpA := seedMP(t, f, siteA, "MP A")
	mpB := seedMP(t, f, siteB, "MP B")
	mpC := seedMP(t, f, siteC, "MP C")

	devA := seedDevice(t, f, "aaaaaaaaaaaaaaa1", "dev-A", time.Now().UTC())
	devB := seedDevice(t, f, "aaaaaaaaaaaaaaa2", "dev-B", time.Now().UTC())
	devC := seedDevice(t, f, "aaaaaaaaaaaaaaa3", "dev-C", time.Now().UTC())
	bindDeviceToMP(t, f, devA, mpA)
	bindDeviceToMP(t, f, devB, mpB)
	bindDeviceToMP(t, f, devC, mpC)

	// Single-site filter.
	res := f.doJSON(t, "GET", "/api/devices?site="+siteA, nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.EqualValues(t, 1, env.TotalCount)
	require.Equal(t, "dev-A", env.Rows[0]["name"])

	// Multi-site filter.
	res2 := f.doJSON(t, "GET", "/api/devices?site="+siteA+"&site="+siteB, nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)
	env2 := decodeListEnvelope(t, res2)
	require.EqualValues(t, 2, env2.TotalCount)
}

// TestListDevicesFiltered_SiteMulti — explicit multi-select check that
// repeated ?site=... query params are AND-aggregated into one IN-list.
func TestListDevicesFiltered_SiteMulti(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	siteA := seedSite(t, f, "Multi-A")
	siteB := seedSite(t, f, "Multi-B")
	mpA := seedMP(t, f, siteA, "MP A")
	mpB := seedMP(t, f, siteB, "MP B")
	devA := seedDevice(t, f, "bbbbbbbbbbbbbbb1", "dev-MA", time.Now().UTC())
	devB := seedDevice(t, f, "bbbbbbbbbbbbbbb2", "dev-MB", time.Now().UTC())
	// Decoy unrelated device on no site.
	seedDevice(t, f, "bbbbbbbbbbbbbbb3", "dev-orphan", time.Now().UTC())
	bindDeviceToMP(t, f, devA, mpA)
	bindDeviceToMP(t, f, devB, mpB)

	res := f.doJSON(t, "GET", "/api/devices?site="+siteA+"&site="+siteB, nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.EqualValues(t, 2, env.TotalCount)
}

// TestListDevicesFiltered_Status — active/inactive/never_joined branches.
func TestListDevicesFiltered_Status(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	now := time.Now().UTC()
	seedDevice(t, f, "cccccccccccccc01", "active-dev", now.Add(-30*time.Minute))   // active
	seedDevice(t, f, "cccccccccccccc02", "inactive-dev", now.Add(-72*time.Hour))   // inactive
	seedDevice(t, f, "cccccccccccccc03", "never-joined-dev", time.Time{})          // never_joined

	for _, tc := range []struct {
		status     string
		wantTotal  int64
		wantName   string
	}{
		{"active", 1, "active-dev"},
		{"inactive", 1, "inactive-dev"},
		{"never_joined", 1, "never-joined-dev"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			res := f.doJSON(t, "GET", "/api/devices?status="+tc.status, nil)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)
			env := decodeListEnvelope(t, res)
			require.EqualValues(t, tc.wantTotal, env.TotalCount)
			require.Equal(t, tc.wantName, env.Rows[0]["name"])
		})
	}
}

// TestListDevicesFiltered_LastSeen — ?last_seen=24h cutoff includes recent
// uplinks only.
func TestListDevicesFiltered_LastSeen(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	now := time.Now().UTC()
	seedDevice(t, f, "dddddddddddddd01", "recent", now.Add(-1*time.Hour))
	seedDevice(t, f, "dddddddddddddd02", "old", now.Add(-72*time.Hour))

	res := f.doJSON(t, "GET", "/api/devices?last_seen=24h", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.EqualValues(t, 1, env.TotalCount)
	require.Equal(t, "recent", env.Rows[0]["name"])
}

// TestListDevicesFiltered_TextSearch — ?q matches name substring OR dev_eui
// substring (case-insensitive). dev_eui is constrained to lowercase 16-hex by
// the schema CHECK, so we can only test dev_eui substring via hex characters.
func TestListDevicesFiltered_TextSearch(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	// Name-substring branch.
	seedDevice(t, f, "ee00000000000001", "meter-1A", time.Now().UTC())
	seedDevice(t, f, "ee00000000000002", "meter-2B", time.Now().UTC())
	// dev_eui-substring branch — schema CHECK requires hex16, so the
	// substring must be hex too. Use "beef" as the marker that appears in
	// the dev_eui but NOT in the name.
	seedDevice(t, f, "0000000000beef00", "vendor-x", time.Now().UTC())

	// Name search: "meter" matches "meter-1A" + "meter-2B" → 2 rows.
	res := f.doJSON(t, "GET", "/api/devices?q=meter", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.EqualValues(t, 2, env.TotalCount, "name ILIKE %%meter%% matches 'meter-1A' and 'meter-2B'")

	// dev_eui search: "beef" matches the third device by dev_eui substring.
	res2 := f.doJSON(t, "GET", "/api/devices?q=beef", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)
	env2 := decodeListEnvelope(t, res2)
	require.EqualValues(t, 1, env2.TotalCount, "dev_eui ILIKE %%beef%% matches the third device")
	require.Equal(t, "vendor-x", env2.Rows[0]["name"])
}

// TestListDevicesSorted — sort=name asc/desc.
func TestListDevicesSorted(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	seedDevice(t, f, "ff00000000000001", "alpha", time.Now().UTC())
	seedDevice(t, f, "ff00000000000002", "bravo", time.Now().UTC())
	seedDevice(t, f, "ff00000000000003", "charlie", time.Now().UTC())

	res := f.doJSON(t, "GET", "/api/devices?sort=name", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.Len(t, env.Rows, 3)
	require.Equal(t, "alpha", env.Rows[0]["name"])
	require.Equal(t, "charlie", env.Rows[2]["name"])

	res2 := f.doJSON(t, "GET", "/api/devices?sort=-name", nil)
	defer res2.Body.Close()
	env2 := decodeListEnvelope(t, res2)
	require.Equal(t, "charlie", env2.Rows[0]["name"])
	require.Equal(t, "alpha", env2.Rows[2]["name"])
}

// TestListDevicesPaginated — 75 devices seeded, per_page=25&page=2 returns
// rows 26-50 + page_count=3 + total_count=75.
func TestListDevicesPaginated(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	for i := 0; i < 75; i++ {
		seedDevice(t, f, fmt.Sprintf("aaaa%012d", i),
			fmt.Sprintf("dev-%03d", i), time.Now().UTC().Add(-time.Duration(i)*time.Minute))
	}

	res := f.doJSON(t, "GET", "/api/devices?per_page=25&page=2&sort=name", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.EqualValues(t, 75, env.TotalCount)
	require.EqualValues(t, 3, env.PageCount)
	require.EqualValues(t, 2, env.Page)
	require.EqualValues(t, 25, env.PerPage)
	require.Len(t, env.Rows, 25)
	// Page 2 of name-sorted = "dev-025" .. "dev-049".
	require.Equal(t, "dev-025", env.Rows[0]["name"])
	require.Equal(t, "dev-049", env.Rows[24]["name"])
}

// TestListDevices_MaxPerPage — per_page=500 rejected with 400.
func TestListDevices_MaxPerPage(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")
	res := f.doJSON(t, "GET", "/api/devices?per_page=500", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "invalid_per_page", body["error"])
}

// TestListDevices_DefaultsApplied — no params → per_page=50, page=1.
func TestListDevices_DefaultsApplied(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")
	seedDevice(t, f, "abcdef0000000001", "default-dev", time.Now().UTC())

	res := f.doJSON(t, "GET", "/api/devices", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.EqualValues(t, 50, env.PerPage)
	require.EqualValues(t, 1, env.Page)
	require.GreaterOrEqual(t, env.TotalCount, int64(1))
}

// TestListDevices_DEV09_NoKeysInRows — DEV-09 structural invariant: the
// list response NEVER includes app_key / nwk_key / app_s_key / nwk_s_key.
// The device schema has no such columns; the JSON projection is fixed.
// This is a regression test in case a future patch accidentally adds those
// columns to the SELECT or the JSON envelope.
func TestListDevices_DEV09_NoKeysInRows(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")
	seedDevice(t, f, "abcdef0000000099", "no-keys-here", time.Now().UTC())

	res := f.doJSON(t, "GET", "/api/devices", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.NotEmpty(t, env.Rows)
	for _, row := range env.Rows {
		for _, banned := range []string{"app_key", "nwk_key", "app_s_key", "nwk_s_key"} {
			_, exists := row[banned]
			require.Falsef(t, exists, "DEV-09: list row must not include %q (row=%v)", banned, row)
		}
	}
}

// TestListDevicesFiltered_ViewerCanRead — viewer role can read the filtered
// list (read action ActionDeviceRead).
func TestListDevicesFiltered_ViewerCanRead(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")
	seedDevice(t, f, "abcdef0000000088", "viewer-ok", time.Now().UTC())
	f.seedRole(t, "viewer")
	res := f.doJSON(t, "GET", "/api/devices", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	env := decodeListEnvelope(t, res)
	require.GreaterOrEqual(t, env.TotalCount, int64(1))
}

// silence unused-imports if a helper is removed later.
var _ = strings.TrimSpace
