package chirpstack

import (
	"context"
	"fmt"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
)

// applicationDescription is the deterministic description Shifter writes on
// the auto-managed `shifter` application. Asserted by Plan 02-05 acceptance.
const applicationDescription = "Shifter-managed application — do not edit"

// EnsureApplication returns the ChirpStack v4 application UUID for
// (tenantID, name), creating it if absent (D-28 idempotent bootstrap, scoped
// to a single tenant).
//
// Same list-then-create idiom as EnsureTenant; List filters by both TenantId
// and Search so we don't accidentally reuse an application of the same name
// in a different tenant (only relevant in external-CS mode where the tenant
// might pre-exist with shared application names).
func (c *Client) EnsureApplication(ctx context.Context, tenantID, name string) (string, error) {
	svc := api.NewApplicationServiceClient(c.conn)

	list, err := svc.List(ctx, &api.ListApplicationsRequest{
		Limit:    100,
		Search:   name,
		TenantId: tenantID,
	})
	if err == nil {
		for _, app := range list.GetResult() {
			if app.GetName() == name {
				return app.GetId(), nil
			}
		}
	}

	resp, err := svc.Create(ctx, &api.CreateApplicationRequest{
		Application: &api.Application{
			Name:        name,
			Description: applicationDescription,
			TenantId:    tenantID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("ApplicationService.Create: %w", err)
	}
	return resp.GetId(), nil
}
