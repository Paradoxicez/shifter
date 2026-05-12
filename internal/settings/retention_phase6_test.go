package settings

import (
	"context"
	"net/http"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
)

// TestGetRetention_IncludesPhase6Fields verifies that GET /api/settings/retention
// returns alerts_days and audit_log_days with migration 0040 defaults.
func TestGetRetention_IncludesPhase6Fields(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)

	adminCookie := injectSession(t, sm, uuid.New().String(), "admin")
	w := doReq(t, router, http.MethodGet, "/api/settings/retention", nil, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RetentionResponse
	decodeJSON(t, w, &resp)

	// Migration 0040 defaults: alerts_days=365, audit_log_days=1825.
	if resp.AlertsDays != 365 {
		t.Errorf("alerts_days: want 365, got %d", resp.AlertsDays)
	}
	if resp.AuditLogDays != 1825 {
		t.Errorf("audit_log_days: want 1825, got %d", resp.AuditLogDays)
	}
}

// TestPatchRetention_UpdatesAlertsDays verifies PATCH with alerts_days=730 updates the row.
func TestPatchRetention_UpdatesAlertsDays(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	adminCookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, router, http.MethodPatch, "/api/settings/retention",
		map[string]any{"alerts_days": 730}, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RetentionResponse
	decodeJSON(t, w, &resp)
	if resp.AlertsDays != 730 {
		t.Errorf("alerts_days after PATCH: want 730, got %d", resp.AlertsDays)
	}
}

// TestPatchRetention_RejectsOutOfRange verifies PATCH with out-of-range alerts_days returns 422.
func TestPatchRetention_RejectsOutOfRange(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)

	adminCookie := injectSession(t, sm, uuid.New().String(), "admin")

	cases := []struct {
		name    string
		body    map[string]any
		wantErr string
	}{
		{"alerts_too_low", map[string]any{"alerts_days": 10}, "alerts_days_out_of_range"},
		{"alerts_too_high", map[string]any{"alerts_days": 10000}, "alerts_days_out_of_range"},
		{"audit_too_low", map[string]any{"audit_log_days": 30}, "audit_log_days_out_of_range"},
		{"audit_too_high", map[string]any{"audit_log_days": 20000}, "audit_log_days_out_of_range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doReq(t, router, http.MethodPatch, "/api/settings/retention", tc.body, adminCookie)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
			}
			var errResp map[string]string
			decodeJSON(t, w, &errResp)
			if errResp["error"] != tc.wantErr {
				t.Errorf("want error %q, got %q", tc.wantErr, errResp["error"])
			}
		})
	}
}

// TestPatchRetention_UpdatesBothInSameTx verifies that both alerts_days and
// audit_log_days can be updated atomically in one PATCH request.
func TestPatchRetention_UpdatesBothInSameTx(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	adminCookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, router, http.MethodPatch, "/api/settings/retention",
		map[string]any{"alerts_days": 730, "audit_log_days": 730}, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RetentionResponse
	decodeJSON(t, w, &resp)
	if resp.AlertsDays != 730 {
		t.Errorf("alerts_days: want 730, got %d", resp.AlertsDays)
	}
	if resp.AuditLogDays != 730 {
		t.Errorf("audit_log_days: want 730, got %d", resp.AuditLogDays)
	}
}

// TestPatchRetention_AuditRow verifies that patching alerts_days writes an
// audit row with action=settings.retention_change containing the changed fields.
func TestPatchRetention_AuditRow(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	adminCookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, router, http.MethodPatch, "/api/settings/retention",
		map[string]any{"alerts_days": 500}, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify an audit row was written.
	var count int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log
		 WHERE action = 'settings.retention_change'`).Scan(&count)
	if err != nil {
		t.Fatalf("query audit_log: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one settings.retention_change audit entry")
	}

	// Also confirm the response contains the updated value.
	var resp RetentionResponse
	decodeJSON(t, w, &resp)
	if resp.AlertsDays != 500 {
		t.Errorf("alerts_days after PATCH: want 500, got %d", resp.AlertsDays)
	}
}
