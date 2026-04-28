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
