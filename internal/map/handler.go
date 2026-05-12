package mapapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// Deps groups every dependency the map handler needs.
type Deps struct {
	// Pool is the shared pgxpool used by all map queries.
	Pool *pgxpool.Pool

	// Logger is a *slog.Logger (optionally namespaced with
	// slog.Logger.With("component", "mapapi")).
	Logger *slog.Logger

	// SessionMgr is the SCS session manager. Used by RegisterRoutes to wire
	// auth.RequireAction (T-05-04-01 — unauthenticated callers receive 401).
	SessionMgr *scs.SessionManager
}

// SiteMarker is the per-site JSON shape in the map response.
// Only sites with both lat AND lng set are included (MAP-01).
type SiteMarker struct {
	ID               uuid.UUID          `json:"id"`
	Name             string             `json:"name"`
	Lat              float64            `json:"lat"`
	Lng              float64            `json:"lng"`
	MPCount          int64              `json:"mp_count"`
	OnlineCount      int64              `json:"online_count"`
	OfflineCount     int64              `json:"offline_count"`
	TodayConsumption map[string]float64 `json:"today_consumption"` // keys subset of {water, electricity}
}

// GatewayMarker is the per-gateway JSON shape in the map response.
// Only gateways with both lat AND lng set are included (MAP-01).
type GatewayMarker struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Lat    float64   `json:"lat"`
	Lng    float64   `json:"lng"`
	Online bool      `json:"online"`
}

// Response is the wire shape for GET /api/map/data.
type Response struct {
	Sites    []SiteMarker    `json:"sites"`
	Gateways []GatewayMarker `json:"gateways"`
}

// DataHandler serves GET /api/map/data.
//
// It reads install_identity for timezone + capabilities, queries sites and
// gateways with lat/lng, computes per-site today's consumption from
// measurement_hourly, gates consumption keys by install capabilities (D-09),
// and returns the assembled JSON.
//
// T-05-04-01: mounted under auth.RequireAction(ActionSiteRead) in routes.go
// so unauthenticated requests receive 401 before reaching this handler.
// T-05-04-02: response is pure JSON of site/gateway data; no tile URL or API
// key is ever written into any query or response field (MAP-04 invariant).
func DataHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		q := sqlc.New(deps.Pool)

		// Read install_identity for timezone + capabilities (server-side only —
		// T-04-04-04: timezone never taken from the request).
		identity, err := q.GetInstallIdentity(ctx)
		if err != nil {
			if deps.Logger != nil {
				deps.Logger.ErrorContext(ctx, "map: get install identity", "err", err)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "identity_load_failed"})
			return
		}

		// Compute install-tz midnight today for the hourly CAGG window.
		tz, err := time.LoadLocation(identity.Timezone)
		if err != nil {
			tz = time.UTC
		}
		now := time.Now().In(tz)
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)

		sites, err := q.ListSitesForMap(ctx)
		if err != nil {
			if deps.Logger != nil {
				deps.Logger.ErrorContext(ctx, "map: list sites", "err", err)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sites_query_failed"})
			return
		}

		gateways, err := q.ListGatewaysForMap(ctx)
		if err != nil {
			if deps.Logger != nil {
				deps.Logger.ErrorContext(ctx, "map: list gateways", "err", err)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "gateways_query_failed"})
			return
		}

		consumptionRows, err := q.TodaySiteConsumption(ctx, midnight)
		if err != nil {
			if deps.Logger != nil {
				deps.Logger.ErrorContext(ctx, "map: today site consumption", "err", err)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "consumption_query_failed"})
			return
		}

		// Build today_consumption map per site, gated by capabilities (D-09).
		consumptionBySite := make(map[uuid.UUID]map[string]float64)
		for _, row := range consumptionRows {
			siteID := uuid.UUID(row.SiteID.Bytes)
			if !capabilityIncludes(identity.Capabilities, row.UtilityClass) {
				continue
			}
			if _, ok := consumptionBySite[siteID]; !ok {
				consumptionBySite[siteID] = make(map[string]float64)
			}
			consumptionBySite[siteID][row.UtilityClass] = float64(row.Consumption)
		}

		siteMarkers := make([]SiteMarker, 0, len(sites))
		for _, s := range sites {
			// sqlc returns *float64 for nullable columns; ListSitesForMap only
			// returns rows where lat IS NOT NULL AND lng IS NOT NULL, so these
			// dereferences are safe.
			var lat, lng float64
			if s.Lat != nil {
				lat = *s.Lat
			}
			if s.Lng != nil {
				lng = *s.Lng
			}

			siteID := uuid.UUID(s.ID.Bytes)
			consumption := consumptionBySite[siteID]
			if consumption == nil {
				consumption = make(map[string]float64)
			}

			siteMarkers = append(siteMarkers, SiteMarker{
				ID:               siteID,
				Name:             s.Name,
				Lat:              lat,
				Lng:              lng,
				MPCount:          s.MpCount,
				OnlineCount:      s.OnlineCount,
				OfflineCount:     s.OfflineCount,
				TodayConsumption: consumption,
			})
		}

		gatewayMarkers := make([]GatewayMarker, 0, len(gateways))
		for _, g := range gateways {
			var lat, lng float64
			if g.Lat != nil {
				lat = *g.Lat
			}
			if g.Lng != nil {
				lng = *g.Lng
			}
			online := g.Online != nil && *g.Online

			gatewayMarkers = append(gatewayMarkers, GatewayMarker{
				ID:     uuid.UUID(g.ID.Bytes),
				Name:   g.Name,
				Lat:    lat,
				Lng:    lng,
				Online: online,
			})
		}

		writeJSON(w, http.StatusOK, Response{
			Sites:    siteMarkers,
			Gateways: gatewayMarkers,
		})
	}
}

// capabilityIncludes returns true when installCaps covers utilityClass.
// Matches Phase 4 D-09 / Phase 5 D-04 capability semantics.
func capabilityIncludes(installCaps, utilityClass string) bool {
	switch installCaps {
	case "both":
		return true
	case "water":
		return utilityClass == "water"
	case "electricity":
		return utilityClass == "electricity"
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
