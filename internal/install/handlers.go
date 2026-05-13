// Package install — wizard step handlers (state, step/1..4, finish).
//
// The HTTP handlers in this file are mounted by Plan 18's chi router behind
// the FirstRunGate middleware (Plan 14). They are anonymous-accessible at
// install time (no admin user yet) and become 410 Gone post-finish via the
// gate's whitelist + per-handler completion check.
//
// Per-step contracts:
//
//	GET  /api/install/state    → 200 + State JSON | 410 install_completed
//	POST /api/install/step/1   → 200 next_step=2  | 422 weak_password
//	POST /api/install/step/2   → 200 next_step=3  | 422 v3_detected | grpc_unreachable
//	POST /api/install/step/3   → 200 next_step=4  | 422 unknown_region
//	POST /api/install/step/4   → 200 next_step=5  | 422 invalid_timezone | invalid_units
//	POST /api/install/finish   → 200 ok           | 410 install_completed | 422 incomplete_wizard
//
// Every state-changing POST requires `X-Requested-With: shifter` (RESEARCH §V13
// CSRF guard, symmetric with Plan 09's auth handlers; SameSite=Lax cookies
// don't exist pre-install, so the header check is the canonical guard).
//
// Step 2's API token is written to {SecretsDir}/chirpstack_api_token (mode
// 0600) and only the path REF is persisted into install_state.step2_chirpstack
// — never the raw token (T-15-02 / RESEARCH §Open Question 1).
package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/config"
)

// Deps is the wizard-handler dependency bundle. Plan 18 constructs one Deps
// at startup and passes it to every handler factory. Test code overrides Dial
// to inject the bufconn-backed ChirpStack mock.
type Deps struct {
	Pool       *pgxpool.Pool
	Store      *Store
	SecretsDir string // e.g. /run/secrets in container, ./secrets in dev
	Log        *slog.Logger
	// Dial is overridable in tests so we can plug in the bufconn mock.
	// Plan 18 wires a production csDialer that wraps chirpstack.Dial.
	Dial func(ctx context.Context, cfg config.CSConfig) (csConn, error)
}

// csConn is the minimum surface Step2Handler needs from a ChirpStack-bound
// *grpc.ClientConn. Tightened per RESEARCH Warning #6: exposes Conn() directly
// instead of round-tripping through interface{}. Plan 18's csConnWrapper
// implementation satisfies both this interface AND serve.go's csBootConn so a
// single wrapper covers production and test paths without `interface{}` indirection.
type csConn interface {
	Conn() *grpc.ClientConn
	Close() error
}

// CSConn is an exported alias for the package-private csConn interface so
// Plan 18's serve.go can construct a single wrapper that satisfies both this
// interface AND the boot-probe csBootConn shape — see internal/cli/serve.go's
// csConnWrapper for the production wiring.
type CSConn = csConn

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// stateWireResponse is the JSON shape returned by GET /api/install/state.
// It is identical to State except Step1Admin is sanitized to omit
// password_hash before it reaches the client (T-15-04).
type stateWireResponse struct {
	StartedAt       time.Time       `json:"started_at"`
	CompletedAt     *time.Time      `json:"completed_at"`
	CurrentStep     int             `json:"current_step"`
	Step1Admin      json.RawMessage `json:"step1_admin"`
	Step2ChirpStack json.RawMessage `json:"step2_chirpstack"`
	Step3Region     json.RawMessage `json:"step3_region"`
	Step4Identity   json.RawMessage `json:"step4_identity"`
}

// redactStep1Admin strips password_hash from the step1_admin JSONB blob
// before it is sent to the browser. The DB retains the hash for FinishSetup.
// Returns nil if the input is nil or unparseable (graceful degradation).
func redactStep1Admin(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw // unparseable blob — pass through untouched
	}
	delete(m, "password_hash")
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// ensureCSRF returns true when the request carries the canonical CSRF guard
// header. Plan 06's apiFetch sends `X-Requested-With: shifter` on every state-
// changing request; combined with SameSite=Lax cookies (Plan 08), this defeats
// classic cross-site form CSRF without per-request token plumbing.
//
// At install time there is no session cookie yet, so the header check is the
// SOLE CSRF guard for these endpoints (T-15-03 / RESEARCH §V13).
func ensureCSRF(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("X-Requested-With"), "shifter")
}

// StateHandler — GET /api/install/state.
//
// Returns the singleton install_state row (created on first call). When an
// admin user already exists, returns 410 Gone — the wizard is no longer
// reachable (D-11, symmetric with FirstRunGate's whitelist).
func StateHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if completed, _ := isCompleted(r.Context(), deps.Pool); completed {
			writeJSON(w, http.StatusGone, map[string]string{"error": "install_completed"})
			return
		}
		st, err := deps.Store.GetOrCreate(r.Context())
		if err != nil {
			deps.Log.Error("install state", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		resp := stateWireResponse{
			StartedAt:       st.StartedAt,
			CompletedAt:     st.CompletedAt,
			CurrentStep:     st.CurrentStep,
			Step1Admin:      redactStep1Admin(st.Step1Admin),
			Step2ChirpStack: st.Step2ChirpStack,
			Step3Region:     st.Step3Region,
			Step4Identity:   st.Step4Identity,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// isCompleted is true if any active admin user exists. Symmetric with
// FirstRunGate's adminExists check — soft-deleted (disabled_at IS NOT NULL)
// admins do NOT count, so a stale row from `shifter create-admin` followed by
// disable does not improperly mark the install completed.
func isCompleted(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var v bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM "user" WHERE role='admin' AND disabled_at IS NULL)`).Scan(&v)
	return v, err
}

type step1Req struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Step1Handler — POST /api/install/step/1.
//
// Validates email + name + password presence, lowercases email, runs Plan 07's
// PasswordStrength gate (rejects StrengthWeak), Argon2id-hashes the password,
// and persists `{email, name, password_hash}` into step1_admin JSONB. The
// raw password NEVER reaches the database (D-09).
func Step1Handler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ensureCSRF(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_csrf_header"})
			return
		}
		var req step1Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
			return
		}
		email := strings.ToLower(strings.TrimSpace(req.Email))
		if email == "" || req.Name == "" || req.Password == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "missing_fields"})
			return
		}
		// 256-byte cap before Hash (T-07-05 / Plan 09 pattern).
		if len(req.Password) > 256 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
			return
		}
		if auth.PasswordStrength(req.Password) == auth.StrengthWeak {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "weak_password", "tier": "weak"})
			return
		}
		hash, err := auth.Hash(req.Password)
		if err != nil {
			deps.Log.Error("install step1 hash", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		payload, _ := json.Marshal(map[string]string{
			"email":         email,
			"name":          req.Name,
			"password_hash": hash,
		})
		if err := deps.Store.UpdateStep1(r.Context(), payload); err != nil {
			deps.Log.Error("install step1 persist", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"current_step": 2})
	}
}

type step2Req struct {
	Mode         string `json:"mode"`
	GRPCURL      string `json:"grpc_url"`
	APIToken     string `json:"api_token"`
	MQTTURL      string `json:"mqtt_url"`
	MQTTUser     string `json:"mqtt_user,omitempty"`
	MQTTPassword string `json:"mqtt_password,omitempty"`
}

// Step2Handler — POST /api/install/step/2.
//
// Dials ChirpStack via deps.Dial (production: chirpstack.Dial; test: bufconn
// mock), runs ProbeVersion to refuse v3 (INST-05 / T-15-01), and persists the
// connection draft. The API token is written to {SecretsDir}/chirpstack_api_token
// (mode 0600) and only the path REF is persisted (T-15-02 / D-04). Likewise
// for the optional MQTT password.
//
// Error mapping:
//   - dial / probe failure → 422 grpc_unreachable
//   - probe returns ErrChirpStackV3OrUnknown → 422 v3_detected (INST-05)
//   - secrets-dir write failure → 500 secret_write
func Step2Handler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ensureCSRF(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_csrf_header"})
			return
		}
		// Sequential enforcement: step 2 requires step 1 to have completed.
		st, err := deps.Store.GetOrCreate(r.Context())
		if err != nil {
			deps.Log.Error("install step2 load state", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if st.CurrentStep != 2 {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":  "step_out_of_order",
				"detail": fmt.Sprintf("expected step 2, wizard is at step %d", st.CurrentStep),
			})
			return
		}
		var req step2Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
			return
		}
		if req.Mode != "bundled" && req.Mode != "external" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid_mode"})
			return
		}
		if req.GRPCURL == "" || req.APIToken == "" || req.MQTTURL == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "missing_fields"})
			return
		}

		// INST-05: gRPC probe — refuse v3.
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		csCfg := config.CSConfig{GRPCURL: req.GRPCURL, APIToken: req.APIToken, Insecure: true}
		conn, err := deps.Dial(ctx, csCfg)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "grpc_unreachable", "detail": err.Error(),
			})
			return
		}
		defer conn.Close()

		version, err := chirpstack.ProbeVersion(ctx, conn.Conn())
		if errors.Is(err, chirpstack.ErrChirpStackV3OrUnknown) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "v3_detected"})
			return
		}
		if errors.Is(err, chirpstack.ErrInvalidAPIToken) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "grpc_unreachable", "detail": "invalid_api_token",
			})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "grpc_unreachable", "detail": err.Error(),
			})
			return
		}

		// Persist API token by REF (path on disk, mode 0600).
		apiTokenRef, err := writeSecret(deps.SecretsDir, "chirpstack_api_token", req.APIToken)
		if err != nil {
			deps.Log.Error("install step2 secret write", "err", err, "name", "chirpstack_api_token")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "secret_write"})
			return
		}
		var mqttPasswordRef string
		if req.MQTTPassword != "" {
			mqttPasswordRef, err = writeSecret(deps.SecretsDir, "mqtt_password", req.MQTTPassword)
			if err != nil {
				deps.Log.Error("install step2 secret write", "err", err, "name", "mqtt_password")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "secret_write"})
				return
			}
		}

		payload, _ := json.Marshal(map[string]any{
			"mode":              req.Mode,
			"grpc_url":          req.GRPCURL,
			"api_token_ref":     apiTokenRef,
			"mqtt_url":          req.MQTTURL,
			"mqtt_user":         req.MQTTUser,
			"mqtt_password_ref": mqttPasswordRef,
			"version":           version,
		})
		if err := deps.Store.UpdateStep2(r.Context(), payload); err != nil {
			deps.Log.Error("install step2 persist", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"current_step":       3,
			"chirpstack_version": version,
		})
	}
}

type step3Req struct {
	Name string `json:"name"`
}

// Step3Handler — POST /api/install/step/3.
//
// Whitelists the operator's region pick against Plan 14's Regions() catalog
// (T-14-04 — never trust the JSONB payload's `name` as-is) and persists both
// the URL-safe `name` (e.g. "as923_2") and ChirpStack's `common_name` (e.g.
// "AS923_2"); both are needed downstream (ChirpStack APIs accept `name`, UI
// displays `common_name`).
func Step3Handler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ensureCSRF(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_csrf_header"})
			return
		}
		// Sequential enforcement: step 3 requires step 2 to have completed.
		st, err := deps.Store.GetOrCreate(r.Context())
		if err != nil {
			deps.Log.Error("install step3 load state", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if st.CurrentStep != 3 {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":  "step_out_of_order",
				"detail": fmt.Sprintf("expected step 3, wizard is at step %d", st.CurrentStep),
			})
			return
		}
		var req step3Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
			return
		}
		region, ok := RegionByName(req.Name)
		if !ok {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "unknown_region"})
			return
		}
		payload, _ := json.Marshal(map[string]string{
			"name":        region.Name,
			"common_name": region.CommonName,
		})
		if err := deps.Store.UpdateStep3(r.Context(), payload); err != nil {
			deps.Log.Error("install step3 persist", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"current_step": 4})
	}
}

type step4Req struct {
	DisplayName string `json:"display_name"`
	Address     string `json:"address,omitempty"`
	Timezone    string `json:"timezone"`
	Units       string `json:"units"` // metric | imperial
	LogoPath    string `json:"logo_path,omitempty"`
}

// Step4Handler — POST /api/install/step/4.
//
// Validates display_name presence, timezone via time.LoadLocation (T-15-07),
// and units against the install_identity.units enum (metric | imperial).
// Persists the install identity draft.
func Step4Handler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ensureCSRF(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_csrf_header"})
			return
		}
		// Sequential enforcement: step 4 requires step 3 to have completed.
		st, err := deps.Store.GetOrCreate(r.Context())
		if err != nil {
			deps.Log.Error("install step4 load state", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if st.CurrentStep != 4 {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":  "step_out_of_order",
				"detail": fmt.Sprintf("expected step 4, wizard is at step %d", st.CurrentStep),
			})
			return
		}
		var req step4Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
			return
		}
		if req.DisplayName == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "missing_fields"})
			return
		}
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid_timezone"})
			return
		}
		if req.Units != "metric" && req.Units != "imperial" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid_units"})
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"display_name": req.DisplayName,
			"address":      req.Address,
			"timezone":     req.Timezone,
			"units":        req.Units,
			"logo_path":    req.LogoPath,
		})
		if err := deps.Store.UpdateStep4(r.Context(), payload); err != nil {
			deps.Log.Error("install step4 persist", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"current_step": 5})
	}
}

// FinishHandler — POST /api/install/finish.
//
// Calls FinishSetup (finish.go) which runs the atomic Serializable txn. Maps
// FinishSetup's typed errors to operator-facing JSON:
//
//   - ErrAlreadyCompleted   → 410 install_completed
//   - ErrIncompleteWizard   → 422 incomplete_wizard
//   - any other error       → 500 internal (logged with full chain)
func FinishHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ensureCSRF(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_csrf_header"})
			return
		}
		if err := FinishSetup(r.Context(), deps); err != nil {
			if errors.Is(err, ErrAlreadyCompleted) {
				writeJSON(w, http.StatusGone, map[string]string{"error": "install_completed"})
				return
			}
			if errors.Is(err, ErrIncompleteWizard) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "incomplete_wizard"})
				return
			}
			deps.Log.Error("install finish", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// writeSecret writes value to {dir}/{name}, mode 0600, returning the absolute
// path. Phase 1 keeps writes local; production install kit pre-populates
// /run/secrets/ via Compose secrets and the wizard simply reuses the SAME path
// idiom for operator-provided values (T-15-02 / D-04).
func writeSecret(dir, name, value string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir secrets: %w", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	return path, nil
}
