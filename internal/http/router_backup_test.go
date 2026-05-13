package http

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/backup"
)

// noopLogger returns a logger that discards all output.
// Reused across test helpers in this package.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestBackupRoutes_UnauthenticatedGetsReject proves /api/backup/list is mounted
// with auth (BUG-02 regression guard): no session → 401.
func TestBackupRoutes_UnauthenticatedGetsReject(t *testing.T) {
	sm := auth.NewSessionManager(nil /*pool*/, true /*dev*/, 8*time.Hour, 24*time.Hour)
	deps := backup.Deps{
		Runner:     nil, // not reached — auth fires first
		Store:      backup.NewStore(nil),
		SessionMgr: sm,
		Log:        noopLogger(),
	}
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	backup.RegisterRoutes(r, deps)

	req := httptest.NewRequest(http.MethodGet, "/api/backup/list", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code,
		"unauthenticated GET /api/backup/list must return 401")
}
