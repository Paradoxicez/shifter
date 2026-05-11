package importpkg

// Dry-run validator. D-06 / D-08 / D-09: every uploaded row gets a per-row
// outcome before any ChirpStack / Postgres mutation. Operator reviews the
// preview, then explicitly commits.
//
// Outcome taxonomy (mirrors sqlc.ImportJobRowStatus):
//
//   - valid           — row passes every check; commit phase will attempt
//   - invalid         — bad format / missing required / unknown FK
//   - already_exists  — dev_eui already provisioned in PG (idempotency, D-06)
//   - created         — set by commit phase (NEVER by dry-run)
//   - failed          — set by commit phase on CS / PG error (NEVER by dry-run)
//
// Pre-lookups: site (by UUID or name) and device_profile (by slug) lists
// are loaded ONCE per dry-run, not per-row, so a 5000-row upload still does
// only two SELECTs to populate the lookup maps.

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// OutcomeStatus mirrors sqlc.ImportJobRowStatus enum strings. Defined here
// as typed Go constants so dryrun.go / commit.go / handlers.go can compare
// outcomes without importing the sqlc enum type at every site.
type OutcomeStatus string

const (
	StatusValid         OutcomeStatus = "valid"
	StatusInvalid       OutcomeStatus = "invalid"
	StatusAlreadyExists OutcomeStatus = "already_exists"
	StatusCreated       OutcomeStatus = "created"
	StatusFailed        OutcomeStatus = "failed"
)

// ToSQLC converts the typed OutcomeStatus to the sqlc enum variant.
func (s OutcomeStatus) ToSQLC() sqlc.ImportJobRowStatus {
	return sqlc.ImportJobRowStatus(string(s))
}

// RowOutcome is the per-row result of a dry-run pass. Reason is empty for
// valid rows; populated with a short machine-readable token for invalid /
// already_exists. Parsed carries the canonicalised values (lowercased EUIs,
// resolved site UUID, resolved device-profile UUID) so the commit pass
// doesn't have to re-do the lookups.
type RowOutcome struct {
	RowIndex int
	Status   OutcomeStatus
	Reason   string

	// Parsed — canonicalised values ready for the commit pass. Keys mirror
	// the input column names; values are typed appropriately:
	//   dev_eui          → string (lowercase 16-hex)
	//   join_eui         → string (lowercase 16-hex)
	//   app_key          → string (lowercase 32-hex)
	//   dev_addr         → string (lowercase 8-hex)
	//   nwk_s_key        → string (lowercase 32-hex)
	//   app_s_key        → string (lowercase 32-hex)
	//   activation_mode  → "OTAA" | "ABP"
	//   name             → string (trimmed)
	//   description      → string (trimmed)
	//   site_id          → string (UUID)
	//   device_profile_id→ string (UUID)
	//   f_cnt_up         → string (numeric text)
	//   f_cnt_down       → string (numeric text)
	Parsed map[string]any
}

// DryRunDeps bundles the read-only handles a dry-run pass needs.
type DryRunDeps struct {
	Queries *sqlc.Queries
}

// NewDryRunDeps returns a DryRunDeps wrapping the given Queries.
func NewDryRunDeps(q *sqlc.Queries) *DryRunDeps {
	return &DryRunDeps{Queries: q}
}

// Validate runs the dry-run pass over parsed rows. No CS or PG WRITEs are
// performed — only the two pre-lookups (sites + profiles + existing
// devices). The output slice is row_index-ordered.
//
// The intra-file duplicate detector uses a hash-set on the normalised
// dev_eui so the SECOND occurrence is the one flagged (the first remains
// valid). Pre-existing dev_eui in PG short-circuits to already_exists per
// D-06 — NOT invalid.
func (d *DryRunDeps) Validate(ctx context.Context, rows []ParsedRow) ([]RowOutcome, error) {
	// Pre-load site + device-profile lookup tables (small lists; one query
	// each).
	sites, err := d.Queries.ListActiveSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("dryrun: list sites: %w", err)
	}
	siteByID := make(map[string]uuid.UUID, len(sites))
	siteByName := make(map[string]uuid.UUID, len(sites))
	for _, s := range sites {
		id := uuid.UUID(s.ID.Bytes)
		siteByID[id.String()] = id
		siteByName[strings.ToLower(strings.TrimSpace(s.Name))] = id
	}

	profiles, err := d.Queries.ListActiveDeviceProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("dryrun: list profiles: %w", err)
	}
	profileBySlug := make(map[string]uuid.UUID, len(profiles))
	for _, p := range profiles {
		profileBySlug[strings.ToLower(p.Slug)] = uuid.UUID(p.ID.Bytes)
	}

	out := make([]RowOutcome, 0, len(rows))
	// seenDevEUI maps canonical dev_eui → first row_index. Intra-file
	// duplicate detection: any subsequent occurrence is invalid with a
	// "duplicate_in_file" reason that points back to the first row.
	seenDevEUI := make(map[string]int, len(rows))

	for _, row := range rows {
		oc := d.validateRow(ctx, row, siteByID, siteByName, profileBySlug, seenDevEUI)
		out = append(out, oc)
	}
	return out, nil
}

func (d *DryRunDeps) validateRow(
	ctx context.Context,
	row ParsedRow,
	siteByID, siteByName map[string]uuid.UUID,
	profileBySlug map[string]uuid.UUID,
	seenDevEUI map[string]int,
) RowOutcome {
	parsed := map[string]any{}

	// 1. dev_eui — required + normalisable.
	rawEUI := row.Raw["dev_eui"]
	if strings.TrimSpace(rawEUI) == "" {
		return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:dev_eui"}
	}
	devEUI, err := NormalizeDevEUI(rawEUI)
	if err != nil {
		return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "invalid_dev_eui"}
	}
	parsed["dev_eui"] = devEUI

	// 2. Intra-file duplicate — the SECOND occurrence is invalid.
	if firstRow, ok := seenDevEUI[devEUI]; ok {
		return RowOutcome{
			RowIndex: row.RowIndex,
			Status:   StatusInvalid,
			Reason:   fmt.Sprintf("duplicate_in_file:row_%d", firstRow),
		}
	}
	// We only stamp seen for non-duplicates; duplicates don't poison the
	// table further.
	seenDevEUI[devEUI] = row.RowIndex

	// 3. Pre-existing in PG → already_exists (NOT invalid, per D-06).
	if _, err := d.Queries.GetDeviceByDevEUI(ctx, devEUI); err == nil {
		return RowOutcome{
			RowIndex: row.RowIndex,
			Status:   StatusAlreadyExists,
			Reason:   "device already provisioned (idempotent skip)",
			Parsed:   parsed,
		}
	} else if !isNoRows(err) {
		// Wrap unexpected DB errors as invalid so the operator sees them in
		// the per-row preview rather than aborting the whole upload.
		return RowOutcome{
			RowIndex: row.RowIndex,
			Status:   StatusInvalid,
			Reason:   "lookup_failed:dev_eui",
		}
	}

	// 4. Required text fields.
	name := strings.TrimSpace(row.Raw["name"])
	if name == "" {
		return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:name"}
	}
	parsed["name"] = name
	if desc := strings.TrimSpace(row.Raw["description"]); desc != "" {
		parsed["description"] = desc
	}

	// 5. Device profile lookup.
	profSlug := strings.ToLower(strings.TrimSpace(row.Raw["device_profile"]))
	if profSlug == "" {
		return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:device_profile"}
	}
	profID, ok := profileBySlug[profSlug]
	if !ok {
		return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_device_profile"}
	}
	parsed["device_profile_id"] = profID.String()

	// 6. Site lookup — UUID or name.
	rawSite := strings.TrimSpace(row.Raw["site_id"])
	if rawSite == "" {
		return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:site_id"}
	}
	var siteID uuid.UUID
	if parsed, err := uuid.Parse(rawSite); err == nil {
		if id, ok := siteByID[parsed.String()]; ok {
			siteID = id
		}
	}
	if siteID == uuid.Nil {
		if id, ok := siteByName[strings.ToLower(rawSite)]; ok {
			siteID = id
		}
	}
	if siteID == uuid.Nil {
		return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_site"}
	}
	parsed["site_id"] = siteID.String()

	// 7. Activation mode — explicit or inferred.
	mode := strings.ToUpper(strings.TrimSpace(row.Raw["activation_mode"]))
	if mode == "" {
		// Infer: if app_key present → OTAA; if nwk_s_key + app_s_key present
		// → ABP; otherwise leave empty (caller will reject).
		appKey := strings.TrimSpace(row.Raw["app_key"])
		nwk := strings.TrimSpace(row.Raw["nwk_s_key"])
		apps := strings.TrimSpace(row.Raw["app_s_key"])
		switch {
		case appKey != "":
			mode = "OTAA"
		case nwk != "" && apps != "":
			mode = "ABP"
		default:
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:activation_mode"}
		}
	}
	if mode != "OTAA" && mode != "ABP" {
		return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "invalid_activation_mode"}
	}
	parsed["activation_mode"] = mode

	switch mode {
	case "OTAA":
		// join_eui optional (defaults to 0000000000000000); app_key required.
		joinEUIRaw := strings.TrimSpace(row.Raw["join_eui"])
		joinEUI := "0000000000000000"
		if joinEUIRaw != "" {
			normalised, err := NormalizeJoinEUI(joinEUIRaw)
			if err != nil {
				return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "invalid_join_eui"}
			}
			joinEUI = normalised
		}
		parsed["join_eui"] = joinEUI

		appKeyRaw := strings.TrimSpace(row.Raw["app_key"])
		if appKeyRaw == "" {
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:app_key"}
		}
		appKey, err := NormalizeAppKey(appKeyRaw)
		if err != nil {
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "invalid_app_key"}
		}
		parsed["app_key"] = appKey

	case "ABP":
		devAddrRaw := strings.TrimSpace(row.Raw["dev_addr"])
		if devAddrRaw == "" {
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:dev_addr"}
		}
		devAddr, err := NormalizeDevAddr(devAddrRaw)
		if err != nil {
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "invalid_dev_addr"}
		}
		parsed["dev_addr"] = devAddr

		nwkRaw := strings.TrimSpace(row.Raw["nwk_s_key"])
		if nwkRaw == "" {
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:nwk_s_key"}
		}
		nwkSKey, err := NormalizeNwkSKey(nwkRaw)
		if err != nil {
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "invalid_nwk_s_key"}
		}
		parsed["nwk_s_key"] = nwkSKey

		appsRaw := strings.TrimSpace(row.Raw["app_s_key"])
		if appsRaw == "" {
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "missing_required:app_s_key"}
		}
		appSKey, err := NormalizeAppSKey(appsRaw)
		if err != nil {
			return RowOutcome{RowIndex: row.RowIndex, Status: StatusInvalid, Reason: "invalid_app_s_key"}
		}
		parsed["app_s_key"] = appSKey

		// Optional FCnt fields — store as-is (text) so commit-phase can
		// uint32-parse with proper error messaging.
		if v := strings.TrimSpace(row.Raw["f_cnt_up"]); v != "" {
			parsed["f_cnt_up"] = v
		}
		if v := strings.TrimSpace(row.Raw["f_cnt_down"]); v != "" {
			parsed["f_cnt_down"] = v
		}
	}

	return RowOutcome{RowIndex: row.RowIndex, Status: StatusValid, Parsed: parsed}
}

// isNoRows is the pgx-no-rows recogniser. Centralised here so the rest of
// the package doesn't import pgx directly.
func isNoRows(err error) bool {
	if err == nil {
		return false
	}
	// pgx.ErrNoRows resolves to "no rows in result set" — substring match is
	// safer than importing pgx in dryrun.go (which keeps the unit-test
	// surface narrower).
	return strings.Contains(err.Error(), "no rows in result set")
}
