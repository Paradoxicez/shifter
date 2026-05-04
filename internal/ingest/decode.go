package ingest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Event is Shifter's narrow view of the ChirpStack v4 uplink event JSON.
//
// We parse only the fields the ingest pipeline needs (DevEUI for resolver
// lookup, FCnt/FPort for diagnostics, RxInfo[].Time + top-level Time for
// the gateway/device timestamp diagnostics, DecodedObject for the codec
// output the mappings walk). Everything else (txInfo, deduplicationId,
// confirmedUplink, regionConfigId, etc.) goes through unread but the
// raw payload is preserved on the measurement row (DATA-07) so per-meter
// detail (Phase 4) can show the firehose.
//
// Decoded as a map[string]any specifically — Pitfall 11: ChirpStack v4's
// uplink event encodes the codec output as a JSON object whose shape is
// vendor-specific. v3's `objectJSON string` is gone. Treating object as
// a map keeps the normalize pass's profile.Resolve walk uniform across
// vendors.
type Event struct {
	DevEUI          string         // lowercase 16-char hex — DATA-01 normalization
	ApplicationID   string         // CS application UUID; informational
	FCnt            uint32         // LoRaWAN frame counter
	FPort           uint8          // LoRaWAN frame port (codec dispatch hint)
	DeduplicationID string         // CS deduplication UUID; informational
	Data            []byte         // base64-decoded LoRaWAN frame payload (raw bytes)
	DecodedObject   map[string]any // QuickJS codec output — arbitrary shape (may be nil)
	RxInfo          []RxInfo       // every gateway that heard this uplink
	DeviceTime      *time.Time     // optional device-side timestamp (top-level `time`)
	GatewayRxTime   *time.Time     // earliest rxInfo[].time across all gateways
}

// RxInfo is the per-gateway diagnostic envelope ChirpStack delivers on
// every uplink. We retain RSSI/SNR for Phase 4 link-quality reporting and
// Time so we can pick the earliest gateway timestamp as GatewayRxTime.
type RxInfo struct {
	GatewayID string
	RSSI      int16
	SNR       float32
	Time      *time.Time
}

// DecodeChirpStackEvent parses the JSON payload of a ChirpStack v4 MQTT
// uplink event into a strongly-typed Event.
//
// Errors only on JSON parse failure — missing/optional fields land as zero
// values and the pipeline's quality flag handles the consequences downstream
// (D-26 — never silent-drop). A missing `object` field (codec absent or
// codec runtime failure) yields ev.DecodedObject == nil with no error so
// the handler proceeds to write a quality='missing_canonical' row.
//
// Defensive normalization:
//   - DevEUI is lowercased (the schema enforces lowercase via 0012 CHECK;
//     resolver also lowercases on lookup — defense in depth).
//   - Base64 decode failure on the `data` field silently returns nil bytes
//     rather than failing the whole decode — the raw_payload column gets
//     the original JSON bytes regardless, so no information is lost.
//   - rxInfo[].time parses with parseRFC3339Maybe (RFC 3339 nanos accepted;
//     unparseable strings → nil). The minimum is captured as GatewayRxTime.
func DecodeChirpStackEvent(payload []byte) (Event, error) {
	var raw struct {
		DeviceInfo struct {
			DevEui        string `json:"devEui"`
			ApplicationID string `json:"applicationId"`
		} `json:"deviceInfo"`
		FCnt            uint32         `json:"fCnt"`
		FPort           uint8          `json:"fPort"`
		DeduplicationID string         `json:"deduplicationId"`
		Data            string         `json:"data"` // base64
		Object          map[string]any `json:"object"`
		RxInfo          []struct {
			GatewayID string  `json:"gatewayId"`
			RSSI      int16   `json:"rssi"`
			SNR       float32 `json:"snr"`
			Time      string  `json:"time"`
		} `json:"rxInfo"`
		Time string `json:"time"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return Event{}, fmt.Errorf("ingest decode: json: %w", err)
	}

	bytes, _ := base64.StdEncoding.DecodeString(raw.Data) // empty on err — handled later

	ev := Event{
		DevEUI:          strings.ToLower(raw.DeviceInfo.DevEui),
		ApplicationID:   raw.DeviceInfo.ApplicationID,
		FCnt:            raw.FCnt,
		FPort:           raw.FPort,
		DeduplicationID: raw.DeduplicationID,
		Data:            bytes,
		DecodedObject:   raw.Object,
	}
	for _, r := range raw.RxInfo {
		t := parseRFC3339Maybe(r.Time)
		ev.RxInfo = append(ev.RxInfo, RxInfo{
			GatewayID: r.GatewayID,
			RSSI:      r.RSSI,
			SNR:       r.SNR,
			Time:      t,
		})
		if t != nil && (ev.GatewayRxTime == nil || t.Before(*ev.GatewayRxTime)) {
			ev.GatewayRxTime = t
		}
	}
	if t := parseRFC3339Maybe(raw.Time); t != nil {
		ev.DeviceTime = t
	}
	return ev, nil
}

// parseRFC3339Maybe attempts RFC 3339 (with nanos). Empty / unparseable
// strings yield nil — callers treat the diagnostic timestamp as absent.
func parseRFC3339Maybe(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return nil
		}
	}
	return &t
}

// devEUIFromTopic extracts the dev_eui from a ChirpStack v4 uplink topic
// of the canonical shape `application/<app-id>/device/<dev-eui>/event/up`.
// Returns "" if the topic doesn't match — caller writes a row without a
// dev_eui anchor in that case (extremely unlikely in production; defense
// in depth for malformed broker traffic).
func devEUIFromTopic(topic string) string {
	parts := strings.Split(topic, "/")
	// application / <id> / device / <eui> / event / up
	if len(parts) < 6 {
		return ""
	}
	if parts[0] != "application" || parts[2] != "device" {
		return ""
	}
	return strings.ToLower(parts[3])
}
