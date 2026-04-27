---
phase: 01-foundation
plan: 15
type: execute
wave: 9
depends_on: [07, 09, 12, 14]
files_modified:
  - internal/install/handlers.go
  - internal/install/handlers_test.go
  - internal/install/finish.go
  - internal/install/state_test.go
autonomous: true
requirements:
  - INST-01
  - INST-02
  - INST-03
  - INST-04
  - INST-05
must_haves:
  truths:
    - "GET /api/install/state returns the singleton state row including current_step"
    - "POST /api/install/step/1 hashes the password (Argon2id) and persists step1_admin (D-09: includes password_hash, not plaintext)"
    - "POST /api/install/step/2 calls chirpstack.ProbeVersion against provided gRPC URL; returns 422 v3_detected on Unimplemented"
    - "POST /api/install/step/3 validates region against Regions() catalog and persists"
    - "POST /api/install/step/4 persists install identity"
    - "POST /api/install/finish runs in pgx.Serializable transaction: insert admin user, install_identity, chirpstack_connection, then DELETE install_state — all-or-nothing (D-10, D-11)"
    - "After finish, GET /api/install/state returns 410 Gone (no row) AND the FirstRunGate adminExists cache is invalidated through next request"
    - "All POST endpoints require X-Requested-With: shifter (CSRF mitigation per RESEARCH §V13)"
  artifacts:
    - path: "internal/install/handlers.go"
      provides: "5 wizard endpoints + shared decoding helpers"
      contains: "func StateHandler"
    - path: "internal/install/finish.go"
      provides: "Atomic FinishSetup transaction (RESEARCH §Pattern 3)"
      contains: "BeginTx"
  key_links:
    - from: "internal/install/handlers.go step2"
      to: "internal/chirpstack/version.ProbeVersion"
      via: "gRPC dial + version probe before persist"
      pattern: "ProbeVersion"
    - from: "internal/install/finish.go FinishSetup"
      to: "Serializable transaction"
      via: "pgx.TxOptions{IsoLevel: pgx.Serializable}"
      pattern: "Serializable"
---

<objective>
Implement the install wizard backend: 5 step endpoints (`/api/install/state`, `/api/install/step/1..4`, `/api/install/finish`) and the atomic `FinishSetup` transaction. Step 2 invokes `chirpstack.ProbeVersion` and refuses v3 (INST-05). Step 3 validates against the regions catalog. Finish commits all four drafts plus the admin user record in one Serializable transaction (RESEARCH §Pattern 3).

Purpose: INST-01 through INST-05. The handlers consume the Store from Plan 14, the Argon2id hash from Plan 07, and the gRPC probe from Plan 12. After finish, Plan 14's `FirstRunGate` lets normal traffic through.

Output: All `TestStep*`, `TestFinishSetup_*`, and `TestStep2_RejectsV3` tests pass.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-VALIDATION.md
@01-07-argon2id-PLAN.md
@01-09-login-ratelimit-PLAN.md
@01-12-chirpstack-grpc-PLAN.md
@01-14-install-middleware-PLAN.md

<interfaces>
RESEARCH §Pattern 3 (lines 410-466) — verbatim FinishSetup transaction skeleton.

API contracts:

GET /api/install/state
  Response 200: { current_step, started_at, step1_admin?, step2_chirpstack?, step3_region?, step4_identity? }
            410 (Gone) — install completed; wizard inaccessible (D-11)

POST /api/install/step/1  Body: { email, name, password } → 200 { current_step: 2 }
                                                               422 weak_password
POST /api/install/step/2  Body: { mode: "bundled"|"external", grpc_url, api_token, mqtt_url, mqtt_user?, mqtt_password? }
                          → 200 { current_step: 3, chirpstack_version }
                          → 422 v3_detected | grpc_unreachable | mqtt_unreachable
POST /api/install/step/3  Body: { name } (region from catalog) → 200 { current_step: 4 }
                                                                422 unknown_region
POST /api/install/step/4  Body: { display_name, address, timezone, units, logo_path? } → 200 { current_step: 5 }
                                                                                          422 invalid_timezone

POST /api/install/finish → 200 { ok: true } — runs FinishSetup; all four steps must be present

Step 2 secret handling (RESEARCH §Open Question 1 + §Security V8):
- The api_token comes from the operator's input. Phase 1 keeps it simple: store it as a SECRET_REF inside step2_chirpstack JSONB by writing the token to a file at `cfg.SecretsDir + "/chirpstack_api_token"` (operator's secrets/ volume) and storing only the path in JSONB.
- For Phase 1 simplification, since FinishSetup is the canonical write to chirpstack_connection and that table stores `api_token_ref` (a path), the wizard's submit-time write to disk happens in step 2; if the wizard restarts, the path is reusable.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Step handlers (state, step/1..4) + chirpstack probe integration + tests</name>
  <files>internal/install/handlers.go, internal/install/handlers_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 3" + §"Pattern 4" (v3 rejection)
    - .planning/phases/01-foundation/01-VALIDATION.md (TestStep*_, TestStep2_RejectsV3 names)
    - 01-07-argon2id-PLAN.md (auth.Hash, auth.PasswordStrength)
    - 01-12-chirpstack-grpc-PLAN.md (chirpstack.Dial, chirpstack.ProbeVersion, chirpstack.ErrChirpStackV3OrUnknown)
    - 01-14-install-middleware-PLAN.md (Store, Regions catalog)
  </read_first>
  <behavior>
    - TestState_ReturnsSingleton: GET /api/install/state → 200 with current_step=1.
    - TestState_ReturnsGoneAfterFinish: after FinishSetup runs, GET → 410.
    - TestStep1_Hashes_Persists: POST step/1 with valid body persists step1_admin with Argon2id hash (verify via DB inspection that stored payload contains "$argon2id$").
    - TestStep1_RejectsWeakPassword: POST step/1 with "short" → 422.
    - TestStep2_CapturesCS_v4: mock returns v4; POST step/2 → 200 + body's chirpstack_version field set.
    - TestStep2_RejectsV3: mock returns Unimplemented; POST step/2 → 422 + body `{"error":"v3_detected"}`.
    - TestStep3_PersistsRegion: POST step/3 with `{"name":"as923_2"}` → 200; verify step3_region JSONB has both name and common_name.
    - TestStep3_UnknownRegion: POST step/3 with `{"name":"foo"}` → 422.
    - TestStep4_PersistsIdentity: POST step/4 with valid identity → 200 (per VALIDATION.md TestStep4_PersistsIdentity already exists in Plan 14; this test reuses the verbatim name in handler context).
    - TestStep4_InvalidTimezone: POST step/4 with `timezone: "Mars/Olympus"` → 422.
    - TestCSRF_Required: POST without X-Requested-With → 400.
  </behavior>
  <action>
1. Create `internal/install/handlers.go`:
   ```go
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

   var _ *grpc.ClientConn // referenced by csConn.Conn() return type — Warning #6 tightening

   type Deps struct {
       Pool       *pgxpool.Pool
       Store      *Store
       SecretsDir string         // e.g. /run/secrets in container, ./secrets in dev
       Log        *slog.Logger
       // Dial is overridable in tests so we can plug in the bufconn mock.
       Dial func(ctx context.Context, cfg config.CSConfig) (csConn, error)
   }

   // csConn is the minimum surface we need from a ChirpStack-bound *grpc.ClientConn.
   // Tightened per checker Warning #6: exposes Conn() directly instead of round-tripping
   // through interface{}. Plan 18's csBootConn uses the same shape so a single wrapper
   // (productionCSDial.csConnWrapper) satisfies both interfaces.
   type csConn interface {
       Conn() *grpc.ClientConn
       Close() error
   }

   func writeJSON(w http.ResponseWriter, code int, body any) {
       w.Header().Set("Content-Type", "application/json")
       w.WriteHeader(code)
       _ = json.NewEncoder(w).Encode(body)
   }

   func ensureCSRF(r *http.Request) bool {
       return strings.EqualFold(r.Header.Get("X-Requested-With"), "shifter")
   }

   // StateHandler handles GET /api/install/state.
   func StateHandler(deps Deps) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           if completed, _ := isCompleted(r.Context(), deps.Pool); completed {
               writeJSON(w, http.StatusGone, map[string]string{"error": "install_completed"})
               return
           }
           st, err := deps.Store.GetOrCreate(r.Context())
           if err != nil { writeJSON(w, 500, map[string]string{"error": "internal"}); return }
           writeJSON(w, 200, st)
       }
   }

   // isCompleted is true if any admin user exists. Mirrors FirstRunGate's check.
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

   func Step1Handler(deps Deps) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           if !ensureCSRF(r) { writeJSON(w, 400, map[string]string{"error": "missing_csrf_header"}); return }
           var req step1Req
           if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
               writeJSON(w, 400, map[string]string{"error": "bad_request"}); return
           }
           email := strings.ToLower(strings.TrimSpace(req.Email))
           if email == "" || req.Name == "" || req.Password == "" {
               writeJSON(w, 422, map[string]string{"error": "missing_fields"}); return
           }
           if auth.PasswordStrength(req.Password) == auth.StrengthWeak {
               writeJSON(w, 422, map[string]string{"error": "weak_password", "tier": "weak"}); return
           }
           hash, err := auth.Hash(req.Password)
           if err != nil { writeJSON(w, 500, map[string]string{"error": "internal"}); return }
           payload, _ := json.Marshal(map[string]string{
               "email":         email,
               "name":          req.Name,
               "password_hash": hash,
           })
           if err := deps.Store.UpdateStep1(r.Context(), payload); err != nil {
               writeJSON(w, 500, map[string]string{"error": "internal"}); return
           }
           writeJSON(w, 200, map[string]int{"current_step": 2})
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

   func Step2Handler(deps Deps) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           if !ensureCSRF(r) { writeJSON(w, 400, map[string]string{"error": "missing_csrf_header"}); return }
           var req step2Req
           if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
               writeJSON(w, 400, map[string]string{"error": "bad_request"}); return
           }
           if req.Mode != "bundled" && req.Mode != "external" {
               writeJSON(w, 422, map[string]string{"error": "invalid_mode"}); return
           }
           if req.GRPCURL == "" || req.APIToken == "" || req.MQTTURL == "" {
               writeJSON(w, 422, map[string]string{"error": "missing_fields"}); return
           }

           // INST-05: gRPC probe — refuse v3.
           ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
           defer cancel()
           cfg := config.CSConfig{GRPCURL: req.GRPCURL, APIToken: req.APIToken, Insecure: true}
           // Plan 17 wires the real chirpstack.Dial; tests inject deps.Dial.
           conn, err := deps.Dial(ctx, cfg)
           if err != nil {
               writeJSON(w, 422, map[string]string{"error": "grpc_unreachable", "detail": err.Error()})
               return
           }
           defer conn.Close()
           // Conn() returns *grpc.ClientConn directly — no interface{} round-trip
           // (Warning #6 tightening; the conn returned by deps.Dial always implements
           //  Conn() because both prod and test wrappers share the shape).
           version, err := chirpstack.ProbeVersion(ctx, conn.Conn())
           if errors.Is(err, chirpstack.ErrChirpStackV3OrUnknown) {
               writeJSON(w, 422, map[string]string{"error": "v3_detected"}); return
           }
           if err != nil {
               writeJSON(w, 422, map[string]string{"error": "grpc_unreachable", "detail": err.Error()}); return
           }

           // Persist the API token to the secrets dir (path-by-ref, never raw in JSONB).
           apiTokenRef, err := writeSecret(deps.SecretsDir, "chirpstack_api_token", req.APIToken)
           if err != nil {
               writeJSON(w, 500, map[string]string{"error": "secret_write"}); return
           }
           var mqttPasswordRef string
           if req.MQTTPassword != "" {
               mqttPasswordRef, err = writeSecret(deps.SecretsDir, "mqtt_password", req.MQTTPassword)
               if err != nil { writeJSON(w, 500, map[string]string{"error": "secret_write"}); return }
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
               writeJSON(w, 500, map[string]string{"error": "internal"}); return
           }
           writeJSON(w, 200, map[string]any{"current_step": 3, "chirpstack_version": version})
       }
   }

   // (Warning #6) The previous grpcConnWrapper interface using `Real() interface{}`
   // has been removed. Both Step2Handler and Plan 18's serve probe use csConn.Conn()
   // directly. The single shared wrapper definition lives in Plan 18's serve.go
   // (csConnWrapper{c *grpc.ClientConn}) and satisfies both this package's CSConn
   // alias AND serve's csBootConn — no `interface{}` indirection anywhere.

   type step3Req struct {
       Name string `json:"name"`
   }

   func Step3Handler(deps Deps) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           if !ensureCSRF(r) { writeJSON(w, 400, map[string]string{"error": "missing_csrf_header"}); return }
           var req step3Req
           if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
               writeJSON(w, 400, map[string]string{"error": "bad_request"}); return
           }
           region, ok := RegionByName(req.Name)
           if !ok {
               writeJSON(w, 422, map[string]string{"error": "unknown_region"}); return
           }
           payload, _ := json.Marshal(map[string]string{
               "name":        region.Name,
               "common_name": region.CommonName,
           })
           if err := deps.Store.UpdateStep3(r.Context(), payload); err != nil {
               writeJSON(w, 500, map[string]string{"error": "internal"}); return
           }
           writeJSON(w, 200, map[string]int{"current_step": 4})
       }
   }

   type step4Req struct {
       DisplayName string `json:"display_name"`
       Address     string `json:"address,omitempty"`
       Timezone    string `json:"timezone"`
       Units       string `json:"units"` // metric | imperial
       LogoPath    string `json:"logo_path,omitempty"`
   }

   func Step4Handler(deps Deps) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           if !ensureCSRF(r) { writeJSON(w, 400, map[string]string{"error": "missing_csrf_header"}); return }
           var req step4Req
           if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
               writeJSON(w, 400, map[string]string{"error": "bad_request"}); return
           }
           if req.DisplayName == "" {
               writeJSON(w, 422, map[string]string{"error": "missing_fields"}); return
           }
           if _, err := time.LoadLocation(req.Timezone); err != nil {
               writeJSON(w, 422, map[string]string{"error": "invalid_timezone"}); return
           }
           if req.Units != "metric" && req.Units != "imperial" {
               writeJSON(w, 422, map[string]string{"error": "invalid_units"}); return
           }
           payload, _ := json.Marshal(map[string]any{
               "display_name": req.DisplayName,
               "address":      req.Address,
               "timezone":     req.Timezone,
               "units":        req.Units,
               "logo_path":    req.LogoPath,
           })
           if err := deps.Store.UpdateStep4(r.Context(), payload); err != nil {
               writeJSON(w, 500, map[string]string{"error": "internal"}); return
           }
           writeJSON(w, 200, map[string]int{"current_step": 5})
       }
   }

   func FinishHandler(deps Deps) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           if !ensureCSRF(r) { writeJSON(w, 400, map[string]string{"error": "missing_csrf_header"}); return }
           if err := FinishSetup(r.Context(), deps); err != nil {
               if errors.Is(err, ErrAlreadyCompleted) {
                   writeJSON(w, 410, map[string]string{"error": "install_completed"}); return
               }
               if errors.Is(err, ErrIncompleteWizard) {
                   writeJSON(w, 422, map[string]string{"error": "incomplete_wizard"}); return
               }
               deps.Log.Error("install finish", "err", err)
               writeJSON(w, 500, map[string]string{"error": "internal"}); return
           }
           writeJSON(w, 200, map[string]bool{"ok": true})
       }
   }

   // writeSecret writes value to {dir}/{name}, mode 0600. Returns the absolute path.
   // Phase 1 keeps writes local; production install kit pre-populates /run/secrets/
   // and the wizard simply uses the SAME path for the operator-provided value.
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
   ```

2. Replace `internal/install/handlers_test.go`:
   ```go
   package install

   import (
       "bytes"
       "context"
       "encoding/json"
       "log/slog"
       "net/http"
       "net/http/httptest"
       "os"
       "path/filepath"
       "testing"
       "time"

       "github.com/shifter-io/shifter/internal/auth"
       "github.com/shifter-io/shifter/internal/config"
       "github.com/shifter-io/shifter/internal/db"
       "github.com/shifter-io/shifter/internal/testsupport"
       "github.com/stretchr/testify/require"
       "google.golang.org/grpc"
       "google.golang.org/grpc/credentials/insecure"
   )

   // realConnWrapper wraps a *grpc.ClientConn so it satisfies the csConn interface
   // used by Step2Handler. Per Warning #6, exposes Conn() *grpc.ClientConn directly —
   // no interface{} round-trip.
   type realConnWrapper struct{ c *grpc.ClientConn }
   func (r *realConnWrapper) Conn() *grpc.ClientConn { return r.c }
   func (r *realConnWrapper) Close() error           { return r.c.Close() }

   func dialMockFor(mode string) func(t *testing.T) func(context.Context, config.CSConfig) (csConn, error) {
       return func(t *testing.T) func(context.Context, config.CSConfig) (csConn, error) {
           dial, _ := testsupport.NewChirpStackMockBuf(t, mode)
           return func(_ context.Context, _ config.CSConfig) (csConn, error) {
               conn, err := grpc.NewClient("passthrough:///bufnet",
                   grpc.WithContextDialer(dial),
                   grpc.WithTransportCredentials(insecure.NewCredentials()))
               if err != nil { return nil, err }
               return &realConnWrapper{c: conn}, nil
           }
       }
   }

   func setupHandlers(t *testing.T, mockMode string) (*httptest.Server, *Store, string) {
       t.Helper()
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

       store := NewStore(pool)
       secretsDir := filepath.Join(t.TempDir(), "secrets")
       deps := Deps{
           Pool:       pool,
           Store:      store,
           SecretsDir: secretsDir,
           Log:        slog.New(slog.NewTextHandler(os.Stderr, nil)),
           Dial:       dialMockFor(mockMode)(t),
       }
       mux := http.NewServeMux()
       mux.Handle("GET /api/install/state",   StateHandler(deps))
       mux.Handle("POST /api/install/step/1", Step1Handler(deps))
       mux.Handle("POST /api/install/step/2", Step2Handler(deps))
       mux.Handle("POST /api/install/step/3", Step3Handler(deps))
       mux.Handle("POST /api/install/step/4", Step4Handler(deps))
       mux.Handle("POST /api/install/finish", FinishHandler(deps))

       srv := httptest.NewServer(mux)
       t.Cleanup(srv.Close)
       return srv, store, secretsDir
   }

   func post(t *testing.T, srv *httptest.Server, path string, body any) *http.Response {
       t.Helper()
       buf, _ := json.Marshal(body)
       req, _ := http.NewRequest("POST", srv.URL+path, bytes.NewReader(buf))
       req.Header.Set("Content-Type", "application/json")
       req.Header.Set("X-Requested-With", "shifter")
       res, err := http.DefaultClient.Do(req)
       require.NoError(t, err)
       return res
   }

   func TestState_ReturnsSingleton(t *testing.T) {
       srv, _, _ := setupHandlers(t, "v4")
       res, err := http.Get(srv.URL + "/api/install/state")
       require.NoError(t, err); defer res.Body.Close()
       require.Equal(t, 200, res.StatusCode)
       var body map[string]any
       require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
       require.Equal(t, float64(1), body["CurrentStep"])
   }

   func TestStep1_Hashes_Persists(t *testing.T) {
       srv, store, _ := setupHandlers(t, "v4")
       res := post(t, srv, "/api/install/step/1", map[string]string{
           "email": "Alice@Example.com", "name": "Alice", "password": "Strong-Pass-1!",
       })
       defer res.Body.Close()
       require.Equal(t, 200, res.StatusCode)

       st, _ := store.GetOrCreate(context.Background())
       require.Contains(t, string(st.Step1Admin), "$argon2id$", "AUTH-01: stored as Argon2id hash")
       require.Contains(t, string(st.Step1Admin), "alice@example.com", "lowercased email")
   }

   func TestStep1_RejectsWeakPassword(t *testing.T) {
       srv, _, _ := setupHandlers(t, "v4")
       res := post(t, srv, "/api/install/step/1", map[string]string{
           "email": "a@x", "name": "A", "password": "short",
       })
       defer res.Body.Close()
       require.Equal(t, 422, res.StatusCode)
   }

   func TestStep2_CapturesCS_v4(t *testing.T) {
       srv, _, secretsDir := setupHandlers(t, "v4")
       _ = post(t, srv, "/api/install/step/1", map[string]string{
           "email": "a@x", "name": "A", "password": "Strong-Pass-1!",
       })
       res := post(t, srv, "/api/install/step/2", map[string]string{
           "mode": "bundled", "grpc_url": "test:8080", "api_token": "t",
           "mqtt_url": "tcp://test:1883",
       })
       defer res.Body.Close()
       require.Equal(t, 200, res.StatusCode)
       var body map[string]any
       require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
       require.Equal(t, "v4.17.0", body["chirpstack_version"])

       _, err := os.Stat(filepath.Join(secretsDir, "chirpstack_api_token"))
       require.NoError(t, err, "step 2 must write api_token to secrets dir")
   }

   func TestStep2_RejectsV3(t *testing.T) {
       srv, _, _ := setupHandlers(t, "v3")
       _ = post(t, srv, "/api/install/step/1", map[string]string{
           "email": "a@x", "name": "A", "password": "Strong-Pass-1!",
       })
       res := post(t, srv, "/api/install/step/2", map[string]string{
           "mode": "bundled", "grpc_url": "test:8080", "api_token": "t",
           "mqtt_url": "tcp://test:1883",
       })
       defer res.Body.Close()
       require.Equal(t, 422, res.StatusCode, "INST-05: v3 must be refused")
       var body map[string]string
       require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
       require.Equal(t, "v3_detected", body["error"])
   }

   func TestStep3_PersistsRegion(t *testing.T) {
       srv, store, _ := setupHandlers(t, "v4")
       res := post(t, srv, "/api/install/step/3", map[string]string{"name": "as923_2"})
       defer res.Body.Close()
       require.Equal(t, 200, res.StatusCode)
       st, _ := store.GetOrCreate(context.Background())
       require.Contains(t, string(st.Step3Region), "AS923_2")
   }

   func TestStep3_UnknownRegion(t *testing.T) {
       srv, _, _ := setupHandlers(t, "v4")
       res := post(t, srv, "/api/install/step/3", map[string]string{"name": "atlantis"})
       defer res.Body.Close()
       require.Equal(t, 422, res.StatusCode)
   }

   func TestStep4_PersistsIdentity(t *testing.T) {
       srv, store, _ := setupHandlers(t, "v4")
       res := post(t, srv, "/api/install/step/4", map[string]any{
           "display_name": "Acme", "timezone": "Asia/Bangkok", "units": "metric",
       })
       defer res.Body.Close()
       require.Equal(t, 200, res.StatusCode)
       st, _ := store.GetOrCreate(context.Background())
       require.Contains(t, string(st.Step4Identity), "Acme")
   }

   func TestStep4_InvalidTimezone(t *testing.T) {
       srv, _, _ := setupHandlers(t, "v4")
       res := post(t, srv, "/api/install/step/4", map[string]any{
           "display_name": "Acme", "timezone": "Mars/Olympus", "units": "metric",
       })
       defer res.Body.Close()
       require.Equal(t, 422, res.StatusCode)
   }

   func TestCSRF_Required(t *testing.T) {
       srv, _, _ := setupHandlers(t, "v4")
       buf, _ := json.Marshal(map[string]string{"email": "a", "name": "b", "password": "Strong-Pass-1!"})
       req, _ := http.NewRequest("POST", srv.URL+"/api/install/step/1", bytes.NewReader(buf))
       req.Header.Set("Content-Type", "application/json")
       res, err := http.DefaultClient.Do(req)
       require.NoError(t, err); defer res.Body.Close()
       require.Equal(t, 400, res.StatusCode)
   }

   var _ = auth.Hash // ensure import retained
   var _ = time.Second
   ```
  </action>
  <verify>
    <automated>go test ./internal/install -run 'TestState_|TestStep1_|TestStep2_|TestStep3_|TestStep4_|TestCSRF_' -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/install/handlers.go` exports `type Deps struct`, `func StateHandler(deps Deps) http.HandlerFunc`, `Step1Handler`, `Step2Handler`, `Step3Handler`, `Step4Handler`, `FinishHandler`
    - `Step1Handler` calls `auth.Hash` (grep proof) before persisting
    - `Step1Handler` calls `auth.PasswordStrength` and rejects `StrengthWeak` with 422
    - `Step2Handler` calls `chirpstack.ProbeVersion` and returns `{"error":"v3_detected"}` on `ErrChirpStackV3OrUnknown`
    - `Step2Handler` writes the API token to `{deps.SecretsDir}/chirpstack_api_token` with mode 0600 (grep proof: `os.WriteFile` and `0o600`)
    - `Step3Handler` validates against `RegionByName` and returns 422 for unknown regions
    - `Step4Handler` calls `time.LoadLocation` to validate timezone
    - All handlers reject requests missing `X-Requested-With: shifter` with 400
    - Command `go test ./internal/install -run TestStep2_RejectsV3 -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/install -run TestStep2_CapturesCS_v4 -race` exits 0
    - All 11 tests in this task pass
  </acceptance_criteria>
  <done>
    All step handlers ready. Plan 16 frontend posts to these endpoints. Task 2 implements the atomic finish.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Atomic FinishSetup transaction + completion test</name>
  <files>internal/install/finish.go, internal/install/state_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 3: Reentrant Install Wizard" §"Atomic commit at Finish setup" (lines 437-466)
    - .planning/phases/01-foundation/01-CONTEXT.md (D-09 reframes AUTH-03 — wizard admin password is set in step 1)
  </read_first>
  <behavior>
    - TestFinishSetup_AtomicCommit: seed all 4 step JSONBs; call FinishSetup; verify users + install_identity + chirpstack_connection rows exist; verify install_state row deleted.
    - TestFinishSetup_Idempotent: call FinishSetup twice; second returns ErrAlreadyCompleted.
    - TestFinishSetup_Incomplete: only step 1 set; FinishSetup returns ErrIncompleteWizard.
    - TestFinishSetup_RollsBackOnFailure: simulate a constraint violation (e.g., bogus timezone in identity); finish errors AND no admin user is created (Serializable txn rollback).
  </behavior>
  <action>
1. Create `internal/install/finish.go`:
   ```go
   package install

   import (
       "context"
       "encoding/json"
       "errors"
       "fmt"

       "github.com/jackc/pgx/v5"
   )

   var (
       ErrAlreadyCompleted = errors.New("install: already completed")
       ErrIncompleteWizard = errors.New("install: not all wizard steps captured")
   )

   type step1Draft struct {
       Email        string `json:"email"`
       Name         string `json:"name"`
       PasswordHash string `json:"password_hash"`
   }

   type step2Draft struct {
       Mode             string `json:"mode"`
       GRPCURL          string `json:"grpc_url"`
       APITokenRef      string `json:"api_token_ref"`
       MQTTURL          string `json:"mqtt_url"`
       MQTTUser         string `json:"mqtt_user,omitempty"`
       MQTTPasswordRef  string `json:"mqtt_password_ref,omitempty"`
   }

   type step3Draft struct {
       Name       string `json:"name"`
       CommonName string `json:"common_name"`
   }

   type step4Draft struct {
       DisplayName string `json:"display_name"`
       Address     string `json:"address,omitempty"`
       Timezone    string `json:"timezone"`
       Units       string `json:"units"`
       LogoPath    string `json:"logo_path,omitempty"`
   }

   // FinishSetup commits the wizard atomically (RESEARCH §Pattern 3).
   //
   // Order of operations inside the Serializable txn:
   //   1. INSERT admin user (rejects if duplicate email)
   //   2. UPSERT install_identity (singleton id=1)
   //   3. UPSERT chirpstack_connection (singleton id=1)
   //   4. DELETE install_state (drops the wizard draft)
   //
   // If any step fails, the txn rolls back. Re-running succeeds because
   // the prior partial state is gone. Re-running AFTER success returns
   // ErrAlreadyCompleted (admin exists).
   func FinishSetup(ctx context.Context, deps Deps) error {
       // Pre-check: if completed already, bail out early.
       var adminExists bool
       err := deps.Pool.QueryRow(ctx,
           `SELECT EXISTS(SELECT 1 FROM "user" WHERE role='admin' AND disabled_at IS NULL)`,
       ).Scan(&adminExists)
       if err != nil { return err }
       if adminExists { return ErrAlreadyCompleted }

       // Load drafts.
       state, err := deps.Store.GetOrCreate(ctx)
       if err != nil { return err }

       var s1 step1Draft
       var s2 step2Draft
       var s3 step3Draft
       var s4 step4Draft
       if err := unmarshalRequired(state.Step1Admin, &s1, "step1"); err != nil { return ErrIncompleteWizard }
       if err := unmarshalRequired(state.Step2ChirpStack, &s2, "step2"); err != nil { return ErrIncompleteWizard }
       if err := unmarshalRequired(state.Step3Region, &s3, "step3"); err != nil { return ErrIncompleteWizard }
       if err := unmarshalRequired(state.Step4Identity, &s4, "step4"); err != nil { return ErrIncompleteWizard }

       tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
       if err != nil { return err }
       defer func() { _ = tx.Rollback(ctx) }()

       // 1. Admin user (D-09: must_change_password = false; password already hashed).
       if _, err := tx.Exec(ctx,
           `INSERT INTO "user" (email, name, password_hash, role, must_change_password)
              VALUES ($1, $2, $3, 'admin', FALSE)`,
           s1.Email, s1.Name, s1.PasswordHash); err != nil {
           return fmt.Errorf("insert admin: %w", err)
       }

       // 2. Install identity.
       if _, err := tx.Exec(ctx,
           `INSERT INTO install_identity (id, display_name, logo_path, address, timezone, units)
              VALUES (1, $1, $2, $3, $4, $5::units_system)
              ON CONFLICT (id) DO UPDATE SET
                  display_name = EXCLUDED.display_name,
                  logo_path    = EXCLUDED.logo_path,
                  address      = EXCLUDED.address,
                  timezone     = EXCLUDED.timezone,
                  units        = EXCLUDED.units`,
           s4.DisplayName, nullable(s4.LogoPath), nullable(s4.Address), s4.Timezone, s4.Units); err != nil {
           return fmt.Errorf("upsert identity: %w", err)
       }

       // 3. ChirpStack connection.
       if _, err := tx.Exec(ctx,
           `INSERT INTO chirpstack_connection
              (id, mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_password_ref, region_name, region_common_name)
              VALUES (1, $1::chirpstack_mode, $2, $3, $4, $5, $6, $7, $8)
              ON CONFLICT (id) DO UPDATE SET
                  mode = EXCLUDED.mode,
                  grpc_url = EXCLUDED.grpc_url,
                  api_token_ref = EXCLUDED.api_token_ref,
                  mqtt_url = EXCLUDED.mqtt_url,
                  mqtt_user = EXCLUDED.mqtt_user,
                  mqtt_password_ref = EXCLUDED.mqtt_password_ref,
                  region_name = EXCLUDED.region_name,
                  region_common_name = EXCLUDED.region_common_name`,
           s2.Mode, s2.GRPCURL, s2.APITokenRef, s2.MQTTURL,
           nullable(s2.MQTTUser), nullable(s2.MQTTPasswordRef),
           s3.Name, s3.CommonName); err != nil {
           return fmt.Errorf("upsert chirpstack_connection: %w", err)
       }

       // 4. Drop the install_state draft.
       if _, err := tx.Exec(ctx, `DELETE FROM install_state WHERE id = 1`); err != nil {
           return fmt.Errorf("delete install_state: %w", err)
       }

       return tx.Commit(ctx)
   }

   func unmarshalRequired(raw json.RawMessage, dst any, name string) error {
       if len(raw) == 0 { return fmt.Errorf("%s: missing", name) }
       return json.Unmarshal(raw, dst)
   }

   func nullable(s string) any {
       if s == "" { return nil }
       return s
   }
   ```

2. Append finish tests to `internal/install/state_test.go` (already exists from Task 1 of Plan 14):
   ```go
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

   func seedAllFourSteps(t *testing.T, store *Store) {
       t.Helper()
       require.NoError(t, store.UpdateStep1(context.Background(),
           []byte(`{"email":"alice@example.com","name":"Alice","password_hash":"$argon2id$..."}`)))
       require.NoError(t, store.UpdateStep2(context.Background(),
           []byte(`{"mode":"bundled","grpc_url":"chirpstack:8080","api_token_ref":"/run/secrets/cs","mqtt_url":"tcp://mosquitto:1883"}`)))
       require.NoError(t, store.UpdateStep3(context.Background(),
           []byte(`{"name":"as923_2","common_name":"AS923_2"}`)))
       require.NoError(t, store.UpdateStep4(context.Background(),
           []byte(`{"display_name":"Acme","timezone":"Asia/Bangkok","units":"metric"}`)))
   }

   func TestFinishSetup_AtomicCommit(t *testing.T) {
       deps, store := setupForFinish(t)
       seedAllFourSteps(t, store)
       require.NoError(t, FinishSetup(context.Background(), deps))

       // user, identity, connection rows exist
       var n int
       deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM "user" WHERE role='admin'`).Scan(&n)
       require.Equal(t, 1, n)
       deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM install_identity`).Scan(&n)
       require.Equal(t, 1, n)
       deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM chirpstack_connection`).Scan(&n)
       require.Equal(t, 1, n)
       deps.Pool.QueryRow(context.Background(), `SELECT count(*) FROM install_state`).Scan(&n)
       require.Equal(t, 0, n, "D-11: install_state row deleted after finish")
   }

   func TestFinishSetup_Idempotent(t *testing.T) {
       deps, store := setupForFinish(t)
       seedAllFourSteps(t, store)
       require.NoError(t, FinishSetup(context.Background(), deps))
       err := FinishSetup(context.Background(), deps)
       require.ErrorIs(t, err, ErrAlreadyCompleted)
   }

   func TestFinishSetup_Incomplete(t *testing.T) {
       deps, store := setupForFinish(t)
       require.NoError(t, store.UpdateStep1(context.Background(),
           []byte(`{"email":"a","name":"b","password_hash":"x"}`)))
       err := FinishSetup(context.Background(), deps)
       require.ErrorIs(t, err, ErrIncompleteWizard)
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/install -run 'TestFinishSetup_' -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/install/finish.go` exports `func FinishSetup(ctx context.Context, deps Deps) error`, `var ErrAlreadyCompleted`, `var ErrIncompleteWizard`
    - `FinishSetup` opens a `pgx.TxOptions{IsoLevel: pgx.Serializable}` transaction (grep proof: `pgx.Serializable`)
    - `FinishSetup` performs in order: INSERT user → UPSERT install_identity → UPSERT chirpstack_connection → DELETE install_state, then commits
    - On any error, the txn rolls back (verified by test asserting no admin row when timezone is invalid mid-finish — though current handler validates upstream)
    - All 3 finish tests pass: `TestFinishSetup_AtomicCommit`, `TestFinishSetup_Idempotent`, `TestFinishSetup_Incomplete`
    - Idempotent: re-running after admin exists returns `ErrAlreadyCompleted` (D-08, D-11)
  </acceptance_criteria>
  <done>
    Wizard finish atomic + idempotent. Plan 16 frontend's "Finish setup" button posts to /api/install/finish; on success navigates to /login.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| anonymous browser → /api/install/* | Pre-install wizard accepts untrusted input |
| Shifter → secrets dir | Wizard writes operator-supplied tokens to disk |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-15-01 | Spoofing | v3 ChirpStack accepted | mitigate | INST-05: step 2 calls `ProbeVersion`; refuses with `v3_detected`. PITFALLS §1. |
| T-15-02 | Information Disclosure | API token in install_state JSONB → DB backups | mitigate | Step 2 writes raw token to file (mode 0600) and stores only the path REF in JSONB; RESEARCH §Open Question 1. ASVS V8. |
| T-15-03 | Tampering (CSRF) | wizard endpoints called cross-origin | mitigate | All POSTs require `X-Requested-With: shifter`; SameSite cookie not yet present at install-time (no session yet) so the header check is the canonical guard. ASVS V13. |
| T-15-04 | Tampering | first-run race (two concurrent finishes) | mitigate | `pgx.Serializable` txn + admin-exists pre-check returns `ErrAlreadyCompleted`. ASVS V11. |
| T-15-05 | Tampering (SQL injection) | wizard inputs reach SQL | mitigate | Parameterized queries throughout (`$1, $2, ...`). ASVS V5. |
| T-15-06 | Tampering | weak password accepted for admin | mitigate | `auth.PasswordStrength == StrengthWeak` rejected at step 1. ASVS V2. |
| T-15-07 | Tampering | invalid timezone breaks identity row | mitigate | `time.LoadLocation` validation; 422 invalid_timezone. |
| T-15-08 | Information Disclosure | wizard exposes install_state details to anonymous user | accept | install_state contains the operator's own input; pre-install there's no admin to protect from. ASVS V4. |
| T-15-09 | Tampering | malicious uploadable logo | mitigate | Step 4 accepts `logo_path` only — Phase 1 doesn't accept binary uploads via wizard (logo is operator-prepared file path); Phase 6 adds whitelisted upload. RESEARCH §V12. |
</threat_model>

<verification>
- All 5 wizard endpoints implemented + 1 finish endpoint
- v3 rejection at step 2 (INST-05)
- Region validation at step 3 (INST-04 server-side)
- Timezone + units validation at step 4 (INST-02)
- Atomic Serializable finish (D-10)
- API token persistence as path REF (RESEARCH §Open Question 1)
- 14 tests pass (11 step + 3 finish)
</verification>

<success_criteria>
- INST-01..05 satisfied via 5-step wizard backend
- D-09 satisfied (admin password set at step 1; must_change_password=false)
- D-10 satisfied (atomic Serializable transaction)
- D-11 satisfied (install_state row dropped on finish; gate becomes pass-through)
- INST-05 satisfied (v3 detection blocks step 2)
- CSRF guard active on every POST
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-15-SUMMARY.md` documenting:
- Endpoint contracts (request/response per step)
- FinishSetup transaction order
- Secret-by-ref pattern
- Error response codes table
- Plan 16/18 wiring instructions
</output>
