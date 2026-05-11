package dashboard

import (
	"encoding/json"
	"net/http"
	"sync"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// OnboardingState is the D-21 onboarding tuple returned by GET /api/dashboard/scope.
// The frontend uses these counts to decide which empty-state card to show.
type OnboardingState struct {
	GatewayCount int64 `json:"gateway_count"`
	DeviceCount  int64 `json:"device_count"`
	UplinkCount  int64 `json:"uplink_count"`
}

// ScopeResponse is the wire shape for GET /api/dashboard/scope.
type ScopeResponse struct {
	Capabilities string          `json:"capabilities"`
	Onboarding   OnboardingState `json:"onboarding"`
}

// handleScope serves GET /api/dashboard/scope.
//
// Runs GetCapabilities and OnboardingCounts in parallel via goroutines.
// Returns the scope JSON used by the frontend to mount the correct dashboard
// variant (D-09) and render the correct empty-state (D-21).
//
// D-23: mounted under authenticated group (any role).
// T-04-04-05: mounted under authenticated group; 401 for anonymous requests is
// enforced at the router level.
func (d Deps) handleScope(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := sqlc.New(d.Pool)

	var (
		capabilities string
		onboarding   sqlc.OnboardingCountsRow
		capsErr      error
		obErr        error
		wg           sync.WaitGroup
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		capabilities, capsErr = q.GetCapabilities(ctx)
	}()

	go func() {
		defer wg.Done()
		onboarding, obErr = q.OnboardingCounts(ctx)
	}()

	wg.Wait()

	if capsErr != nil {
		d.Logger.ErrorContext(ctx, "scope: get capabilities", "err", capsErr)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if obErr != nil {
		d.Logger.ErrorContext(ctx, "scope: onboarding counts", "err", obErr)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := ScopeResponse{
		Capabilities: capabilities,
		Onboarding: OnboardingState{
			GatewayCount: onboarding.GatewayCount,
			DeviceCount:  onboarding.DeviceCount,
			UplinkCount:  onboarding.UplinkCount,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		d.Logger.ErrorContext(ctx, "scope: encode response", "err", err)
	}
}
