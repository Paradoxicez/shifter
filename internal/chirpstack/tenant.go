package chirpstack

import (
	"context"
	"fmt"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
)

// tenantDescription is the deterministic description Shifter writes on every
// auto-managed tenant. The literal is asserted by Plan 02-05 acceptance
// criteria so a future refactor can't silently rebrand the bootstrap.
const tenantDescription = "Shifter-managed tenant — do not edit"

// EnsureTenant returns the ChirpStack v4 tenant UUID for `name`, creating the
// tenant if it does not already exist (D-28 idempotent bootstrap).
//
// The pattern is list-then-create: TenantService.List with Search=name first,
// looking for an EXACT name match in the result set; only when no match exists
// do we call Create. This is safer than swallowing AlreadyExists from Create
// because CS v4's Create allows duplicate names — relying on the AlreadyExists
// error code would silently let two tenants accumulate over restarts.
//
// On a transient List failure we fall through to Create (List failed → we
// don't know what's there → safest is to attempt creation; a duplicate-name
// outcome means we end up with a slightly extra tenant rather than a
// hard-fail boot). Caller upstream (bootstrap.go) persists the returned
// UUID immediately so subsequent boots short-circuit.
func (c *Client) EnsureTenant(ctx context.Context, name string) (string, error) {
	svc := api.NewTenantServiceClient(c.conn)

	list, err := svc.List(ctx, &api.ListTenantsRequest{
		Limit:  100,
		Search: name,
	})
	if err == nil {
		for _, t := range list.GetResult() {
			if t.GetName() == name {
				return t.GetId(), nil
			}
		}
	}

	resp, err := svc.Create(ctx, &api.CreateTenantRequest{
		Tenant: &api.Tenant{
			Name:                name,
			Description:         tenantDescription,
			CanHaveGateways:     true,
			PrivateGatewaysUp:   true,
			PrivateGatewaysDown: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("TenantService.Create: %w", err)
	}
	return resp.GetId(), nil
}
