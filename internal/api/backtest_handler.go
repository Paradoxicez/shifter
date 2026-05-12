// Package api — backtest HTTP handler (Plan 07-10).
//
// POST /api/alerts/backtest runs a read-only backtest of an anomaly rule
// against measurement_hourly CAGG data. It NEVER writes to the alert table
// (T-07-10-04 mitigation). RBAC: ActionAlertRuleCreate (admin only).
package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/alert"
)

// BacktestDeps groups the dependencies for the backtest handler.
type BacktestDeps struct {
	Pool *pgxpool.Pool
}

// backtestRequest is the JSON body for POST /api/alerts/backtest.
type backtestRequest struct {
	RuleKind string    `json:"rule_kind"`
	MpID     uuid.UUID `json:"mp_id"`
	Days     int       `json:"days"`
}

// BacktestHandler returns an http.HandlerFunc for POST /api/alerts/backtest.
//
// Body: { "rule_kind": "anomaly_p95", "mp_id": "<uuid>", "days": 30 }
// Response: BacktestResult JSON  { "fires_count": N, "daily_fires": [...] }
//
// Security: caller must hold ActionAlertRuleCreate (enforced at router layer).
// This handler is read-only — it calls BacktestRun which never writes alerts.
func BacktestHandler(deps BacktestDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req backtestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Default days to 30 when omitted.
		if req.Days == 0 {
			req.Days = 30
		}

		result, err := alert.BacktestRun(r.Context(), deps.Pool, req.RuleKind, req.MpID, req.Days)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			// Headers already sent — nothing recoverable.
			return
		}
	}
}
