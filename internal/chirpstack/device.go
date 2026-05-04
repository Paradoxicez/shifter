package chirpstack

import (
	"context"
	"fmt"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
)

// CreateDeviceInput is the Shifter-side shape for the atomic add-device flow
// (CHIRP-04). Atomic semantics live in Plan 02-10's transaction wrapper; this
// type only models the gRPC payload.
//
// AppKey is the LoRaWAN 1.0.x AppKey (16 bytes / 32 hex chars). It is written
// to ChirpStack via CreateKeys but NEVER read back per DEV-09: Shifter does
// not persist it locally and does not expose a "show keys" endpoint. CS owns
// the canonical copy.
//
// JoinEUI defaults to "0000000000000000" (the legacy 1.0.x sentinel) when
// empty — caller can override for 1.1.x devices that pre-configure JoinEUI.
type CreateDeviceInput struct {
	DevEUI          string // lowercase 16-char hex
	ApplicationID   string // CS application UUID
	DeviceProfileID string // CS device-profile UUID
	Name            string
	Description     string
	AppKey          string // hex 32 chars; written to CS, never read back (DEV-09)
	JoinEUI         string // hex 16 chars; default "0000000000000000" for 1.0.x
}

const defaultJoinEUI = "0000000000000000"

// CreateDevice creates the Shifter-mapped device in ChirpStack. The handler
// MUST follow up with CreateDeviceKeys for OTAA — the two are separated so
// Plan 02-10's transaction wrapper can roll the device back via DeleteDevice
// if the keys-create fails. Most callers will use the convenience method
// CreateDeviceWithKeys defined below.
func (c *Client) CreateDevice(ctx context.Context, in CreateDeviceInput) error {
	if in.DevEUI == "" {
		return fmt.Errorf("CreateDevice: DevEUI is required")
	}
	if in.ApplicationID == "" {
		return fmt.Errorf("CreateDevice: ApplicationID is required")
	}
	if in.DeviceProfileID == "" {
		return fmt.Errorf("CreateDevice: DeviceProfileID is required")
	}
	joinEUI := in.JoinEUI
	if joinEUI == "" {
		joinEUI = defaultJoinEUI
	}

	svc := api.NewDeviceServiceClient(c.conn)
	if _, err := svc.Create(ctx, &api.CreateDeviceRequest{
		Device: &api.Device{
			DevEui:          in.DevEUI,
			ApplicationId:   in.ApplicationID,
			DeviceProfileId: in.DeviceProfileID,
			Name:            in.Name,
			Description:     in.Description,
			JoinEui:         joinEUI,
		},
	}); err != nil {
		return fmt.Errorf("DeviceService.Create: %w", err)
	}
	return nil
}

// CreateDeviceKeys writes OTAA keys for an existing CS device. Per LoRaWAN
// 1.0.x the operator only provides AppKey; we set NwkKey = AppKey because
// CS server-side splits the derivation for 1.1.x devices internally (RESEARCH
// Pattern 4). DevEUI of the device is required.
func (c *Client) CreateDeviceKeys(ctx context.Context, devEUI, appKey string) error {
	if devEUI == "" {
		return fmt.Errorf("CreateDeviceKeys: devEUI is required")
	}
	if appKey == "" {
		return fmt.Errorf("CreateDeviceKeys: appKey is required")
	}
	svc := api.NewDeviceServiceClient(c.conn)
	if _, err := svc.CreateKeys(ctx, &api.CreateDeviceKeysRequest{
		DeviceKeys: &api.DeviceKeys{
			DevEui: devEUI,
			AppKey: appKey,
			NwkKey: appKey, // 1.0.x compat per RESEARCH Pattern 4
		},
	}); err != nil {
		return fmt.Errorf("DeviceService.CreateKeys: %w", err)
	}
	return nil
}

// CreateDeviceWithKeys is the convenience wrapper Plan 02-10's atomic
// add-device transaction calls: CreateDevice → CreateDeviceKeys, with a
// best-effort DeleteDevice rollback if the keys call fails. The outer txn
// (Postgres BEGIN/COMMIT) still owns Shifter-side rollback; this helper
// only ensures CS-side state is not orphaned by a half-completed pair.
func (c *Client) CreateDeviceWithKeys(ctx context.Context, in CreateDeviceInput) error {
	if err := c.CreateDevice(ctx, in); err != nil {
		return err
	}
	if err := c.CreateDeviceKeys(ctx, in.DevEUI, in.AppKey); err != nil {
		// Best-effort cleanup of the orphaned device. Use a fresh context so
		// the cleanup attempt is not pre-cancelled by the caller's deadline.
		// We INTENTIONALLY swallow the delete error: the caller already has a
		// keys-create error to surface, and a noisy double-error obscures the
		// real failure. The orphaned device is benign — re-running the flow
		// for the same DevEUI will fail-fast with AlreadyExists, prompting
		// manual cleanup in CS UI.
		cleanup, cancel := context.WithTimeout(context.Background(), defaultCleanupTimeout)
		defer cancel()
		_ = c.DeleteDevice(cleanup, in.DevEUI)
		return err
	}
	return nil
}

// DeleteDevice removes the CS device + its keys. Used by Plan 02-10's swap
// rollback path and by the future decommission flow (D-15).
func (c *Client) DeleteDevice(ctx context.Context, devEUI string) error {
	if devEUI == "" {
		return fmt.Errorf("DeleteDevice: devEUI is required")
	}
	svc := api.NewDeviceServiceClient(c.conn)
	if _, err := svc.Delete(ctx, &api.DeleteDeviceRequest{DevEui: devEUI}); err != nil {
		return fmt.Errorf("DeviceService.Delete: %w", err)
	}
	return nil
}

// GetDevice returns the current CS device by DevEUI. Used by tests + the
// future device-detail page's "live status from CS" panel.
func (c *Client) GetDevice(ctx context.Context, devEUI string) (*api.Device, error) {
	if devEUI == "" {
		return nil, fmt.Errorf("GetDevice: devEUI is required")
	}
	svc := api.NewDeviceServiceClient(c.conn)
	resp, err := svc.Get(ctx, &api.GetDeviceRequest{DevEui: devEUI})
	if err != nil {
		return nil, fmt.Errorf("DeviceService.Get: %w", err)
	}
	return resp.GetDevice(), nil
}
