package meteringpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// bytesToHex renders raw bytes as lowercase space-separated hex pairs.
// D-18: format is "0a 1f 3c 4d" (lowercase, space-separated).
func bytesToHex(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = fmt.Sprintf("%02x", x)
	}
	return strings.Join(parts, " ")
}

// DetailResponse is the wire shape for GET /api/metering-points/{id} (Plan 05
// DETL-01). The Advanced tab consumes decoded_object + extra from
// latest_reading. Frontend uses latest_reading==null to disable Advanced and
// Uplinks tabs (D-22 empty case).
type DetailResponse struct {
	MeteringPoint  detailMP             `json:"metering_point"`
	ActiveBinding  *detailBinding       `json:"active_binding"`
	LatestReading  *detailReading       `json:"latest_reading"`
	QualitySummary detailQualitySummary `json:"quality_summary"`
	Online         *bool                `json:"online"`
}

type detailMP struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	SiteID              string `json:"site_id"`
	SiteName            string `json:"site_name"`
	UtilityClass        string `json:"utility_class"`
	LocationDescription any    `json:"location_description"`
}

type detailBinding struct {
	DeviceID          string `json:"device_id"`
	DevEUI            string `json:"dev_eui"`
	DeviceProfileName string `json:"device_profile_name"`
	ValidFrom         string `json:"valid_from"`
}

type detailReading struct {
	Time            string          `json:"time"`
	CumulativeValue any             `json:"cumulative_value"`
	InstantValue    any             `json:"instant_value"`
	Quality         string          `json:"quality"`
	BatteryPct      any             `json:"battery_pct"`
	Rssi            any             `json:"rssi"`
	Snr             any             `json:"snr"`
	Fcnt            any             `json:"fcnt"`
	DecodedObject   json.RawMessage `json:"decoded_object"`
	Extra           json.RawMessage `json:"extra"`
	RawPayloadHex   string          `json:"raw_payload_hex"`
}

type detailQualitySummary struct {
	WindowSize    int            `json:"window_size"`
	FlaggedCount  int            `json:"flagged_count"`
	ByQuality     map[string]int `json:"by_quality"`
}

// handleDetail serves GET /api/metering-points/{id}.
//
// T-04-05-01: `:id` is uuid-parsed at entry; 400 on parse error.
// D-22: MP with no binding / no measurements returns 200 with null sub-objects.
// D-07: online flag derived from MeteringPointOnlineStatus.
func (deps Deps) handleDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	ctx := r.Context()
	q := sqlc.New(deps.Pool)

	// 1. Composite MP + binding + latest reading row.
	row, err := q.GetMeteringPointDetail(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
		return
	}
	if err != nil {
		internalError(deps.Log, w, "get MP detail", err)
		return
	}

	// 2. Online status (D-07). Two non-null booleans: device_bound + is_online.
	onlineRow, err := q.MeteringPointOnlineStatus(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		internalError(deps.Log, w, "get online status", err)
		return
	}

	// 3. Quality summary (D-19).
	qualityRows, err := q.QualitySummaryByMP(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		internalError(deps.Log, w, "get quality summary", err)
		return
	}

	// --- Build response ---

	// Metering point sub-object.
	siteName := ""
	if row.SiteName != nil {
		siteName = *row.SiteName
	}
	mp := detailMP{
		ID:                  uuidString(row.ID),
		Name:                row.Name,
		SiteID:              uuidString(row.SiteID),
		SiteName:            siteName,
		UtilityClass:        row.UtilityClass,
		LocationDescription: derefString(row.LocationDescription),
	}

	// Active binding — null when LEFT JOIN produced no row (valid_to IS NULL AND no binding).
	var activebinding *detailBinding
	if row.BindingID.Valid {
		devEUI := ""
		if row.DevEui != nil {
			devEUI = *row.DevEui
		}
		dpName := ""
		if row.DeviceProfileName != nil {
			dpName = *row.DeviceProfileName
		}
		vf := ""
		if row.ValidFrom.Valid {
			vf = row.ValidFrom.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		activebinding = &detailBinding{
			DeviceID:          uuidString(row.DeviceID),
			DevEUI:            devEUI,
			DeviceProfileName: dpName,
			ValidFrom:         vf,
		}
	}

	// Latest reading — null when MP has no measurement rows yet (D-22).
	var latestReading *detailReading
	if row.LatestTime.Valid {
		snr := any(nil)
		if row.Snr != nil {
			snr = *row.Snr
		}
		fcnt := any(nil)
		if row.Fcnt != nil {
			fcnt = *row.Fcnt
		}
		// Ensure decoded_object and extra are never bare nulls in JSON.
		decodedObj := json.RawMessage(row.DecodedObject)
		if len(decodedObj) == 0 {
			decodedObj = json.RawMessage(`{}`)
		}
		extraJSON := json.RawMessage(row.Extra)
		if len(extraJSON) == 0 {
			extraJSON = json.RawMessage(`{}`)
		}
		latestReading = &detailReading{
			Time:            row.LatestTime.Time.UTC().Format("2006-01-02T15:04:05Z"),
			CumulativeValue: numericText(row.CumulativeValue),
			InstantValue:    numericText(row.InstantValue),
			Quality:         row.Quality,
			BatteryPct:      derefInt16(row.BatteryPct),
			Rssi:            derefInt16(row.Rssi),
			Snr:             snr,
			Fcnt:            fcnt,
			DecodedObject:   decodedObj,
			Extra:           extraJSON,
			RawPayloadHex:   bytesToHex(row.RawPayload),
		}
	}

	// Quality summary (D-19 window of last 100).
	byQuality := make(map[string]int)
	windowSize := 0
	flaggedCount := 0
	for _, qr := range qualityRows {
		count := int(qr.Count)
		byQuality[qr.Quality] = count
		windowSize += count
		if qr.Quality != "ok" {
			flaggedCount += count
		}
	}
	qualitySummary := detailQualitySummary{
		WindowSize:   windowSize,
		FlaggedCount: flaggedCount,
		ByQuality:    byQuality,
	}

	// Online flag: null=no binding, true/false=D-07 rule.
	var onlinePtr *bool
	deviceBound, _ := onlineRow.DeviceBound.(bool)
	if deviceBound {
		isOnline, _ := onlineRow.IsOnline.(bool)
		onlinePtr = &isOnline
	}

	resp := DetailResponse{
		MeteringPoint:  mp,
		ActiveBinding:  activebinding,
		LatestReading:  latestReading,
		QualitySummary: qualitySummary,
		Online:         onlinePtr,
	}
	writeJSON(w, http.StatusOK, resp)
}
