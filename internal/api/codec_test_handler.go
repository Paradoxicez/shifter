// Package api — thin HTTP handler shims that sit above packages with import
// constraints. This package can import both internal/profile and internal/ingest
// without creating a cycle (profile → ingest is the cycle direction; api imports
// both but is not imported by either).
package api

// codec_test_handler.go — POST /api/device-profiles/{id}/test-codec
//
// V2-VEND-02 backend half: runs operator-supplied codec JS inside the goja
// sandbox (internal/codec_runner) and returns decoded JSON + canonical mapping.
//
// Security:
//   - T-07-07-08: admin-only via ActionCodecTestRun; viewer → 403
//   - T-07-07-06: 256-byte payload cap enforced here AND inside RunCodecTest
//   - T-07-07-07: no audit row written (D-08 scratch-pad semantics)
//   - Per-user rate limit: 30 calls/min via inline token-bucket

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/codec_runner"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/ingest"
	"github.com/shifter-io/shifter/internal/profile"
)

// CodecTestDeps bundles the dependencies for the codec test handler.
type CodecTestDeps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger
}

// codecTestRequest is the JSON body for POST /api/device-profiles/{id}/test-codec.
type codecTestRequest struct {
	Hex   string `json:"hex"`
	FPort int    `json:"fPort"`
}

// codecTestLimiterMap is a per-user token-bucket rate limiter.
// 30 calls/min (T-07-07-05 mitigation).
type codecTestLimiterMap struct {
	mu sync.Mutex
	m  map[string]*codecTestEntry
}

type codecTestEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// globalCodecTestLimiter is the singleton rate limiter for this handler.
var globalCodecTestLimiter = &codecTestLimiterMap{m: map[string]*codecTestEntry{}}

func init() {
	// Background cleanup: evict stale buckets every 15 minutes.
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			globalCodecTestLimiter.mu.Lock()
			for k, e := range globalCodecTestLimiter.m {
				if time.Since(e.lastSeen) > time.Hour {
					delete(globalCodecTestLimiter.m, k)
				}
			}
			globalCodecTestLimiter.mu.Unlock()
		}
	}()
}

func (lm *codecTestLimiterMap) allow(userID string) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	e, ok := lm.m[userID]
	if !ok {
		// 30 calls/min: every 2s refill, burst=30 so a fresh user can burst
		// up to 30 before throttling.
		e = &codecTestEntry{
			lim: rate.NewLimiter(rate.Every(2*time.Second), 30),
		}
		lm.m[userID] = e
	}
	e.lastSeen = time.Now()
	return e.lim.Allow()
}

// RegisterCodecTestRoute mounts POST /api/device-profiles/{id}/test-codec on r.
// Admin-only (ActionCodecTestRun). Per-user rate limit 30/min.
func RegisterCodecTestRoute(r chi.Router, deps CodecTestDeps) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionCodecTestRun))
		rt.Post("/api/device-profiles/{id}/test-codec", CodecTestHandler(deps))
	})
}

// CodecTestHandler returns an http.HandlerFunc for the codec test endpoint.
func CodecTestHandler(deps CodecTestDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Per-user rate limit (T-07-07-05).
		if user, ok := auth.GetUser(r.Context(), deps.SessionMgr); ok {
			if !globalCodecTestLimiter.allow(user.ID) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}

		// Parse profile ID from path.
		idStr := chi.URLParam(r, "id")
		profileUUID, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, "bad profile id", http.StatusBadRequest)
			return
		}

		// Decode request body.
		var req codecTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Clean and validate hex input (whitespace / colon tolerant).
		hexClean := strings.Map(func(c rune) rune {
			if c == ' ' || c == '\n' || c == '\t' || c == ':' {
				return -1
			}
			return c
		}, req.Hex)
		payload, err := hex.DecodeString(hexClean)
		if err != nil {
			http.Error(w, "invalid hex", http.StatusBadRequest)
			return
		}
		if len(payload) > 256 {
			http.Error(w, "payload too large: max 256 bytes", http.StatusBadRequest)
			return
		}

		// Load profile (codec_js).
		q := sqlc.New(deps.Pool)
		pgID := pgtype.UUID{Bytes: profileUUID, Valid: true}
		profileRow, err := q.GetProfileForCodecTest(r.Context(), pgID)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}
		if err != nil {
			if deps.Log != nil {
				deps.Log.Error("codec test: load profile", "error", err)
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Load mappings for canonical normalization.
		mappings, _ := loadMappingsForCodecTest(r.Context(), deps.Pool, profileUUID)

		// Execute codec in sandbox (T-07-07-01/02/03/09).
		result := codec_runner.RunCodecTest(profileRow.CodecJs, payload, req.FPort)

		// If decode succeeded, derive canonical Layer1 mapping.
		if result.ErrorMessage == "" && result.DecodedJSON != nil && len(mappings) > 0 {
			layer1, normErr := ingest.NormalizeMeasurement(result.DecodedJSON, mappings)
			if normErr == nil {
				result.CanonicalMapping = layer1
			}
			// Normalization failure is non-fatal — surface decode result even
			// if canonical mapping is unavailable (profile has no mappings, or
			// decoded fields don't match any mapping target).
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil && deps.Log != nil {
			deps.Log.Error("codec test: encode response", "error", err)
		}
	}
}

// loadMappingsForCodecTest fetches device_profile_mapping rows and converts
// them to []profile.Mapping for ingest.NormalizeMeasurement.
func loadMappingsForCodecTest(ctx context.Context, pool *pgxpool.Pool, profileID uuid.UUID) ([]profile.Mapping, error) {
	q := sqlc.New(pool)
	rows, err := q.ListMappingsByProfile(ctx, pgtype.UUID{Bytes: profileID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]profile.Mapping, 0, len(rows))
	for _, row := range rows {
		m := profile.Mapping{
			JSONPointer: row.JsonPointer,
			Target:      row.Target,
			DataType:    row.DataType,
			Position:    row.Position,
		}
		if row.Scale.Valid {
			m.Scale = ingest.BigFloatFromNumeric(row.Scale)
			// BigFloatFromNumeric returns big.NewFloat(0) for invalid numeric;
			// treat scale=0 as identity (1) to avoid silently zeroing values.
			if m.Scale.Cmp(big.NewFloat(0)) == 0 {
				m.Scale = big.NewFloat(1)
			}
		}
		out = append(out, m)
	}
	return out, nil
}
