package chirpstack

// Phase 3 Plan 03-03 — GatewayService wrappers (6 RPCs).
//
// [VERIFIED rename] ChirpStack v4.17 method is `GetMetrics`, NOT the obsolete
// pre-v4 stats RPC name CONTEXT D-02 still uses. The wrapper method name
// matches the proto (api.GatewayServiceClient.GetMetrics) so we don't carry an
// obsolete identifier through the call chain. The CACHE strategy
// (1-minute TTL, 24h window, hourly buckets) remains CONTEXT-authoritative —
// it's wired in gateway_metrics_cache.go.
//
// Same architectural seam as Phase 2: this package is the SOLE importer of
// github.com/chirpstack/chirpstack/api/go/v4. Every handler that talks to
// ChirpStack must go through these wrappers — no handler imports api.* directly.

import (
	"context"
	"fmt"
	"strings"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	common "github.com/chirpstack/chirpstack/api/go/v4/common"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CreateGatewayInput is the Shifter-side payload for GatewayService.Create.
// Region is a Shifter-only attribute (we persist it locally for the regional
// CSV/dropdown UX) — ChirpStack itself derives the LoRa region from the bound
// device-profile, not the gateway, so we do NOT push it on the wire.
//
// Lat / Lng / Altitude are operator-typed values; the wrapper stamps
// Location.Source = CONFIG so CS doesn't try to merge them with GPS
// uplinks from a packet forwarder that happens to send its own location.
type CreateGatewayInput struct {
	GatewayID   string // lowercase 16-char hex EUI64
	Name        string
	Description string
	Region      string // Shifter-only; not forwarded to CS proto
	Lat         float64
	Lng         float64
	Altitude    float64
	Tags        map[string]string
	TenantID    string // CS tenant UUID (single global tenant per D-28)
}

// Gateway is the Shifter-side domain shape returned by GetGateway / list-item
// mapping. State is normalised to the proto enum's string name
// ("NEVER_SEEN" | "ONLINE" | "OFFLINE") so handlers don't need to import the
// api package to compare. LastSeenAt is nil-able because a gateway that has
// never connected to the network server doesn't have one yet.
type Gateway struct {
	GatewayID   string
	Name        string
	Description string
	Lat         float64
	Lng         float64
	Altitude    float64
	Tags        map[string]string
	LastSeenAt  *time.Time
	State       string // "NEVER_SEEN" | "ONLINE" | "OFFLINE"
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListGatewaysInput models the paging + filter knobs of GatewayService.List.
// OrderBy is a canonical lowercase string ("name", "gateway_id", "last_seen")
// rather than the proto enum so callers don't import api.* — parseOrderBy
// translates internally.
type ListGatewaysInput struct {
	TenantID    string
	Search      string // substring match on name (CS-side ILIKE)
	Limit       uint32
	Offset      uint32
	OrderBy     string // "name" | "gateway_id" | "last_seen"
	OrderByDesc bool
}

// ListGatewaysOutput pairs the total-row count (for pagination UI) with the
// returned page of items.
type ListGatewaysOutput struct {
	TotalCount uint32
	Items      []Gateway
}

// GetGatewayMetricsInput is the wrapper for GatewayService.GetMetrics.
// Aggregation is the canonical proto enum name as a string
// ("HOUR" | "DAY" | "MONTH" | "MINUTE"); the cache layer (D-02 default)
// passes "HOUR" with a 24h window so the result fits a 24-bucket sparkline.
type GetGatewayMetricsInput struct {
	GatewayID   string
	Start       time.Time
	End         time.Time
	Aggregation string // "HOUR" | "DAY" | "MONTH" | "MINUTE"
}

// CreateGateway pushes a new gateway to ChirpStack with operator-typed
// coordinates (Location.Source = CONFIG). StatsInterval defaults to 30s, the
// canonical packet-forwarder default — operators rarely override it and the
// proto requires a value to avoid CS computing a NEVER_SEEN gateway from a
// silent-but-alive forwarder.
func (c *Client) CreateGateway(ctx context.Context, in CreateGatewayInput) error {
	if in.GatewayID == "" {
		return fmt.Errorf("CreateGateway: GatewayID is required")
	}
	svc := api.NewGatewayServiceClient(c.conn)
	_, err := svc.Create(ctx, &api.CreateGatewayRequest{
		Gateway: gatewayProtoFromInput(in),
	})
	return wrapCSErr(err)
}

// GetGateway returns a single gateway by EUI64. Translates codes.NotFound to
// ErrNotFound; other gRPC errors flow through wrapCSErr.
func (c *Client) GetGateway(ctx context.Context, gatewayID string) (*Gateway, error) {
	if gatewayID == "" {
		return nil, fmt.Errorf("GetGateway: gatewayID is required")
	}
	svc := api.NewGatewayServiceClient(c.conn)
	resp, err := svc.Get(ctx, &api.GetGatewayRequest{GatewayId: gatewayID})
	if err != nil {
		return nil, wrapCSErr(err)
	}
	return gatewayProtoToDomain(resp.GetGateway(), resp.GetCreatedAt(), resp.GetUpdatedAt(), resp.GetLastSeenAt(), ""), nil
}

// UpdateGateway issues GatewayService.Update with full-object replace
// semantics (mirrors Plan 02-08 UpdateDeviceProfile — CS v4 Update wipes
// fields not set in the request). Caller is expected to pass a complete
// CreateGatewayInput shape; missing fields will null/zero on the CS side.
func (c *Client) UpdateGateway(ctx context.Context, in CreateGatewayInput) error {
	if in.GatewayID == "" {
		return fmt.Errorf("UpdateGateway: GatewayID is required")
	}
	svc := api.NewGatewayServiceClient(c.conn)
	_, err := svc.Update(ctx, &api.UpdateGatewayRequest{
		Gateway: gatewayProtoFromInput(in),
	})
	return wrapCSErr(err)
}

// DeleteGateway issues GatewayService.Delete. A second delete on the same
// gateway_id surfaces ErrNotFound — callers that want idempotent semantics
// can `errors.Is(err, ErrNotFound)` and swallow.
func (c *Client) DeleteGateway(ctx context.Context, gatewayID string) error {
	if gatewayID == "" {
		return fmt.Errorf("DeleteGateway: gatewayID is required")
	}
	svc := api.NewGatewayServiceClient(c.conn)
	_, err := svc.Delete(ctx, &api.DeleteGatewayRequest{GatewayId: gatewayID})
	return wrapCSErr(err)
}

// ListGateways issues GatewayService.List with paging + tenant filter.
// Limit=0 is forwarded as-is (CS treats it as "total_count only"); most
// callers want a non-zero page.
func (c *Client) ListGateways(ctx context.Context, in ListGatewaysInput) (*ListGatewaysOutput, error) {
	svc := api.NewGatewayServiceClient(c.conn)
	resp, err := svc.List(ctx, &api.ListGatewaysRequest{
		TenantId:    in.TenantID,
		Search:      in.Search,
		Limit:       in.Limit,
		Offset:      in.Offset,
		OrderBy:     parseGatewayOrderBy(in.OrderBy),
		OrderByDesc: in.OrderByDesc,
	})
	if err != nil {
		return nil, wrapCSErr(err)
	}
	out := &ListGatewaysOutput{
		TotalCount: resp.GetTotalCount(),
		Items:      make([]Gateway, 0, len(resp.GetResult())),
	}
	for _, it := range resp.GetResult() {
		out.Items = append(out.Items, listItemToDomain(it))
	}
	return out, nil
}

// GetMetrics issues GatewayService.GetMetrics for the (gateway_id, [start,end],
// aggregation) tuple. Returns the raw *api.GetGatewayMetricsResponse so the
// cache layer (gateway_metrics_cache.go) can hold a single shared response
// across N concurrent list-page requests without re-deriving sparkline data
// per call.
//
// [VERIFIED rename] — the proto method is GetMetrics; CONTEXT D-02 still
// names the cache strategy after the obsolete pre-v4 stats RPC. Don't be
// surprised by the asymmetry — the cache's behaviour matches D-02 verbatim.
func (c *Client) GetMetrics(ctx context.Context, in GetGatewayMetricsInput) (*api.GetGatewayMetricsResponse, error) {
	if in.GatewayID == "" {
		return nil, fmt.Errorf("GetMetrics: GatewayID is required")
	}
	svc := api.NewGatewayServiceClient(c.conn)
	resp, err := svc.GetMetrics(ctx, &api.GetGatewayMetricsRequest{
		GatewayId:   in.GatewayID,
		Start:       timestamppb.New(in.Start),
		End:         timestamppb.New(in.End),
		Aggregation: parseAggregation(in.Aggregation),
	})
	if err != nil {
		return nil, wrapCSErr(err)
	}
	return resp, nil
}

// gatewayProtoFromInput is the Create/Update shared encoder. Location.Source
// is unconditionally CONFIG — Shifter never collects GPS-based coords from
// the gateway hardware (D-02 says operator types lat/lng during onboarding).
func gatewayProtoFromInput(in CreateGatewayInput) *api.Gateway {
	return &api.Gateway{
		GatewayId:   in.GatewayID,
		Name:        in.Name,
		Description: in.Description,
		Location: &common.Location{
			Latitude:  in.Lat,
			Longitude: in.Lng,
			Altitude:  in.Altitude,
			Source:    common.LocationSource_CONFIG, // operator-typed coords
			Accuracy:  0,
		},
		TenantId:      in.TenantID,
		Tags:          in.Tags,
		StatsInterval: 30, // packet-forwarder canonical default
	}
}

// gatewayProtoToDomain projects the Get response onto our domain Gateway.
// stateOverride is used by the list-item path (which has its own State enum);
// the Get path passes "" and we leave State empty since GetGatewayResponse
// has no state field in proto v4.17.
func gatewayProtoToDomain(gw *api.Gateway, createdAt, updatedAt, lastSeenAt *timestamppb.Timestamp, stateOverride string) *Gateway {
	if gw == nil {
		return nil
	}
	out := &Gateway{
		GatewayID:   gw.GetGatewayId(),
		Name:        gw.GetName(),
		Description: gw.GetDescription(),
		Tags:        gw.GetTags(),
		State:       stateOverride,
	}
	if loc := gw.GetLocation(); loc != nil {
		out.Lat = loc.GetLatitude()
		out.Lng = loc.GetLongitude()
		out.Altitude = loc.GetAltitude()
	}
	if createdAt != nil {
		out.CreatedAt = createdAt.AsTime()
	}
	if updatedAt != nil {
		out.UpdatedAt = updatedAt.AsTime()
	}
	if lastSeenAt != nil {
		t := lastSeenAt.AsTime()
		out.LastSeenAt = &t
	}
	return out
}

// listItemToDomain projects a GatewayListItem onto the domain Gateway.
// GatewayListItem differs from Gateway: it lacks Tags + StatsInterval but
// gains State + Properties + LastSeenAt — exactly the fields the GW-01 list
// column needs.
func listItemToDomain(it *api.GatewayListItem) Gateway {
	out := Gateway{
		GatewayID:   it.GetGatewayId(),
		Name:        it.GetName(),
		Description: it.GetDescription(),
		State:       gatewayStateString(it.GetState()),
	}
	if loc := it.GetLocation(); loc != nil {
		out.Lat = loc.GetLatitude()
		out.Lng = loc.GetLongitude()
		out.Altitude = loc.GetAltitude()
	}
	if it.GetCreatedAt() != nil {
		out.CreatedAt = it.GetCreatedAt().AsTime()
	}
	if it.GetUpdatedAt() != nil {
		out.UpdatedAt = it.GetUpdatedAt().AsTime()
	}
	if it.GetLastSeenAt() != nil {
		t := it.GetLastSeenAt().AsTime()
		out.LastSeenAt = &t
	}
	return out
}

// gatewayStateString maps the proto enum value to the canonical Shifter name.
// We don't expose the integer enum to callers — handlers compare against the
// string label.
func gatewayStateString(s api.GatewayState) string {
	switch s {
	case api.GatewayState_NEVER_SEEN:
		return "NEVER_SEEN"
	case api.GatewayState_ONLINE:
		return "ONLINE"
	case api.GatewayState_OFFLINE:
		return "OFFLINE"
	default:
		return "NEVER_SEEN"
	}
}

// parseGatewayOrderBy maps the Shifter string label ("name" / "gateway_id" /
// "last_seen") to the proto enum. Unknown / empty → NAME (proto's zero value,
// matches CS server-side default).
func parseGatewayOrderBy(s string) api.ListGatewaysRequest_OrderBy {
	switch strings.ToLower(s) {
	case "", "name":
		return api.ListGatewaysRequest_NAME
	case "gateway_id":
		return api.ListGatewaysRequest_GATEWAY_ID
	case "last_seen", "last_seen_at":
		return api.ListGatewaysRequest_LAST_SEEN_AT
	default:
		return api.ListGatewaysRequest_NAME
	}
}

// parseAggregation maps the Shifter string label ("HOUR" / "DAY" / "MONTH" /
// "MINUTE") to common.Aggregation. Unknown / empty → HOUR (matches the
// 24-bucket D-02 sparkline default).
func parseAggregation(s string) common.Aggregation {
	switch strings.ToUpper(s) {
	case "", "HOUR":
		return common.Aggregation_HOUR
	case "DAY":
		return common.Aggregation_DAY
	case "MONTH":
		return common.Aggregation_MONTH
	case "MINUTE":
		return common.Aggregation_MINUTE
	default:
		return common.Aggregation_HOUR
	}
}
