package install

// Region is a LoRaWAN regional plan entry presented in the install wizard's
// step 3 picker. The catalog is intentionally hardcoded for Phase 1 (RESEARCH
// §Pattern 13); a regulator-versioned catalog file is deferred to Phase 7.
//
// Field semantics:
//   - Name              — sub-band slug ChirpStack expects on the device profile (e.g. "as923_2")
//   - Display           — human label shown in the picker UI
//   - CommonName        — ChirpStack's `commonName` enum value (e.g. "AS923_2")
//   - Group             — UI grouping for the Asia / Europe / Americas / Oceania / India sections
//   - DefaultForCountry — ISO-3166 alpha-2 (e.g. "TH") that pre-selects the entry; empty if no default
//   - Note              — short operator-facing copy explaining a regulatory constraint
type Region struct {
	Name              string `json:"name"`
	Display           string `json:"display"`
	CommonName        string `json:"common_name"`
	Group             string `json:"group"`
	DefaultForCountry string `json:"default_for_country,omitempty"`
	Note              string `json:"note,omitempty"`
}

// Regions returns the Phase 1 hardcoded LoRaWAN region catalog. Order is
// stable so the picker UI is deterministic across reloads. INST-04 anchor:
// AS923-2 carries DefaultForCountry="TH" so the wizard pre-selects the
// Thailand-correct sub-plan (regulator NBTC requirement).
func Regions() []Region {
	return []Region{
		{Name: "as923", Display: "AS923-1", CommonName: "AS923", Group: "asia"},
		{
			Name:              "as923_2",
			Display:           "AS923-2 (Thailand)",
			CommonName:        "AS923_2",
			Group:             "asia",
			DefaultForCountry: "TH",
			Note:              "Required by Thai regulator NBTC.",
		},
		{Name: "as923_3", Display: "AS923-3", CommonName: "AS923_3", Group: "asia"},
		{Name: "as923_4", Display: "AS923-4", CommonName: "AS923_4", Group: "asia"},
		{Name: "eu868", Display: "EU868 (Europe)", CommonName: "EU868", Group: "europe"},
		{Name: "us915_0", Display: "US915 sub-band 1 (ch 0-7)", CommonName: "US915", Group: "americas"},
		{Name: "au915_0", Display: "AU915 sub-band 1", CommonName: "AU915", Group: "oceania"},
		{Name: "in865", Display: "IN865 (India)", CommonName: "IN865", Group: "india"},
	}
}

// RegionByName returns the catalog entry matching name. Plan 15's step 3
// handler uses this to whitelist the operator's pick before persistence
// (T-14-04: never trust the JSONB payload's "name" field as-is).
func RegionByName(name string) (Region, bool) {
	for _, r := range Regions() {
		if r.Name == name {
			return r, true
		}
	}
	return Region{}, false
}
