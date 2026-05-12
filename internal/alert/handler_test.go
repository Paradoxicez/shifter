// Package alert — HTTP handler tests for Plan 06-04. Uses the same
// testcontainer fixture pattern as internal/user/handler_test.go: stand up
// Postgres, run migrations, seed admin + viewer, mount routes behind a
// LoadAndSave-wrapped chi router with a /test/seed/{role} helper.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

type handlerFixture struct {
	pool       *pgxpool.Pool
	server     *httptest.Server
	client     *http.Client
	rules      *RuleStore
	alerts     *AlertStore
	adminID    string
	viewerID   string
	siteID     uuid.UUID
	meterID    uuid.UUID
}

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func setupHandlerFixture(t *testing.T) *handlerFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	// Seed install_identity (singleton) so payload builder has something.
	_, err := pool.Exec(ctx,
		`INSERT INTO install_identity (id, display_name, timezone, units)
		 VALUES (1, 'Handler Test', 'UTC', 'metric')
		 ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	store := auth.NewStore(pool)
	hash, err := auth.Hash("Strong-Pass-1234!")
	require.NoError(t, err)
	adminID, err := store.InsertAdminUser(ctx, "admin@example.com", "Admin", hash)
	require.NoError(t, err)
	var viewerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer@example.com', 'Viewer', $1, 'viewer') RETURNING id::text`,
		hash).Scan(&viewerID))

	// Seed site + metering point.
	var siteIDStr, mpIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('handler-site', 'UTC') RETURNING id`,
	).Scan(&siteIDStr))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'handler-mp', 'water') RETURNING id`, siteIDStr,
	).Scan(&mpIDStr))

	siteID, _ := uuid.Parse(siteIDStr)
	mpID, _ := uuid.Parse(mpIDStr)

	rules := NewRuleStore(pool)
	alerts := NewAlertStore(pool)
	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)

	deps := HTTPDeps{
		Pool:       pool,
		SessionMgr: sm,
		Rules:      rules,
		Alerts:     alerts,
		Log:        nopLogger(),
	}

	r := chi.NewRouter()
	r.Post("/test/seed/{role}", func(w http.ResponseWriter, req *http.Request) {
		role := chi.URLParam(req, "role")
		var id string
		switch role {
		case "admin":
			id = adminID
		case "viewer":
			id = viewerID
		default:
			http.Error(w, "bad role", 400)
			return
		}
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: id, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Mount the alert routes with RBAC.
	r.Route("/api/alerts", func(rt chi.Router) {
		rt.Group(func(g chi.Router) {
			g.Use(auth.RequireAction(sm, auth.ActionAlertRead))
			g.Get("/", ListHandler(deps))
			g.Get("/recent", RecentForDrawerHandler(deps))
			g.Get("/{id}", GetHandler(deps))
		})
		rt.Group(func(g chi.Router) {
			g.Use(auth.RequireAction(sm, auth.ActionAlertAck))
			g.Post("/{id}/ack", AckHandler(deps))
		})
		rt.Group(func(g chi.Router) {
			g.Use(auth.RequireAction(sm, auth.ActionAlertSnooze))
			g.Post("/{id}/snooze", SnoozeHandler(deps))
		})
		rt.Route("/rules", func(rr chi.Router) {
			rr.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(sm, auth.ActionAlertRead))
				g.Get("/", ListRulesHandler(deps))
			})
			rr.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(sm, auth.ActionAlertRuleCreate))
				g.Post("/", CreateRuleHandler(deps))
			})
			rr.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(sm, auth.ActionAlertRuleUpdate))
				g.Patch("/{id}", UpdateRuleHandler(deps))
			})
			rr.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(sm, auth.ActionAlertRuleDisable))
				g.Post("/{id}/disable", DisableRuleHandler(deps))
			})
			rr.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(sm, auth.ActionAlertRuleEnable))
				g.Post("/{id}/enable", EnableRuleHandler(deps))
			})
			rr.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(sm, auth.ActionAlertTestFire))
				g.Post("/{id}/test-fire", TestFireHandler(TestFireDeps{
					HTTPDeps: deps,
					// EnqueueClear=nil → test envs skip the river insert
				}))
			})
		})
	})
	r.Group(func(g chi.Router) {
		g.Use(auth.RequireAction(sm, auth.ActionAlertRead))
		g.Get("/api/anomaly-roster", RosterHandler(deps))
		g.Get("/api/metering-points/{id}/anomaly-state", MPAnomalyStateHandler(deps))
	})
	r.Group(func(g chi.Router) {
		g.Use(auth.RequireAction(sm, auth.ActionAlertRuleCreate))
		g.Patch("/api/metering-points/{id}/anomaly-rules/{kind}", ToggleMPAnomalyHandler(deps))
	})

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	return &handlerFixture{
		pool:     pool,
		server:   srv,
		client:   cli,
		rules:    rules,
		alerts:   alerts,
		adminID:  adminID,
		viewerID: viewerID,
		siteID:   siteID,
		meterID:  mpID,
	}
}

func (f *handlerFixture) seed(t *testing.T, role string) {
	t.Helper()
	resp, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "seed %s failed: %d", role, resp.StatusCode)
}

func (f *handlerFixture) doJSON(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.server.URL+path, rdr)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := f.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// seedFiringAlert inserts a single firing alert backed by a real rule and
// returns the (rule_id, alert_id) pair. Used by ack/snooze tests.
func (f *handlerFixture) seedFiringAlert(t *testing.T, severity string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	rule, err := f.rules.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:  "threshold_instantaneous",
		ScopeKind: "metering_point",
		ScopeID:   &f.meterID,
		Severity:  severity,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	tx, err = f.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	alert, err := f.alerts.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           rule.ID,
		RuleKind:         rule.RuleKind,
		Severity:         severity,
		Payload:          []byte(`{"x":1}`),
		TargetEntityType: "metering_point",
		TargetEntityID:   f.meterID,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return rule.ID, alert.ID
}

// ─────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────

// TestAlertList_OpenStatusDefault — GET /api/alerts returns firing alerts
// sorted by fired_at DESC + bell badge counts.
func TestAlertList_OpenStatusDefault(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")

	_, alertID := f.seedFiringAlert(t, "critical")

	status, body := f.doJSON(t, "GET", "/api/alerts", nil)
	require.Equal(t, http.StatusOK, status, string(body))
	var resp struct {
		Rows         []alertDTO   `json:"rows"`
		UnreadCounts unreadCounts `json:"unread_counts"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Len(t, resp.Rows, 1)
	require.Equal(t, alertID, resp.Rows[0].ID)
	require.Equal(t, int64(1), resp.UnreadCounts.Critical)
}

// TestAlertList_FilterChips — severity=warning filters out critical.
func TestAlertList_FilterChips(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")
	f.seedFiringAlert(t, "critical")
	_, warnID := f.seedFiringAlert(t, "warning")

	status, body := f.doJSON(t, "GET", "/api/alerts?severity=warning", nil)
	require.Equal(t, http.StatusOK, status, string(body))
	var resp struct {
		Rows []alertDTO `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Len(t, resp.Rows, 1)
	require.Equal(t, warnID, resp.Rows[0].ID)
}

// TestAlertAck_WritesAuditInTx — POST .../ack returns 200, alert is
// acknowledged, and audit_log has a matching alert.acknowledged row.
func TestAlertAck_WritesAuditInTx(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")
	_, alertID := f.seedFiringAlert(t, "critical")

	status, body := f.doJSON(t, "POST", "/api/alerts/"+alertID.String()+"/ack",
		map[string]string{"note": "checked floor 3"})
	require.Equal(t, http.StatusOK, status, string(body))

	// Alert row state.
	var state string
	var ackNote *string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT state, ack_note FROM alert WHERE id = $1`, alertID).Scan(&state, &ackNote))
	require.Equal(t, "acknowledged", state)
	require.NotNil(t, ackNote)
	require.Equal(t, "checked floor 3", *ackNote)

	// Audit row.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log
		 WHERE action = $1 AND entity_type = $2 AND entity_id = $3`,
		audit.ActionAlertAcked, audit.EntityTypeAlert, alertID).Scan(&auditCount))
	require.Equal(t, 1, auditCount, "expected exactly one acknowledged audit row")
}

// TestAlertSnooze_AcceptsPresets — body {"duration":"8h"} → state='snoozed',
// snoozed_until ~ now+8h.
func TestAlertSnooze_AcceptsPresets(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")
	_, alertID := f.seedFiringAlert(t, "warning")

	status, body := f.doJSON(t, "POST", "/api/alerts/"+alertID.String()+"/snooze",
		map[string]string{"duration": "8h"})
	require.Equal(t, http.StatusOK, status, string(body))

	var state string
	var snoozedUntil *time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT state, snoozed_until FROM alert WHERE id = $1`, alertID).Scan(&state, &snoozedUntil))
	require.Equal(t, "snoozed", state)
	require.NotNil(t, snoozedUntil)
	// snoozed_until ~ now+8h (within a 2 min buffer).
	diff := time.Until(*snoozedUntil)
	require.InDelta(t, (8 * time.Hour).Seconds(), diff.Seconds(), 120, "snoozed_until ~ 8h ahead")
}

// TestAlertSnooze_AcceptsMute — body {"duration":"mute"} → muted=true.
func TestAlertSnooze_AcceptsMute(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")
	_, alertID := f.seedFiringAlert(t, "info")

	status, body := f.doJSON(t, "POST", "/api/alerts/"+alertID.String()+"/snooze",
		map[string]string{"duration": "mute"})
	require.Equal(t, http.StatusOK, status, string(body))

	var muted bool
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT muted FROM alert WHERE id = $1`, alertID).Scan(&muted))
	require.True(t, muted, "muted should be true")
}

// TestAlertSnooze_RejectsInvalidDuration — body {"duration":"99y"} → 422.
func TestAlertSnooze_RejectsInvalidDuration(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")
	_, alertID := f.seedFiringAlert(t, "warning")

	status, _ := f.doJSON(t, "POST", "/api/alerts/"+alertID.String()+"/snooze",
		map[string]string{"duration": "99y"})
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

// TestRuleCreate_WritesAuditInTx — POST /api/alerts/rules creates the rule
// and writes audit 'alert.rule_create' in the SAME tx.
func TestRuleCreate_WritesAuditInTx(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")

	high := 100.0
	cmp := "gt"
	unit := "kWh"
	body := map[string]any{
		"rule_kind":  "threshold_instantaneous",
		"scope_kind": "metering_point",
		"scope_id":   f.meterID.String(),
		"high_bound": &high,
		"comparison": &cmp,
		"unit":       &unit,
		"severity":   "warning",
	}
	status, respBody := f.doJSON(t, "POST", "/api/alerts/rules", body)
	require.Equal(t, http.StatusCreated, status, string(respBody))
	var dto ruleDTO
	require.NoError(t, json.Unmarshal(respBody, &dto))

	// Audit row written in same tx.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log
		 WHERE action = $1 AND entity_type = $2 AND entity_id = $3`,
		audit.ActionAlertRuleCreate, audit.EntityTypeAlertRule, dto.ID).Scan(&auditCount))
	require.Equal(t, 1, auditCount)
}

// TestRuleCreate_RejectsScopeIDMismatch — scope_kind='global' with scope_id
// set → 422.
func TestRuleCreate_RejectsScopeIDMismatch(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")

	body := map[string]any{
		"rule_kind":  "threshold_instantaneous",
		"scope_kind": "global",
		"scope_id":   f.meterID.String(), // should be null for global
		"severity":   "warning",
	}
	status, _ := f.doJSON(t, "POST", "/api/alerts/rules", body)
	require.Equal(t, http.StatusUnprocessableEntity, status)

	// And the opposite — scope_kind=metering_point with no scope_id.
	body2 := map[string]any{
		"rule_kind":  "threshold_instantaneous",
		"scope_kind": "metering_point",
		"severity":   "warning",
	}
	status, _ = f.doJSON(t, "POST", "/api/alerts/rules", body2)
	require.Equal(t, http.StatusUnprocessableEntity, status)
}

// TestRuleDisable_AuditsAndPersists — POST .../disable writes audit row +
// disabled_at IS NOT NULL.
func TestRuleDisable_AuditsAndPersists(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")

	// Create a rule first.
	body := map[string]any{
		"rule_kind":  "threshold_instantaneous",
		"scope_kind": "metering_point",
		"scope_id":   f.meterID.String(),
		"severity":   "warning",
	}
	_, respBody := f.doJSON(t, "POST", "/api/alerts/rules", body)
	var dto ruleDTO
	require.NoError(t, json.Unmarshal(respBody, &dto))

	// Disable it.
	status, _ := f.doJSON(t, "POST", "/api/alerts/rules/"+dto.ID.String()+"/disable", nil)
	require.Equal(t, http.StatusOK, status)

	var disabledAt *time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT disabled_at FROM alert_rule WHERE id = $1`, dto.ID).Scan(&disabledAt))
	require.NotNil(t, disabledAt)

	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log
		 WHERE action = $1 AND entity_id = $2`,
		audit.ActionAlertRuleDisable, dto.ID).Scan(&auditCount))
	require.Equal(t, 1, auditCount)
}

// TestAnomalyRoster_AdminAndViewerBothAllowed — both roles can GET the
// roster. D-11 viewer read.
func TestAnomalyRoster_AdminAndViewerBothAllowed(t *testing.T) {
	f := setupHandlerFixture(t)

	f.seed(t, "admin")
	status, _ := f.doJSON(t, "GET", "/api/anomaly-roster", nil)
	require.Equal(t, http.StatusOK, status)

	f.seed(t, "viewer")
	status, _ = f.doJSON(t, "GET", "/api/anomaly-roster", nil)
	require.Equal(t, http.StatusOK, status)
}

// TestMPAnomalyState_PatchToggle — PATCH .../anomaly-rules/p95 with
// {enabled:true} creates an mp-scoped anomaly_p95 rule + audit row.
func TestMPAnomalyState_PatchToggle(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")

	path := fmt.Sprintf("/api/metering-points/%s/anomaly-rules/p95", f.meterID.String())
	status, body := f.doJSON(t, "PATCH", path, map[string]bool{"enabled": true})
	require.Equal(t, http.StatusOK, status, string(body))

	var ruleCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alert_rule
		 WHERE rule_kind = 'anomaly_p95' AND scope_kind = 'metering_point' AND scope_id = $1`,
		f.meterID).Scan(&ruleCount))
	require.Equal(t, 1, ruleCount)

	// Disable + ensure disabled_at is set.
	status, _ = f.doJSON(t, "PATCH", path, map[string]bool{"enabled": false})
	require.Equal(t, http.StatusOK, status)

	var disabledAt *time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT disabled_at FROM alert_rule
		 WHERE rule_kind = 'anomaly_p95' AND scope_kind = 'metering_point' AND scope_id = $1`,
		f.meterID).Scan(&disabledAt))
	require.NotNil(t, disabledAt)
}

// TestTestFire_CreatesSyntheticAlert — POST .../test-fire creates an
// is_test=true info-severity alert + audit 'alert.test_fired' row.
func TestTestFire_CreatesSyntheticAlert(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")

	// Create a rule first.
	body := map[string]any{
		"rule_kind":  "threshold_instantaneous",
		"scope_kind": "metering_point",
		"scope_id":   f.meterID.String(),
		"severity":   "warning",
	}
	_, respBody := f.doJSON(t, "POST", "/api/alerts/rules", body)
	var rule ruleDTO
	require.NoError(t, json.Unmarshal(respBody, &rule))

	// Test-fire it.
	status, fireBody := f.doJSON(t, "POST", "/api/alerts/rules/"+rule.ID.String()+"/test-fire", nil)
	require.Equal(t, http.StatusOK, status, string(fireBody))
	var fired alertDTO
	require.NoError(t, json.Unmarshal(fireBody, &fired))
	require.True(t, fired.IsTest)
	require.Equal(t, "info", fired.Severity)

	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log
		 WHERE action = $1 AND entity_id = $2`,
		audit.ActionAlertTestFired, fired.ID).Scan(&auditCount))
	require.Equal(t, 1, auditCount)
}

// TestTestFire_DoesNotTouchLastFiredAt — Pitfall 8 mitigation: test-fires
// must not start the cooldown timer.
func TestTestFire_DoesNotTouchLastFiredAt(t *testing.T) {
	f := setupHandlerFixture(t)
	f.seed(t, "admin")

	body := map[string]any{
		"rule_kind":  "threshold_instantaneous",
		"scope_kind": "metering_point",
		"scope_id":   f.meterID.String(),
		"severity":   "warning",
	}
	_, respBody := f.doJSON(t, "POST", "/api/alerts/rules", body)
	var rule ruleDTO
	require.NoError(t, json.Unmarshal(respBody, &rule))

	// Snapshot last_fired_at (should be NULL).
	var beforeLast *time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT last_fired_at FROM alert_rule WHERE id = $1`, rule.ID).Scan(&beforeLast))
	require.Nil(t, beforeLast, "fresh rule must have NULL last_fired_at")

	// Fire the test.
	status, _ := f.doJSON(t, "POST", "/api/alerts/rules/"+rule.ID.String()+"/test-fire", nil)
	require.Equal(t, http.StatusOK, status)

	// last_fired_at MUST still be NULL — Pitfall 8.
	var afterLast *time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT last_fired_at FROM alert_rule WHERE id = $1`, rule.ID).Scan(&afterLast))
	require.Nil(t, afterLast, "test-fire must NOT set last_fired_at (Pitfall 8)")
}

// TestAlertViewer_ReadOnlyEnforced — viewer can list/get but ack/snooze 403.
func TestAlertViewer_ReadOnlyEnforced(t *testing.T) {
	f := setupHandlerFixture(t)
	_, alertID := f.seedFiringAlert(t, "warning")

	f.seed(t, "viewer")

	// GET /api/alerts works.
	status, _ := f.doJSON(t, "GET", "/api/alerts", nil)
	require.Equal(t, http.StatusOK, status)

	// POST /ack 403s.
	status, _ = f.doJSON(t, "POST", "/api/alerts/"+alertID.String()+"/ack",
		map[string]string{"note": "viewer attempt"})
	require.Equal(t, http.StatusForbidden, status)

	// POST /snooze 403s.
	status, _ = f.doJSON(t, "POST", "/api/alerts/"+alertID.String()+"/snooze",
		map[string]string{"duration": "1h"})
	require.Equal(t, http.StatusForbidden, status)
}
