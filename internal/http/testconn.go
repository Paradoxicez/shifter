// Package http — HTTP handlers that don't fit cleanly into a single domain
// package. Today: ChirpStack Test Connection (CHIRP-03) plus the GET / PUT
// chirpstack-connection settings handlers (SETT-01 / SETT-03).
//
// Plan 17 ships the wire surface; Plan 18 mounts these handlers under chi
// behind `auth.RequireAction(sm, ActionConnectionTest)` (POST /test) and
// `auth.RequireAction(sm, ActionConnectionEdit)` (PUT). The GET handler is
// authenticated-only — both admin and viewer can read connection metadata
// (api_token is NEVER returned, T-17-01 / V8).
package http

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

	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/config"
)

// TestConnDeps is the dependency bundle for the Test Connection handler plus
// the GET/PUT chirpstack settings handlers. Plan 18 constructs one TestConnDeps
// at startup; tests inject Dial + PingMQTT to drive the bufconn mock.
type TestConnDeps struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
	// Dial is overridable in tests so the bufconn-backed ChirpStack mock can
	// be swapped in without exercising real TLS / network. Plan 18 wires
	// ProductionDial which wraps chirpstack.Dial.
	Dial func(ctx context.Context, cfg config.CSConfig) (chirpStackConn, error)
	// PingMQTT is overridable in tests so the MQTT half can be exercised
	// without a Mosquitto testcontainer when the gRPC half is the focus.
	// Production passes chirpstack.PingMQTT directly.
	PingMQTT func(ctx context.Context, url, user, pass string) error
}

// chirpStackConn is the minimum surface the handlers need from a ChirpStack-
// bound *grpc.ClientConn. Tightened per RESEARCH Warning #6: exposes
// Conn() *grpc.ClientConn directly instead of round-tripping through interface{}.
// Plan 15's csConn (install) and Plan 18's csBootConn share this exact shape so
// a single wrapper (csConnWrapper) can satisfy all three interfaces.
type chirpStackConn interface {
	Conn() *grpc.ClientConn
	Close() error
}

// testConnRequest mirrors the wire body documented in RESEARCH §Pattern 14.
type testConnRequest struct {
	GRPCURL  string `json:"grpc_url"`
	APIToken string `json:"api_token"`
	MQTTURL  string `json:"mqtt_url"`
	MQTTUser string `json:"mqtt_user,omitempty"`
	MQTTPass string `json:"mqtt_pass,omitempty"`
}

// channelResult is one row of the two-channel probe. status is one of
// "reachable" | "unreachable" | "skipped" (UI-SPEC StatusRow variants verbatim).
type channelResult struct {
	Status    string `json:"status"`
	LatencyMs *int   `json:"latency_ms,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type testConnResponse struct {
	GRPC channelResult `json:"grpc"`
	MQTT channelResult `json:"mqtt"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// ensureCSRF is the canonical CSRF guard inherited from Plan 09 / Plan 15.
// Combined with SameSite=Lax cookies, the X-Requested-With header defeats
// classic cross-site form CSRF without per-request token plumbing (ASVS V13).
func ensureCSRF(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("X-Requested-With"), "shifter")
}

// TestConnHandler — POST /api/settings/chirpstack/test (CHIRP-03).
//
// Two-channel probe (RESEARCH §Pattern 14):
//  1. gRPC: dial + ProbeVersion. v3 → "unreachable" with v3 detail; refuse to
//     probe MQTT in that case (UI-SPEC contract — operator must fix gRPC first).
//  2. MQTT: PingMQTT (connect-only). Skipped when gRPC failed first.
//
// The response is always 200; failure is encoded in the channel results so
// the SPA can render both rows uniformly via StatusRow.
func TestConnHandler(deps TestConnDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ensureCSRF(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_csrf_header"})
			return
		}
		var req testConnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()

		resp := testConnResponse{}

		// Channel 1: gRPC + ProbeVersion
		t0 := time.Now()
		cfg := config.CSConfig{GRPCURL: req.GRPCURL, APIToken: req.APIToken, Insecure: true}
		conn, err := deps.Dial(ctx, cfg)
		if err != nil {
			ms := int(time.Since(t0).Milliseconds())
			resp.GRPC = channelResult{Status: "unreachable", LatencyMs: &ms, Detail: err.Error()}
			resp.MQTT = channelResult{Status: "skipped", Detail: "gRPC failed first"}
			writeJSON(w, http.StatusOK, resp)
			return
		}
		defer conn.Close()
		grpcConn := conn.Conn()
		version, err := chirpstack.ProbeVersion(ctx, grpcConn)
		ms := int(time.Since(t0).Milliseconds())
		if errors.Is(err, chirpstack.ErrChirpStackV3OrUnknown) {
			resp.GRPC = channelResult{
				Status:    "unreachable",
				LatencyMs: &ms,
				Detail:    "ChirpStack v3 detected — Shifter requires v4",
			}
			resp.MQTT = channelResult{Status: "skipped", Detail: "gRPC failed first"}
			writeJSON(w, http.StatusOK, resp)
			return
		}
		if err != nil {
			resp.GRPC = channelResult{Status: "unreachable", LatencyMs: &ms, Detail: err.Error()}
			resp.MQTT = channelResult{Status: "skipped", Detail: "gRPC failed first"}
			writeJSON(w, http.StatusOK, resp)
			return
		}
		resp.GRPC = channelResult{Status: "reachable", LatencyMs: &ms, Detail: "ChirpStack " + version}

		// Channel 2: MQTT
		t1 := time.Now()
		if err := deps.PingMQTT(ctx, req.MQTTURL, req.MQTTUser, req.MQTTPass); err != nil {
			ms2 := int(time.Since(t1).Milliseconds())
			resp.MQTT = channelResult{Status: "unreachable", LatencyMs: &ms2, Detail: err.Error()}
		} else {
			ms2 := int(time.Since(t1).Milliseconds())
			resp.MQTT = channelResult{Status: "reachable", LatencyMs: &ms2, Detail: "Mosquitto reachable"}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// ProductionDial wraps chirpstack.Dial in a chirpStackConn-shaped adapter so
// Plan 18 can plug a single value into Deps.Dial without `interface{}` round-
// trip. The same wrapper shape is reused by Plan 15's install package
// (csConn) and Plan 18's csBootConn.
func ProductionDial(ctx context.Context, cfg config.CSConfig) (chirpStackConn, error) {
	conn, err := chirpstack.Dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &realConnWrapper{c: conn}, nil
}

type realConnWrapper struct{ c *grpc.ClientConn }

func (r *realConnWrapper) Conn() *grpc.ClientConn { return r.c }
func (r *realConnWrapper) Close() error           { return r.c.Close() }

// GetChirpStackHandler — GET /api/settings/chirpstack.
//
// Returns mode + grpc_url + mqtt_url + mqtt_user + region. NEVER returns
// api_token / api_token_ref / mqtt_password_ref (T-17-01 / ASVS V8). Operators
// who need to update credentials use the Edit dialog → PUT path.
func GetChirpStackHandler(deps TestConnDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row := deps.Pool.QueryRow(r.Context(),
			`SELECT mode::text, grpc_url, mqtt_url, mqtt_user, region_name, region_common_name
			   FROM chirpstack_connection WHERE id = 1`)
		var mode, grpcURL, mqttURL, regionName, regionCommon string
		var mqttUser *string
		if err := row.Scan(&mode, &grpcURL, &mqttURL, &mqttUser, &regionName, &regionCommon); err != nil {
			deps.Log.Error("get chirpstack settings", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		userOut := ""
		if mqttUser != nil {
			userOut = *mqttUser
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"mode":      mode,
			"grpc_url":  grpcURL,
			"mqtt_url":  mqttURL,
			"mqtt_user": userOut,
			"region": map[string]string{
				"name":        regionName,
				"common_name": regionCommon,
			},
		})
	}
}

// putChirpStackRequest is the wire body for SETT-03. api_token is OPTIONAL —
// when omitted, the existing api_token_ref on disk is left in place.
type putChirpStackRequest struct {
	Mode             string `json:"mode"`
	GRPCURL          string `json:"grpc_url"`
	APIToken         string `json:"api_token,omitempty"`
	MQTTURL          string `json:"mqtt_url"`
	MQTTUser         string `json:"mqtt_user,omitempty"`
	MQTTPassword     string `json:"mqtt_password,omitempty"`
	RegionName       string `json:"region_name"`
	RegionCommonName string `json:"region_common_name"`
}

// PutChirpStackHandler — PUT /api/settings/chirpstack (SETT-03 + Open Q 2).
//
// Re-runs Dial + ProbeVersion + PingMQTT BEFORE persisting the update — Open
// Question 2 recommendation. Rejects with 422 if any probe fails. This is the
// admin's safety net: if they save a v3 URL or a typo'd MQTT host the change
// never lands and the prior good config keeps Shifter running.
//
// Error mapping:
//   - dial / probe failure → 422 grpc_unreachable
//   - probe returns ErrChirpStackV3OrUnknown → 422 v3_detected
//   - mqtt PingMQTT failure → 422 mqtt_unreachable
//   - secrets-dir write failure → 500 secret_write
func PutChirpStackHandler(deps TestConnDeps, secretsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ensureCSRF(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_csrf_header"})
			return
		}
		var req putChirpStackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
			return
		}
		if req.Mode != "bundled" && req.Mode != "external" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid_mode"})
			return
		}
		if req.GRPCURL == "" || req.MQTTURL == "" || req.RegionName == "" || req.RegionCommonName == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "missing_fields"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		cfg := config.CSConfig{GRPCURL: req.GRPCURL, APIToken: req.APIToken, Insecure: true}
		conn, err := deps.Dial(ctx, cfg)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "grpc_unreachable", "detail": err.Error(),
			})
			return
		}
		defer conn.Close()
		if _, err := chirpstack.ProbeVersion(ctx, conn.Conn()); err != nil {
			if errors.Is(err, chirpstack.ErrChirpStackV3OrUnknown) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "v3_detected"})
				return
			}
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "grpc_unreachable", "detail": err.Error(),
			})
			return
		}
		if err := deps.PingMQTT(ctx, req.MQTTURL, req.MQTTUser, req.MQTTPassword); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "mqtt_unreachable", "detail": err.Error(),
			})
			return
		}

		// Persist secrets by REF if the operator supplied new values. An empty
		// api_token in the request means "keep the existing token on disk" —
		// don't rewrite the file, don't change api_token_ref.
		var apiTokenRef string
		if req.APIToken != "" {
			apiTokenRef, err = writeSecret(secretsDir, "chirpstack_api_token", req.APIToken)
			if err != nil {
				deps.Log.Error("put chirpstack secret write", "err", err, "name", "chirpstack_api_token")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "secret_write"})
				return
			}
		}
		var mqttPasswordRef string
		if req.MQTTPassword != "" {
			mqttPasswordRef, err = writeSecret(secretsDir, "mqtt_password", req.MQTTPassword)
			if err != nil {
				deps.Log.Error("put chirpstack secret write", "err", err, "name", "mqtt_password")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "secret_write"})
				return
			}
		}

		// UPDATE — only set api_token_ref when a new token was provided
		// (otherwise keep the existing value). Same for mqtt_password_ref.
		if apiTokenRef != "" && mqttPasswordRef != "" {
			_, err = deps.Pool.Exec(ctx,
				`UPDATE chirpstack_connection SET
				    mode = $1::chirpstack_mode, grpc_url = $2, mqtt_url = $3,
				    mqtt_user = $4, region_name = $5, region_common_name = $6,
				    api_token_ref = $7, mqtt_password_ref = $8
				  WHERE id = 1`,
				req.Mode, req.GRPCURL, req.MQTTURL, nullable(req.MQTTUser),
				req.RegionName, req.RegionCommonName, apiTokenRef, mqttPasswordRef)
		} else if apiTokenRef != "" {
			_, err = deps.Pool.Exec(ctx,
				`UPDATE chirpstack_connection SET
				    mode = $1::chirpstack_mode, grpc_url = $2, mqtt_url = $3,
				    mqtt_user = $4, region_name = $5, region_common_name = $6,
				    api_token_ref = $7
				  WHERE id = 1`,
				req.Mode, req.GRPCURL, req.MQTTURL, nullable(req.MQTTUser),
				req.RegionName, req.RegionCommonName, apiTokenRef)
		} else if mqttPasswordRef != "" {
			_, err = deps.Pool.Exec(ctx,
				`UPDATE chirpstack_connection SET
				    mode = $1::chirpstack_mode, grpc_url = $2, mqtt_url = $3,
				    mqtt_user = $4, region_name = $5, region_common_name = $6,
				    mqtt_password_ref = $7
				  WHERE id = 1`,
				req.Mode, req.GRPCURL, req.MQTTURL, nullable(req.MQTTUser),
				req.RegionName, req.RegionCommonName, mqttPasswordRef)
		} else {
			_, err = deps.Pool.Exec(ctx,
				`UPDATE chirpstack_connection SET
				    mode = $1::chirpstack_mode, grpc_url = $2, mqtt_url = $3,
				    mqtt_user = $4, region_name = $5, region_common_name = $6
				  WHERE id = 1`,
				req.Mode, req.GRPCURL, req.MQTTURL, nullable(req.MQTTUser),
				req.RegionName, req.RegionCommonName)
		}
		if err != nil {
			deps.Log.Error("put chirpstack persist", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// writeSecret writes value to {dir}/{name} (mode 0600) and returns the
// absolute path. Mirrors install.writeSecret — kept duplicated so the http
// package doesn't import internal/install (which would create a cycle once
// Plan 18 wires both behind chi). One day a shared `internal/secrets` package
// can absorb both copies.
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

// nullable returns nil for an empty string so pgx writes SQL NULL instead of
// the empty string. Mirrors install.nullable.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
