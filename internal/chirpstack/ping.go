package chirpstack

import (
	"context"
	"fmt"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
)

// PingDevices is the CHIRP-01 smoke check: list one device via the gRPC client
// to prove the channel + auth interceptor + DeviceServiceClient are wired
// end-to-end. Plan 17 (Test Connection) calls this from the gRPC half of the
// wizard's connectivity probe.
//
// applicationID may be empty — ChirpStack v4 lists across all applications when
// the field is unset (per chirpstack-api docs). Limit is hardcoded to 1 because
// Phase 1 only needs to verify the call SUCCEEDS; deeper iteration belongs to
// Phase 2's device-management surface.
func (c *Client) PingDevices(ctx context.Context, applicationID string) error {
	cli := api.NewDeviceServiceClient(c.conn)
	_, err := cli.List(ctx, &api.ListDevicesRequest{
		ApplicationId: applicationID,
		Limit:         1,
	})
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}
	return nil
}
