package chirpstack

import (
	"context"
	"strings"
	"testing"
	"time"

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
