package chirpstack

import (
	"context"
	"fmt"
	"strings"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"github.com/chirpstack/chirpstack/api/go/v4/common"
)

// CreateProfileInput is the Shifter-side shape for DeviceProfileService.Create.
// Region/MACVersion/RegParamsRevision are passed as canonical lower-case
// identifier strings ("as923_2", "lorawan_1_0_3", "rp002_1_0_3") so callers
// don't have to import the chirpstack-api enum types — the architectural seam
// (only this package imports chirpstack-api/go/v4) stays intact.
type CreateProfileInput struct {
	TenantID          string
	Name              string
	Description       string
	Region            string // e.g. "AS923_2" (case-insensitive)
	MACVersion        string // e.g. "LORAWAN_1_0_3"
	RegParamsRevision string // e.g. "RP002_1_0_3"
	CodecJS           string // raw QuickJS source (D-09)
	UplinkInterval    uint32 // seconds; 0 → defaults to 900 (15 min)
	SupportsOTAA      bool
}

// UpdateProfileInput is the partial-update shape: caller supplies the CS
// profile UUID + the codec_js to push. We Get the existing record first and
// only mutate Name + PayloadCodecRuntime + PayloadCodecScript so unrelated
// fields (region, mac version, etc.) are not accidentally cleared by a
// half-formed Update.
type UpdateProfileInput struct {
	ID      string
	Name    string
	CodecJS string
}

// CreateDeviceProfile pushes a new profile to ChirpStack with a QuickJS codec
// (D-09: codec runs in CS sandbox, never in Shifter). Returns the CS profile
// UUID on success.
func (c *Client) CreateDeviceProfile(ctx context.Context, in CreateProfileInput) (string, error) {
	region, err := regionEnum(in.Region)
	if err != nil {
		return "", err
	}
	macV, err := macVersionEnum(in.MACVersion)
	if err != nil {
		return "", err
	}
	rev, err := regParamsRevisionEnum(in.RegParamsRevision)
	if err != nil {
		return "", err
	}

	uplink := in.UplinkInterval
	if uplink == 0 {
		uplink = 900 // 15 min default; profile editor in 02-08 lets admin override.
	}

	svc := api.NewDeviceProfileServiceClient(c.conn)
	resp, err := svc.Create(ctx, &api.CreateDeviceProfileRequest{
		DeviceProfile: &api.DeviceProfile{
			TenantId:            in.TenantID,
			Name:                in.Name,
			Description:         in.Description,
			Region:              region,
			MacVersion:          macV,
			RegParamsRevision:   rev,
			PayloadCodecRuntime: api.CodecRuntime_JS, // CONTEXT D-09
			PayloadCodecScript:  in.CodecJS,
			UplinkInterval:      uplink,
			SupportsOtaa:        in.SupportsOTAA,
		},
	})
	if err != nil {
		return "", fmt.Errorf("DeviceProfileService.Create: %w", err)
	}
	return resp.GetId(), nil
}

// UpdateDeviceProfile is the codec-sync path for Plan 02-08 profile-editor
// saves. Get-then-modify-then-Update is mandatory because CS v4's Update
// replaces the WHOLE record — calling Update with only the changed fields
// would clear Region / MacVersion / etc. and break every device bound to the
// profile.
func (c *Client) UpdateDeviceProfile(ctx context.Context, in UpdateProfileInput) error {
	if in.ID == "" {
		return fmt.Errorf("UpdateDeviceProfile: ID is required")
	}
	svc := api.NewDeviceProfileServiceClient(c.conn)

	existing, err := svc.Get(ctx, &api.GetDeviceProfileRequest{Id: in.ID})
	if err != nil {
		return fmt.Errorf("DeviceProfileService.Get: %w", err)
	}
	dp := existing.GetDeviceProfile()
	if dp == nil {
		return fmt.Errorf("DeviceProfileService.Get returned nil device_profile for id=%s", in.ID)
	}
	if in.Name != "" {
		dp.Name = in.Name
	}
	dp.PayloadCodecRuntime = api.CodecRuntime_JS
	dp.PayloadCodecScript = in.CodecJS

	if _, err := svc.Update(ctx, &api.UpdateDeviceProfileRequest{DeviceProfile: dp}); err != nil {
		return fmt.Errorf("DeviceProfileService.Update: %w", err)
	}
	return nil
}

// GetDeviceProfile returns the current CS profile by UUID. Used by tests +
// the future profile editor's "preview from CS" path.
func (c *Client) GetDeviceProfile(ctx context.Context, id string) (*api.DeviceProfile, error) {
	svc := api.NewDeviceProfileServiceClient(c.conn)
	resp, err := svc.Get(ctx, &api.GetDeviceProfileRequest{Id: id})
	if err != nil {
		return nil, fmt.Errorf("DeviceProfileService.Get: %w", err)
	}
	return resp.GetDeviceProfile(), nil
}

// regionEnum maps "AS923_2" / "as923_2" / etc. → common.Region. Case-insensitive
// because the value flows from chirpstack_connection.region_name (Phase 1
// stored it lowercase) but humans paste from CS docs in upper.
func regionEnum(s string) (common.Region, error) {
	switch strings.ToUpper(s) {
	case "EU868":
		return common.Region_EU868, nil
	case "US915":
		return common.Region_US915, nil
	case "CN779":
		return common.Region_CN779, nil
	case "EU433":
		return common.Region_EU433, nil
	case "CN470":
		return common.Region_CN470, nil
	case "AS923":
		return common.Region_AS923, nil
	case "AS923_2":
		return common.Region_AS923_2, nil
	case "AS923_3":
		return common.Region_AS923_3, nil
	case "AS923_4":
		return common.Region_AS923_4, nil
	case "KR920":
		return common.Region_KR920, nil
	case "IN865":
		return common.Region_IN865, nil
	case "RU864":
		return common.Region_RU864, nil
	case "ISM2400":
		return common.Region_ISM2400, nil
	default:
		return 0, fmt.Errorf("chirpstack: unknown region %q (must be one of EU868, US915, CN779, EU433, CN470, AS923, AS923_2..4, KR920, IN865, RU864, ISM2400)", s)
	}
}

// macVersionEnum maps "LORAWAN_1_0_3" / "lorawan_1_0_3" → common.MacVersion.
func macVersionEnum(s string) (common.MacVersion, error) {
	switch strings.ToUpper(s) {
	case "LORAWAN_1_0_0":
		return common.MacVersion_LORAWAN_1_0_0, nil
	case "LORAWAN_1_0_1":
		return common.MacVersion_LORAWAN_1_0_1, nil
	case "LORAWAN_1_0_2":
		return common.MacVersion_LORAWAN_1_0_2, nil
	case "LORAWAN_1_0_3":
		return common.MacVersion_LORAWAN_1_0_3, nil
	case "LORAWAN_1_0_4":
		return common.MacVersion_LORAWAN_1_0_4, nil
	case "LORAWAN_1_1_0":
		return common.MacVersion_LORAWAN_1_1_0, nil
	default:
		return 0, fmt.Errorf("chirpstack: unknown mac version %q", s)
	}
}

// regParamsRevisionEnum maps "RP002_1_0_3" / "A" / "B" → common.RegParamsRevision.
func regParamsRevisionEnum(s string) (common.RegParamsRevision, error) {
	switch strings.ToUpper(s) {
	case "A":
		return common.RegParamsRevision_A, nil
	case "B":
		return common.RegParamsRevision_B, nil
	case "RP002_1_0_0":
		return common.RegParamsRevision_RP002_1_0_0, nil
	case "RP002_1_0_1":
		return common.RegParamsRevision_RP002_1_0_1, nil
	case "RP002_1_0_2":
		return common.RegParamsRevision_RP002_1_0_2, nil
	case "RP002_1_0_3":
		return common.RegParamsRevision_RP002_1_0_3, nil
	case "RP002_1_0_4":
		return common.RegParamsRevision_RP002_1_0_4, nil
	case "RP002_1_0_5":
		return common.RegParamsRevision_RP002_1_0_5, nil
	default:
		return 0, fmt.Errorf("chirpstack: unknown reg params revision %q", s)
	}
}
