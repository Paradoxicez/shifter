// Package codec ships the embedded Phase 7 vendor catalog. Entries are
// JSON files at internal/codec/catalog/*.json bundled via //go:embed.
// D-01, D-21, D-33.
package codec

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

//go:embed all:catalog
var catalogFS embed.FS

// ErrCatalogEntryNotFound is returned by Get when slug has no entry.
var ErrCatalogEntryNotFound = errors.New("codec: catalog entry not found")

// CatalogEntry mirrors the D-33 locked schema. See 07-CONTEXT.md §D-33.
type CatalogEntry struct {
	Slug                          string   `json:"slug"`
	Name                          string   `json:"name"`
	Vendor                        string   `json:"vendor"`
	Family                        string   `json:"family"`
	Capabilities                  []string `json:"capabilities"`
	Version                       string   `json:"version"`
	CodecJSPath                   string   `json:"codec_js_path"`
	CounterModulus                int64    `json:"counter_modulus"`
	MACVersion                    string   `json:"mac_version"`
	Region                        *string  `json:"region"`
	ExpectedUplinkIntervalSeconds int      `json:"expected_uplink_interval_seconds"`
	OfflineThresholdMultiplier    float64  `json:"offline_threshold_multiplier"`
	AnomalyCompatibility          string   `json:"anomaly_compatibility"`
	BatteryCurve                  string   `json:"battery_curve"`
	VendorHasSeparateMeterSerial  bool     `json:"vendor_has_separate_meter_serial"`
	FPort                         *int     `json:"fPort,omitempty"`
}

// LoadAll returns all embedded catalog entries sorted by vendor then family.
// The catalog/*.json files are bundled at build time via //go:embed.
// D-22 build-time guarantee: TestCatalogValid validates every entry at test time.
func LoadAll() ([]CatalogEntry, error) {
	dirEntries, err := catalogFS.ReadDir("catalog")
	if err != nil {
		return nil, fmt.Errorf("read catalog dir: %w", err)
	}
	var out []CatalogEntry
	for _, e := range dirEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := catalogFS.ReadFile("catalog/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var entry CatalogEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("unmarshal %s: %w", e.Name(), err)
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Vendor != out[j].Vendor {
			return out[i].Vendor < out[j].Vendor
		}
		return out[i].Family < out[j].Family
	})
	return out, nil
}

// Get returns the catalog entry for slug or ErrCatalogEntryNotFound.
func Get(slug string) (CatalogEntry, error) {
	all, err := LoadAll()
	if err != nil {
		return CatalogEntry{}, err
	}
	for _, e := range all {
		if e.Slug == slug {
			return e, nil
		}
	}
	return CatalogEntry{}, ErrCatalogEntryNotFound
}
