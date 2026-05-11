package chirpstack

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"github.com/stretchr/testify/require"
)

const (
	testDevEUI  = "0102030405060708"
	testDevEUI2 = "1112131415161718"
	testAppKey  = "00112233445566778899AABBCCDDEEFF"
)

// TestCreateDevice_Standalone — Create only (no keys); verifies the gRPC
// Create call is issued exactly once and the device is then Get-able.
func TestCreateDevice_Standalone(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		Name:            "meter-001",
		Description:     "test water meter",
		AppKey:          testAppKey,
	}
	require.NoError(t, cli.CreateDevice(ctx, in))
	require.Equal(t, int64(1), h.Device.CreateCalls())

	dev, err := cli.GetDevice(ctx, testDevEUI)
	require.NoError(t, err)
	require.Equal(t, "meter-001", dev.GetName())
	require.Equal(t, "app-id", dev.GetApplicationId())
	require.Equal(t, "dp-id", dev.GetDeviceProfileId())
	require.Equal(t, "0000000000000000", dev.GetJoinEui(),
		"JoinEUI must default to the 1.0.x sentinel when input is empty")
}

// TestCreateDevice_AlsoCreatesKeys — CreateDeviceWithKeys issues Create THEN
// CreateKeys exactly once each, in that order. CHIRP-04 atomic flow.
func TestCreateDevice_AlsoCreatesKeys(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateDeviceWithKeys(ctx, CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		Name:            "meter-002",
		AppKey:          testAppKey,
	}))
	require.Equal(t, int64(1), h.Device.CreateCalls(),
		"Create must run exactly once")
	require.Equal(t, int64(1), h.Device.KeysCalls(),
		"CreateKeys must run exactly once")
}

// TestCreateDevice_DupeReturnsError — second Create with same DevEUI errors
// (mock returns codes.AlreadyExists). Caller error is non-nil.
func TestCreateDevice_DupeReturnsError(t *testing.T) {
	conn, _ := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		Name:            "first",
		AppKey:          testAppKey,
	}
	require.NoError(t, cli.CreateDevice(ctx, in))

	in.Name = "second"
	err := cli.CreateDevice(ctx, in)
	require.Error(t, err, "second create with same DevEUI must fail")
	require.True(t,
		strings.Contains(err.Error(), "AlreadyExists") ||
			strings.Contains(err.Error(), "already exists"),
		"error must surface the AlreadyExists code (got %v)", err)
}

// TestCreateDevice_RollbackOnKeysFailure — when CreateKeys fails (we simulate
// by pre-seeding the keys map ourselves so the second CreateKeys call returns
// AlreadyExists), CreateDeviceWithKeys must call DeleteDevice as best-effort
// cleanup so CS doesn't end up with an orphaned device. The outer error from
// CreateKeys is returned; the cleanup error is intentionally swallowed.
func TestCreateDevice_RollbackOnKeysFailure(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First, seed a complete device + keys row with the testDevEUI so the
	// next CreateDevice attempt with the SAME DevEUI fails — we want to
	// observe that CreateDeviceWithKeys cleanly surfaces the error path.
	// (The rollback path itself is exercised by the AppKey="" branch below,
	// which fails on the keys precondition AFTER the device is created.)
	require.NoError(t, cli.CreateDeviceWithKeys(ctx, CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		AppKey:          testAppKey,
	}))

	// Now pass DevEUI2 with empty AppKey: CreateDevice succeeds, CreateKeys
	// fails on the local "appKey is required" precondition before any RPC,
	// triggering the DeleteDevice cleanup branch.
	err := cli.CreateDeviceWithKeys(ctx, CreateDeviceInput{
		DevEUI:          testDevEUI2,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		AppKey:          "", // forces the keys-create precondition to fail
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "appKey is required")
	require.Equal(t, int64(1), h.Device.DeleteCalls(),
		"failed keys-create must trigger best-effort DeleteDevice cleanup")
}

// TestDeleteDevice — straight-through Delete + idempotency-by-NotFound.
func TestDeleteDevice(t *testing.T) {
	conn, _ := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateDevice(ctx, CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		AppKey:          testAppKey,
	}))
	require.NoError(t, cli.DeleteDevice(ctx, testDevEUI))

	err := cli.DeleteDevice(ctx, testDevEUI)
	require.Error(t, err, "second delete must surface NotFound — caller decides what to do with it")
}

const (
	testDevAddr = "01020304"
	testNwkSKey = "112233445566778899aabbccddeeff00"
	testAppSKey = "ffeeddccbbaa99887766554433221100"
)

// TestActivateDevice — ABP activation. The wrapper MUST copy NwkSKey into
// NwkSEncKey, SNwkSIntKey, and FNwkSIntKey for LoRaWAN 1.0.x compatibility
// (device.pb.go lines 1178-1185 — CS requires all three set to NwkSKey for
// 1.0.x devices).
func TestActivateDevice(t *testing.T) {
	conn, _ := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateDevice(ctx, CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		Name:            "abp-meter",
		AppKey:          testAppKey,
	}))

	require.NoError(t, cli.ActivateDevice(ctx, ActivateDeviceInput{
		DevEUI:   testDevEUI,
		DevAddr:  testDevAddr,
		NwkSKey:  testNwkSKey,
		AppSKey:  testAppSKey,
		FCntUp:   0,
		FCntDown: 0,
	}))

	// Read back the activation via the proto-level API to confirm the 1.0.x
	// 3-key copy semantics: NwkSEncKey == SNwkSIntKey == FNwkSIntKey == NwkSKey.
	svc := api.NewDeviceServiceClient(conn)
	resp, err := svc.GetActivation(ctx, &api.GetDeviceActivationRequest{DevEui: testDevEUI})
	require.NoError(t, err)
	require.NotNil(t, resp.DeviceActivation)
	require.Equal(t, testDevAddr, resp.DeviceActivation.DevAddr)
	require.Equal(t, testAppSKey, resp.DeviceActivation.AppSKey)
	require.Equal(t, testNwkSKey, resp.DeviceActivation.NwkSEncKey,
		"NwkSEncKey must equal NwkSKey for 1.0.x compat")
	require.Equal(t, testNwkSKey, resp.DeviceActivation.SNwkSIntKey,
		"SNwkSIntKey must equal NwkSKey for 1.0.x compat")
	require.Equal(t, testNwkSKey, resp.DeviceActivation.FNwkSIntKey,
		"FNwkSIntKey must equal NwkSKey for 1.0.x compat")
}

// TestGetDeviceKeys — OTAA key reveal. Returns DeviceKeys struct mirroring
// the proto. NotFound translates to ErrNotFound (handlers surface as 409
// no_credentials per D-27).
func TestGetDeviceKeys(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateDeviceWithKeys(ctx, CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		Name:            "otaa-meter",
		AppKey:          testAppKey,
	}))
	require.Equal(t, int64(1), h.Device.KeysCalls())

	keys, err := cli.GetDeviceKeys(ctx, testDevEUI)
	require.NoError(t, err)
	require.Equal(t, testDevEUI, keys.DevEUI)
	require.Equal(t, testAppKey, keys.AppKey)
	require.Equal(t, testAppKey, keys.NwkKey, "NwkKey = AppKey for 1.0.x devices (Plan 02-10 invariant)")

	// Unknown DevEUI → ErrNotFound.
	_, err = cli.GetDeviceKeys(ctx, testDevEUI2)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound),
		"GetDeviceKeys on unknown DevEUI must return ErrNotFound (got %v)", err)
}

// TestGetDeviceActivation — ABP session reveal. Pre-seed via mock helper
// so we don't depend on a prior ActivateDevice call ordering.
func TestGetDeviceActivation(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Seed device + activation directly in the mock.
	require.NoError(t, cli.CreateDevice(ctx, CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		AppKey:          testAppKey,
	}))
	h.Device.SeedDeviceActivation(testDevEUI, &api.DeviceActivation{
		DevEui:      testDevEUI,
		DevAddr:     testDevAddr,
		AppSKey:     testAppSKey,
		NwkSEncKey:  testNwkSKey,
		SNwkSIntKey: testNwkSKey,
		FNwkSIntKey: testNwkSKey,
		FCntUp:      42,
		NFCntDown:   7,
		AFCntDown:   3,
	})

	act, err := cli.GetDeviceActivation(ctx, testDevEUI)
	require.NoError(t, err)
	require.Equal(t, testDevEUI, act.DevEUI)
	require.Equal(t, testDevAddr, act.DevAddr)
	require.Equal(t, testNwkSKey, act.NwkSKey, "wrapper surfaces NwkSEncKey as NwkSKey for 1.0.x")
	require.Equal(t, testAppSKey, act.AppSKey)
	require.Equal(t, uint32(42), act.FCntUp)
	require.Equal(t, uint32(7), act.NFCntDown)
	require.Equal(t, uint32(3), act.AFCntDown)

	// Unknown DevEUI → ErrNotFound.
	_, err = cli.GetDeviceActivation(ctx, testDevEUI2)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound))
}

// TestGetDeviceActivation_EmptyResponseReturnsNotFound — CS v4 occasionally
// returns OK with a nil DeviceActivation for an OTAA device that has never
// joined. The wrapper translates this into ErrNotFound so handlers branch
// identically regardless of whether the no-credentials condition surfaces as
// codes.NotFound or empty-payload.
func TestGetDeviceActivation_EmptyResponseReturnsNotFound(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateDevice(ctx, CreateDeviceInput{
		DevEUI:          testDevEUI,
		ApplicationID:   "app-id",
		DeviceProfileID: "dp-id",
		AppKey:          testAppKey,
	}))
	// Seed an *empty* activation — DevEui set but no keys / addr.
	h.Device.SeedDeviceActivation(testDevEUI, &api.DeviceActivation{})

	_, err := cli.GetDeviceActivation(ctx, testDevEUI)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound),
		"empty DeviceActivation payload must be translated to ErrNotFound (got %v)", err)
}
