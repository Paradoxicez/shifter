// Package codec ships the embedded Phase 7 vendor catalog. Entries are
// JSON files at internal/codec/catalog/*.json bundled via //go:embed.
// D-01, D-21, D-33.
package codec

import (
	"embed"
)

//go:embed all:catalog
var catalogFS embed.FS

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

// LoadAll returns every embedded catalog entry. Implementation in plan 07-03.
func LoadAll() ([]CatalogEntry, error) { return nil, nil }
