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

// ActivateDeviceInput is the ABP activation payload (Plan 03-XX add-device
// ABP path). NwkSKey is copied into NwkSEncKey, SNwkSIntKey, and FNwkSIntKey
// for LoRaWAN 1.0.x compatibility — see device.pb.go lines 1178-1185: CS
// stores the three separate keys natively for 1.1.x but for 1.0.x devices
// (the only profile Shifter targets in v1) all three MUST equal NwkSKey.
//
// FCntUp / FCntDown are the operator-typed frame counters (typically 0 for a
// fresh ABP device); NFCntDown maps to FCntDown — see DeviceActivation proto
// for the asymmetric naming. AFCntDown defaults to 0 since 1.0.x devices use
// a single downlink counter.
type ActivateDeviceInput struct {
	DevEUI   string // lowercase 16-hex
	DevAddr  string // 8-hex
	NwkSKey  string // 32-hex (LoRaWAN 1.0.x — also used for NwkSEnc/SNwkSInt/FNwkSInt)
	AppSKey  string // 32-hex
	FCntUp   uint32
	FCntDown uint32 // mapped to NFCntDown on the wire
}

// ActivateDevice calls api.DeviceService/Activate (ABP path). For 1.0.x
// devices the wrapper sets NwkSEncKey = SNwkSIntKey = FNwkSIntKey = NwkSKey
// (canonical 1.0.x compat copy). Returns ErrNotFound if the underlying
// device doesn't exist in CS — handlers should branch on errors.Is.
func (c *Client) ActivateDevice(ctx context.Context, in ActivateDeviceInput) error {
	if in.DevEUI == "" {
		return fmt.Errorf("ActivateDevice: DevEUI is required")
	}
	if in.DevAddr == "" {
		return fmt.Errorf("ActivateDevice: DevAddr is required")
	}
	if in.NwkSKey == "" {
		return fmt.Errorf("ActivateDevice: NwkSKey is required")
	}
	if in.AppSKey == "" {
		return fmt.Errorf("ActivateDevice: AppSKey is required")
	}
	svc := api.NewDeviceServiceClient(c.conn)
	_, err := svc.Activate(ctx, &api.ActivateDeviceRequest{
		DeviceActivation: &api.DeviceActivation{
			DevEui:      in.DevEUI,
			DevAddr:     in.DevAddr,
			AppSKey:     in.AppSKey,
			NwkSEncKey:  in.NwkSKey, // 1.0.x copy
			SNwkSIntKey: in.NwkSKey, // 1.0.x copy
			FNwkSIntKey: in.NwkSKey, // 1.0.x copy
			FCntUp:      in.FCntUp,
			NFCntDown:   in.FCntDown,
			AFCntDown:   0,
		},
	})
	return wrapCSErr(err)
}

// DeviceKeys is the OTAA key reveal payload (D-27 reveal-secrets endpoint
// consumes this). Mirrors api.DeviceKeys but in domain-package shape so
// handlers don't import api.*. JoinNonce is intentionally NOT exposed —
// proto v4.17 doesn't carry it on DeviceKeys (verified at proto level).
type DeviceKeys struct {
	DevEUI string
	AppKey string // 32-hex
	NwkKey string // 32-hex (= AppKey for 1.0.x per CreateDeviceKeys invariant)
}

// GetDeviceKeys calls api.DeviceService/GetKeys. Returns ErrNotFound when CS
// has no recorded OTAA keys for the DevEUI (typical for ABP-only devices) —
// the reveal handler (D-27) maps this to HTTP 409 no_credentials.
func (c *Client) GetDeviceKeys(ctx context.Context, devEUI string) (*DeviceKeys, error) {
	if devEUI == "" {
		return nil, fmt.Errorf("GetDeviceKeys: devEUI is required")
	}
	svc := api.NewDeviceServiceClient(c.conn)
	resp, err := svc.GetKeys(ctx, &api.GetDeviceKeysRequest{DevEui: devEUI})
	if err != nil {
		return nil, wrapCSErr(err)
	}
	k := resp.GetDeviceKeys()
	if k == nil {
		return nil, ErrNotFound
	}
	return &DeviceKeys{
		DevEUI: k.GetDevEui(),
		AppKey: k.GetAppKey(),
		NwkKey: k.GetNwkKey(),
	}, nil
}

// DeviceActivation is the ABP session reveal payload (D-27). NwkSKey surfaces
// the NwkSEncKey field for 1.0.x devices (the wrapper does not re-validate
// the 3-key copy invariant since that's CS's responsibility post-Activate).
type DeviceActivation struct {
	DevEUI    string
	DevAddr   string
	NwkSKey   string // surfaced from NwkSEncKey for 1.0.x devices
	AppSKey   string
	FCntUp    uint32
	NFCntDown uint32
	AFCntDown uint32
}

// GetDeviceActivation calls api.DeviceService/GetActivation. Two distinct
// "no activation" surfaces are normalised to ErrNotFound:
//
//  1. CS returns codes.NotFound (the canonical case for OTAA-only devices
//     that haven't joined yet).
//  2. CS returns OK but DeviceActivation == nil or DevAddr == "" (the
//     edge case for a device row that exists but has no session — observed
//     in CS v4 when the join-server hasn't acked the device yet).
//
// Both surfaces represent "no credentials available" to the handler, so
// folding them is correct for the reveal-secrets endpoint.
func (c *Client) GetDeviceActivation(ctx context.Context, devEUI string) (*DeviceActivation, error) {
	if devEUI == "" {
		return nil, fmt.Errorf("GetDeviceActivation: devEUI is required")
	}
	svc := api.NewDeviceServiceClient(c.conn)
	resp, err := svc.GetActivation(ctx, &api.GetDeviceActivationRequest{DevEui: devEUI})
	if err != nil {
		return nil, wrapCSErr(err)
	}
	act := resp.GetDeviceActivation()
	if act == nil || act.GetDevAddr() == "" {
		return nil, ErrNotFound
	}
	return &DeviceActivation{
		DevEUI:    act.GetDevEui(),
		DevAddr:   act.GetDevAddr(),
		NwkSKey:   act.GetNwkSEncKey(), // surface as NwkSKey for 1.0.x
		AppSKey:   act.GetAppSKey(),
		FCntUp:    act.GetFCntUp(),
		NFCntDown: act.GetNFCntDown(),
		AFCntDown: act.GetAFCntDown(),
	}, nil
}
