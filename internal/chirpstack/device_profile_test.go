package chirpstack

import (
	"context"
	"testing"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"github.com/chirpstack/chirpstack/api/go/v4/common"
	"github.com/stretchr/testify/require"
)

// TestCreateDeviceProfile_RoundTrip — Create with codec_js="A", then Get with
// returned id, assert codec_js + runtime + region make it round-trip.
func TestCreateDeviceProfile_RoundTrip(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	tenantID := h.Tenant.SeedTenant("shifter-default")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const codec = "function decodeUplink(input) { return { data: input.bytes } }"
	id, err := cli.CreateDeviceProfile(ctx, CreateProfileInput{
		TenantID:          tenantID,
		Name:              "axioma-w1",
		Description:       "Axioma Qalcosonic W1",
		Region:            "AS923_2",
		MACVersion:        "LORAWAN_1_0_3",
		RegParamsRevision: "RP002_1_0_3",
		CodecJS:           codec,
		SupportsOTAA:      true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.Equal(t, int64(1), h.DeviceProfile.CreateCalls())

	dp, err := cli.GetDeviceProfile(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, dp)
	require.Equal(t, codec, dp.GetPayloadCodecScript(),
		"D-09: codec_js must round-trip through CS")
	require.Equal(t, api.CodecRuntime_JS, dp.GetPayloadCodecRuntime(),
		"runtime must be CodecRuntime_JS (D-09)")
	require.Equal(t, common.Region_AS923_2, dp.GetRegion())
	require.Equal(t, common.MacVersion_LORAWAN_1_0_3, dp.GetMacVersion())
	require.Equal(t, common.RegParamsRevision_RP002_1_0_3, dp.GetRegParamsRevision())
	require.Equal(t, uint32(900), dp.GetUplinkInterval(),
		"UplinkInterval default should be 900s when input is 0")
}

// TestUpdateDeviceProfile_PartialUpdate — Create with codec="A", Update with
// codec="B" via Get-then-modify-then-Update; Get again returns "B" and the
// Region (untouched by Update) is preserved.
func TestUpdateDeviceProfile_PartialUpdate(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	tenantID := h.Tenant.SeedTenant("shifter-default")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := cli.CreateDeviceProfile(ctx, CreateProfileInput{
		TenantID:          tenantID,
		Name:              "acrel-adw300",
		Region:            "AS923_2",
		MACVersion:        "LORAWAN_1_0_3",
		RegParamsRevision: "RP002_1_0_3",
		CodecJS:           "A",
	})
	require.NoError(t, err)

	err = cli.UpdateDeviceProfile(ctx, UpdateProfileInput{
		ID:      id,
		Name:    "acrel-adw300-renamed",
		CodecJS: "B",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), h.DeviceProfile.UpdateCalls())

	dp, err := cli.GetDeviceProfile(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "B", dp.GetPayloadCodecScript(), "codec must be the updated value")
	require.Equal(t, "acrel-adw300-renamed", dp.GetName())
	require.Equal(t, common.Region_AS923_2, dp.GetRegion(),
		"Region must be preserved across Get-modify-Update — D-09 codec sync must not clear immutable fields")
}

// TestCreateDeviceProfile_UnknownRegion — defensive enum mapping returns a
// wrapped error (not a panic) on unknown region strings.
func TestCreateDeviceProfile_UnknownRegion(t *testing.T) {
	conn, _ := dialMockConnWithHandles(t)
	cli := NewClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := cli.CreateDeviceProfile(ctx, CreateProfileInput{
		TenantID:          "tenant-id",
		Name:              "bad",
		Region:            "MARS868",
		MACVersion:        "LORAWAN_1_0_3",
		RegParamsRevision: "RP002_1_0_3",
		CodecJS:           "x",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown region")
}

// TestRegionEnumMapping — table-driven coverage of regionEnum / macVersionEnum
// / regParamsRevisionEnum so a future protobuf upgrade that renames an enum
// breaks the test rather than the production codec sync path.
func TestRegionEnumMapping(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want common.Region
	}{
		{"as923_2 lower", "as923_2", common.Region_AS923_2},
		{"AS923_2 upper", "AS923_2", common.Region_AS923_2},
		{"eu868", "eu868", common.Region_EU868},
		{"us915", "us915", common.Region_US915},
		{"in865", "IN865", common.Region_IN865},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := regionEnum(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	macTests := []struct {
		in   string
		want common.MacVersion
	}{
		{"LORAWAN_1_0_3", common.MacVersion_LORAWAN_1_0_3},
		{"lorawan_1_0_3", common.MacVersion_LORAWAN_1_0_3},
		{"LORAWAN_1_1_0", common.MacVersion_LORAWAN_1_1_0},
	}
	for _, tc := range macTests {
		t.Run("mac/"+tc.in, func(t *testing.T) {
			got, err := macVersionEnum(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	revTests := []struct {
		in   string
		want common.RegParamsRevision
	}{
		{"A", common.RegParamsRevision_A},
		{"B", common.RegParamsRevision_B},
		{"rp002_1_0_3", common.RegParamsRevision_RP002_1_0_3},
	}
	for _, tc := range revTests {
		t.Run("rev/"+tc.in, func(t *testing.T) {
			got, err := regParamsRevisionEnum(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
