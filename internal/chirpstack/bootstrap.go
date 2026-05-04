package chirpstack

import (
	"context"
	"fmt"
	"log/slog"
)

// CONTEXT D-28 names — every Shifter install lands a single "shifter-default"
// tenant + a single "shifter" application. Hardcoded constants instead of
// config values: Plan 02-05 explicitly rejected per-install renaming because
// (a) external-CS mode with an existing tenant is handled by reuse on List,
// not by renaming, and (b) divergent names break the documented "search the
// tenant in CS UI" operator runbook.
const (
	BootstrapTenantName      = "shifter-default"
	BootstrapApplicationName = "shifter"
)

// ConnectionStore is the persistence interface bootstrap.go consumes. Plan
// 02-06 implements it via the new sqlc query SetChirpStackTenantApp; Plan
// 02-05 tests use a fake in-memory implementation. Read returns the existing
// (cs_tenant_id, cs_application_id) from chirpstack_connection — empty
// strings indicate "not yet bootstrapped" for the corresponding entity.
type ConnectionStore interface {
	GetCSConnection(ctx context.Context) (csTenantID, csApplicationID string, err error)
	SetCSTenantApp(ctx context.Context, tenantID, applicationID string) error
}

// EnsureTenantAndApplication is the D-28 first-boot routine.
//
// Behaviour:
//
//  1. Read the persisted (cs_tenant_id, cs_application_id) from
//     chirpstack_connection.
//  2. If both are non-empty → short-circuit; we're already bootstrapped.
//     This is the steady-state path for every boot after the first one.
//  3. If only the tenant is missing → call EnsureTenant(BootstrapTenantName).
//     Reuse-or-create per CONTEXT D-28.
//  4. If only the application is missing → call EnsureApplication(t, "shifter").
//  5. Persist the (now both non-empty) UUIDs back via SetCSTenantApp.
//
// On Pitfall #9: callers (cmd/shifter serve in Plan 02-12 wiring) should NOT
// fail boot on a transient CS-unreachable error here. The resolver +
// already-bound device ingest path do not depend on the global tenant +
// application UUIDs at runtime — only the FIRST add-device flow does. So a
// recommended pattern is to call this routine on serve startup, log the
// returned error as a warning, and proceed with degraded functionality;
// the first add-device attempt after CS recovers will trigger the bootstrap
// again (idempotently).
func EnsureTenantAndApplication(ctx context.Context, c *Client, store ConnectionStore, log *slog.Logger) (tenantID, appID string, err error) {
	if log == nil {
		log = slog.Default()
	}

	curT, curA, err := store.GetCSConnection(ctx)
	if err != nil {
		return "", "", fmt.Errorf("read chirpstack_connection: %w", err)
	}

	if curT != "" && curA != "" {
		log.Debug("chirpstack bootstrap skipped — already bootstrapped",
			"cs_tenant_id", curT, "cs_application_id", curA)
		return curT, curA, nil
	}

	if curT == "" {
		curT, err = c.EnsureTenant(ctx, BootstrapTenantName)
		if err != nil {
			return "", "", fmt.Errorf("ensure tenant %q: %w", BootstrapTenantName, err)
		}
	}

	if curA == "" {
		curA, err = c.EnsureApplication(ctx, curT, BootstrapApplicationName)
		if err != nil {
			return "", "", fmt.Errorf("ensure application %q: %w", BootstrapApplicationName, err)
		}
	}

	if err := store.SetCSTenantApp(ctx, curT, curA); err != nil {
		return "", "", fmt.Errorf("persist cs ids: %w", err)
	}

	log.Info("chirpstack bootstrap complete",
		"cs_tenant_id", curT, "cs_application_id", curA)
	return curT, curA, nil
}
