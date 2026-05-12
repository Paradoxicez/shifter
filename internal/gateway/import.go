package gateway

// ImportService implements bulk gateway import (Plan 07-13 / D-17).
//
// Lifecycle mirrors the Phase 3 device-import pattern but is intentionally
// simpler: gateways have no ChirpStack "device keys" or activation modes, and
// the PG schema already has a unique index on gateway_id, so a single
// UpsertGatewayForBulkImport call per row is sufficient.
//
// Two-phase approach (no import_job table — gateway import is synchronous):
//
//  1. Validate(ctx, csvBytes) — parses CSV, applies per-row checks, returns
//     ValidateResult. No DB writes.
//  2. Commit(ctx, csvBytes, actorID) — re-parses, upserts each valid row by
//     EUI (idempotent), writes one audit row per gateway in the same Postgres
//     transaction, returns CommitResult.
//
// Idempotency outcomes:
//
//   - "created"  — gateway_id did not exist; UpsertGatewayForBulkImport inserted.
//   - "updated"  — gateway_id existed but name/description/lat/lng/region changed.
//   - "skipped"  — gateway_id existed and ALL fields match the input (no-op update).
//   - "error"    — parse/validation failure; no DB write.

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/install"
)

// MaxImportRows is the row cap (T-07-13-03). Uploads exceeding this are
// rejected before any DB interaction.
const MaxImportRows = 5000

// gatewayEUIRegexp validates a 16-hex-char lowercase EUI64.
var gatewayEUIRegexp = regexp.MustCompile(`^[0-9a-f]{16}$`)

// knownRegionNames is the set of valid LoRaWAN region slugs from
// install.Regions(). Lazily populated on first use.
var knownRegionSet map[string]struct{}

func init() {
	knownRegionSet = make(map[string]struct{}, 16)
	for _, r := range install.Regions() {
		knownRegionSet[r.Name] = struct{}{}
	}
}

// RowOutcome holds the per-row result of a Validate or Commit call.
type RowOutcome struct {
	RowNumber    int
	GatewayEUI   string
	Outcome      string // "created" | "updated" | "skipped" | "error"
	ErrorMessage string
}

// ValidateResult is returned by ImportService.Validate.
type ValidateResult struct {
	ValidRows int
	ErrorRows int
	Errors    []RowOutcome
}

// CommitResult is returned by ImportService.Commit.
type CommitResult struct {
	Created  int
	Updated  int
	Skipped  int
	Outcomes []RowOutcome
}

// parsedGatewayRow holds the canonicalised values for a single validated row.
type parsedGatewayRow struct {
	rowNumber   int
	gatewayEUI  string
	name        string
	description *string
	lat         *float64
	lng         *float64
	region      string
}

// ImportService provides Validate + Commit for bulk gateway CSV imports.
type ImportService struct {
	pool *pgxpool.Pool
}

// NewImportService returns a new ImportService backed by the given pool.
func NewImportService(pool *pgxpool.Pool) *ImportService {
	return &ImportService{pool: pool}
}

// Validate parses csvBytes, applies per-row validation, and returns a
// ValidateResult. No database writes are performed.
func (s *ImportService) Validate(ctx context.Context, csvBytes []byte) (ValidateResult, error) {
	rows, errs, err := parseGatewayCSV(csvBytes)
	if err != nil {
		return ValidateResult{}, fmt.Errorf("gateway import validate: %w", err)
	}

	result := ValidateResult{
		ValidRows: len(rows),
		ErrorRows: len(errs),
		Errors:    errs,
	}
	return result, nil
}

// Commit re-parses csvBytes, upserts each valid row, and writes one audit row
// per gateway in a per-row Postgres transaction. actorID must be a valid user
// UUID string (from the HTTP session).
func (s *ImportService) Commit(ctx context.Context, csvBytes []byte, actorID string) (CommitResult, error) {
	rows, _, err := parseGatewayCSV(csvBytes)
	if err != nil {
		return CommitResult{}, fmt.Errorf("gateway import commit: parse: %w", err)
	}

	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return CommitResult{}, fmt.Errorf("gateway import commit: invalid actorID %q: %w", actorID, err)
	}

	var result CommitResult
	q := sqlc.New(s.pool)
	_ = q // used inside commitRow

	for _, row := range rows {
		outcome, err := s.commitRow(ctx, actorUUID, row)
		if err != nil {
			// Per-row errors surface as "error" outcomes; don't abort the batch.
			result.Outcomes = append(result.Outcomes, RowOutcome{
				RowNumber:    row.rowNumber,
				GatewayEUI:   row.gatewayEUI,
				Outcome:      "error",
				ErrorMessage: err.Error(),
			})
			continue
		}
		result.Outcomes = append(result.Outcomes, RowOutcome{
			RowNumber:  row.rowNumber,
			GatewayEUI: row.gatewayEUI,
			Outcome:    outcome,
		})
		switch outcome {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		case "skipped":
			result.Skipped++
		}
	}
	return result, nil
}

// commitRow upserts a single gateway row and writes an audit row, both in a
// single Serializable transaction.
func (s *ImportService) commitRow(ctx context.Context, actorID uuid.UUID, row parsedGatewayRow) (string, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txQ := sqlc.New(tx)

	// Pre-fetch existing gateway to detect skipped vs updated after the upsert.
	// This lookup happens inside the same Serializable tx so we get a consistent
	// snapshot; the upsert below is the only writer for this gateway_id.
	existing, existsErr := txQ.GetGatewayByGatewayID(ctx, row.gatewayEUI)

	upserted, err := txQ.UpsertGatewayForBulkImport(ctx, sqlc.UpsertGatewayForBulkImportParams{
		GatewayID:   row.gatewayEUI,
		Name:        row.name,
		Description: row.description,
		Region:      row.region,
		Lat:         row.lat,
		Lng:         row.lng,
		CsTenantID:  nil, // bulk import doesn't bootstrap CS; operator provisions CS separately
	})
	if err != nil {
		return "", fmt.Errorf("upsert gateway %q: %w", row.gatewayEUI, err)
	}

	// Determine outcome:
	//   - WasInserted=true → "created"
	//   - WasInserted=false + pre-existing fields match input → "skipped"
	//   - WasInserted=false + fields changed → "updated"
	var outcome string
	if upserted.WasInserted {
		outcome = "created"
	} else if existsErr == nil && isExistingGatewayUnchanged(existing, row) {
		outcome = "skipped"
	} else {
		outcome = "updated"
	}

	// Write per-row audit entry (T-07-13-05 mitigation).
	after := map[string]any{
		"gateway_id":  row.gatewayEUI,
		"name":        row.name,
		"region":      row.region,
		"outcome":     outcome,
	}
	if row.description != nil {
		after["description"] = *row.description
	}
	if row.lat != nil {
		after["lat"] = *row.lat
	}
	if row.lng != nil {
		after["lng"] = *row.lng
	}

	gwID := uuid.UUID(upserted.ID.Bytes)
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     actorID,
		Action:     audit.AuditActionGatewayBulkImported,
		EntityType: audit.EntityTypeGateway,
		EntityID:   gwID,
		Before:     nil,
		After:      after,
		Notes:      "bulk_import",
	}); err != nil {
		return "", fmt.Errorf("audit write for gateway %q: %w", row.gatewayEUI, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("tx commit for gateway %q: %w", row.gatewayEUI, err)
	}
	return outcome, nil
}

// isExistingGatewayUnchanged reports whether the pre-existing DB row matches
// the CSV input exactly (name, description, lat, lng, region). Called when
// WasInserted=false to distinguish "skipped" (no effective change) from
// "updated" (at least one field changed).
func isExistingGatewayUnchanged(existing sqlc.Gateway, row parsedGatewayRow) bool {
	if existing.Name != row.name {
		return false
	}
	if existing.Region != row.region {
		return false
	}
	// description: both nil, or both non-nil with equal value.
	if (existing.Description == nil) != (row.description == nil) {
		return false
	}
	if existing.Description != nil && row.description != nil && *existing.Description != *row.description {
		return false
	}
	// lat/lng: compare with epsilon to handle float round-trip noise.
	if !floatPtrEqual(existing.Lat, row.lat) {
		return false
	}
	if !floatPtrEqual(existing.Lng, row.lng) {
		return false
	}
	return true
}

// floatPtrEqual compares two *float64 values using an epsilon to handle
// Postgres double precision round-trip noise.
func floatPtrEqual(a, b *float64) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	return math.Abs(*a-*b) < 1e-9
}

// parseGatewayCSV parses a gateway CSV (header + data rows) and returns
// valid rows and error outcomes. The parser reuses the UTF-8 BOM guard pattern
// from internal/import/parser_csv.go.
//
// Expected headers: gateway_eui, name, description, latitude, longitude, region
// (case-insensitive; extra columns are ignored).
func parseGatewayCSV(data []byte) ([]parsedGatewayRow, []RowOutcome, error) {
	// Strip UTF-8 BOM.
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	all, err := r.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("csv parse: %w", err)
	}
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("csv is empty")
	}

	// Build header index map (lowercase).
	headerIdx := make(map[string]int, len(all[0]))
	for i, h := range all[0] {
		h = strings.TrimSpace(strings.ToLower(h))
		headerIdx[h] = i
	}

	col := func(row []string, name string) string {
		idx, ok := headerIdx[name]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	if len(all) > MaxImportRows+1 {
		return nil, nil, fmt.Errorf("too many rows: max %d, got %d", MaxImportRows, len(all)-1)
	}

	var valid []parsedGatewayRow
	var errors []RowOutcome

	for i, row := range all[1:] {
		rowNum := i + 2 // 1-indexed, header is row 1

		// 1. Gateway EUI: required, 16 hex chars lowercase.
		rawEUI := col(row, "gateway_eui")
		if rawEUI == "" {
			errors = append(errors, RowOutcome{
				RowNumber:    rowNum,
				GatewayEUI:   rawEUI,
				Outcome:      "error",
				ErrorMessage: "missing required field: gateway_eui",
			})
			continue
		}
		eui := strings.ToLower(strings.ReplaceAll(rawEUI, ":", ""))
		if !gatewayEUIRegexp.MatchString(eui) {
			errors = append(errors, RowOutcome{
				RowNumber:    rowNum,
				GatewayEUI:   rawEUI,
				Outcome:      "error",
				ErrorMessage: fmt.Sprintf("invalid gateway_eui %q: must be 16 lowercase hex characters", rawEUI),
			})
			continue
		}

		// 2. Name: required, 1-80 chars.
		name := col(row, "name")
		if name == "" {
			errors = append(errors, RowOutcome{
				RowNumber:    rowNum,
				GatewayEUI:   eui,
				Outcome:      "error",
				ErrorMessage: "missing required field: name",
			})
			continue
		}
		if len(name) > 80 {
			errors = append(errors, RowOutcome{
				RowNumber:    rowNum,
				GatewayEUI:   eui,
				Outcome:      "error",
				ErrorMessage: fmt.Sprintf("name too long: %d chars (max 80)", len(name)),
			})
			continue
		}

		// 3. Description: optional.
		var descPtr *string
		if d := col(row, "description"); d != "" {
			descPtr = &d
		}

		// 4. Latitude + Longitude: both optional, but if one is given both must be.
		latStr := col(row, "latitude")
		lngStr := col(row, "longitude")
		var latPtr, lngPtr *float64
		if latStr != "" || lngStr != "" {
			if latStr == "" || lngStr == "" {
				errors = append(errors, RowOutcome{
					RowNumber:    rowNum,
					GatewayEUI:   eui,
					Outcome:      "error",
					ErrorMessage: "latitude and longitude must both be provided or both omitted",
				})
				continue
			}
			lat, err := strconv.ParseFloat(latStr, 64)
			if err != nil || lat < -90 || lat > 90 {
				errors = append(errors, RowOutcome{
					RowNumber:    rowNum,
					GatewayEUI:   eui,
					Outcome:      "error",
					ErrorMessage: fmt.Sprintf("invalid latitude %q: must be a decimal between -90 and 90", latStr),
				})
				continue
			}
			lng, err := strconv.ParseFloat(lngStr, 64)
			if err != nil || lng < -180 || lng > 180 {
				errors = append(errors, RowOutcome{
					RowNumber:    rowNum,
					GatewayEUI:   eui,
					Outcome:      "error",
					ErrorMessage: fmt.Sprintf("invalid longitude %q: must be a decimal between -180 and 180", lngStr),
				})
				continue
			}
			latPtr = &lat
			lngPtr = &lng
		}

		// 5. Region: must match known LoRaWAN region slug.
		region := strings.ToLower(col(row, "region"))
		if region == "" {
			errors = append(errors, RowOutcome{
				RowNumber:    rowNum,
				GatewayEUI:   eui,
				Outcome:      "error",
				ErrorMessage: "missing required field: region",
			})
			continue
		}
		if _, ok := knownRegionSet[region]; !ok {
			errors = append(errors, RowOutcome{
				RowNumber:    rowNum,
				GatewayEUI:   eui,
				Outcome:      "error",
				ErrorMessage: fmt.Sprintf("unknown region %q: must be one of the known LoRaWAN region slugs", region),
			})
			continue
		}

		valid = append(valid, parsedGatewayRow{
			rowNumber:   rowNum,
			gatewayEUI:  eui,
			name:        name,
			description: descPtr,
			lat:         latPtr,
			lng:         lngPtr,
			region:      region,
		})
	}

	return valid, errors, nil
}
