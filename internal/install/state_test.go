package install

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

func setupStore(t *testing.T) *Store {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	return NewStore(pool)
}

func TestInstallState_GetOrCreate_NewInstall(t *testing.T) {
	s := setupStore(t)
	st, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, st.CurrentStep)
	require.Nil(t, st.CompletedAt)
}

func TestInstallState_GetOrCreate_Reentrant(t *testing.T) {
	s := setupStore(t)
	_, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	_, err = s.GetOrCreate(context.Background())
	require.NoError(t, err)
}

func TestStep1_PersistsAdmin(t *testing.T) {
	s := setupStore(t)
	_, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	payload := []byte(`{"email":"alice@example.com","name":"Alice","password_hash":"$argon2id$..."}`)
	require.NoError(t, s.UpdateStep1(context.Background(), payload))
	st, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, st.CurrentStep, 2)
	require.Contains(t, string(st.Step1Admin), "alice")
}

func TestStep2_CapturesCS(t *testing.T) {
	s := setupStore(t)
	_, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	payload := []byte(`{"mode":"bundled","grpc_url":"chirpstack:8080","mqtt_url":"tcp://mosquitto:1883"}`)
	require.NoError(t, s.UpdateStep2(context.Background(), payload))
	st, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, st.CurrentStep, 3)
	require.Contains(t, string(st.Step2ChirpStack), "bundled")
}

func TestStep3_PersistsRegion(t *testing.T) {
	s := setupStore(t)
	_, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	payload := []byte(`{"name":"as923_2","common_name":"AS923_2"}`)
	require.NoError(t, s.UpdateStep3(context.Background(), payload))
	st, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, st.CurrentStep, 4)
	require.Contains(t, string(st.Step3Region), "AS923_2")
}

func TestStep4_PersistsIdentity(t *testing.T) {
	s := setupStore(t)
	_, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	payload := []byte(`{"display_name":"Acme","timezone":"Asia/Bangkok","units":"metric"}`)
	require.NoError(t, s.UpdateStep4(context.Background(), payload))
	st, err := s.GetOrCreate(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, st.CurrentStep, 5)
	require.Contains(t, string(st.Step4Identity), "Acme")
}

func TestRegions_HasThailand(t *testing.T) {
	found := false
	for _, r := range Regions() {
		if r.Name == "as923_2" && r.DefaultForCountry == "TH" {
			found = true
		}
	}
	require.True(t, found, "INST-04: Regions must include Thailand AS923-2 default")
}

// setupForFinish builds a minimal Deps backed by a freshly-migrated
// testcontainer pool. Used by FinishSetup tests + TestState_ReturnsGoneAfterFinish.
func setupForFinish(t *testing.T) (Deps, *Store) {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := NewStore(pool)
	deps := Deps{
		Pool:  pool,
		Store: store,
		Log:   slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	return deps, store
}

// seedAllFourSteps populates install_state with valid drafts for every step
// so FinishSetup has a complete wizard to commit. Used by FinishSetup tests
// and the post-finish 410 handler test.
func seedAllFourSteps(t *testing.T, store *Store) {
	t.Helper()
	require.NoError(t, store.UpdateStep1(context.Background(),
		[]byte(`{"email":"alice@example.com","name":"Alice","password_hash":"$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`)))
	require.NoError(t, store.UpdateStep2(context.Background(),
		[]byte(`{"mode":"bundled","grpc_url":"chirpstack:8080","api_token_ref":"/run/secrets/cs","mqtt_url":"tcp://mosquitto:1883"}`)))
	require.NoError(t, store.UpdateStep3(context.Background(),
		[]byte(`{"name":"as923_2","common_name":"AS923_2"}`)))
	require.NoError(t, store.UpdateStep4(context.Background(),
		[]byte(`{"display_name":"Acme","timezone":"Asia/Bangkok","units":"metric"}`)))
}

// TestFinishSetup_AtomicCommit — D-10: a full wizard ends in one transaction
// that creates the admin user, install_identity, chirpstack_connection rows
// and deletes install_state. After commit, GET /state returns 410 (D-11).
func TestFinishSetup_AtomicCommit(t *testing.T) {
	deps, store := setupForFinish(t)
	seedAllFourSteps(t, store)
	require.NoError(t, FinishSetup(context.Background(), deps))

	var n int
	require.NoError(t, deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "user" WHERE role='admin'`).Scan(&n))
	require.Equal(t, 1, n, "atomic commit must create admin user")

	require.NoError(t, deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM install_identity`).Scan(&n))
	require.Equal(t, 1, n, "atomic commit must create install_identity")

	require.NoError(t, deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM chirpstack_connection`).Scan(&n))
	require.Equal(t, 1, n, "atomic commit must create chirpstack_connection")

	require.NoError(t, deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM install_state`).Scan(&n))
	require.Equal(t, 0, n, "D-11: install_state row deleted after finish")
}

// TestFinishSetup_Idempotent — re-running FinishSetup after a successful commit
// must return ErrAlreadyCompleted (admin row already exists). The pre-check
// short-circuits before opening the txn so concurrent finishes never duplicate
// the admin row.
func TestFinishSetup_Idempotent(t *testing.T) {
	deps, store := setupForFinish(t)
	seedAllFourSteps(t, store)
	require.NoError(t, FinishSetup(context.Background(), deps))

	err := FinishSetup(context.Background(), deps)
	require.ErrorIs(t, err, ErrAlreadyCompleted)
}

// TestFinishSetup_Incomplete — when fewer than four step drafts are captured,
// FinishSetup returns ErrIncompleteWizard without opening a txn or writing
// any user / identity / connection rows.
func TestFinishSetup_Incomplete(t *testing.T) {
	deps, store := setupForFinish(t)
	require.NoError(t, store.UpdateStep1(context.Background(),
		[]byte(`{"email":"a@x.com","name":"b","password_hash":"x"}`)))
	err := FinishSetup(context.Background(), deps)
	require.ErrorIs(t, err, ErrIncompleteWizard)

	// Confirm nothing was written.
	var n int
	require.NoError(t, deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "user"`).Scan(&n))
	require.Equal(t, 0, n)
}

// TestFinishSetup_RollsBackOnFailure — if the txn fails mid-way (here we
// simulate by feeding step4 a bogus units enum value bypassing the handler),
// the entire transaction rolls back: no admin user is created.
func TestFinishSetup_RollsBackOnFailure(t *testing.T) {
	deps, store := setupForFinish(t)
	// Re-seed with an invalid units enum value — the handlers reject this
	// upstream, but FinishSetup is the last line of defence (D-10).
	seedAllFourSteps(t, store)
	require.NoError(t, store.UpdateStep4(context.Background(),
		[]byte(`{"display_name":"Acme","timezone":"Asia/Bangkok","units":"furlongs"}`)))

	err := FinishSetup(context.Background(), deps)
	require.Error(t, err, "invalid units enum must error inside the txn")

	// Crucially, the admin user must NOT exist — Serializable txn rolled back.
	var n int
	require.NoError(t, deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "user"`).Scan(&n))
	require.Equal(t, 0, n, "Serializable rollback must drop the partial admin insert")
}
