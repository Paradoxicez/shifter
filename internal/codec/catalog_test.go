package codec_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"

	"github.com/shifter-io/shifter/internal/codec"
	"github.com/shifter-io/shifter/internal/profile/codecs"
)

func TestCatalogValid(t *testing.T) {
	entries, err := codec.LoadAll()
	require.NoError(t, err, "catalog.LoadAll failed — check catalog/*.json syntax")
	require.NotEmpty(t, entries, "catalog is empty")

	validCurves := map[string]bool{"linear_pct": true, "li_socl2_3v6": true, "li_mnox_3v0": true, "alkaline_3v0": true, "none": true}
	validAnomalyCompat := map[string]bool{"full": true, "limited": true, "unsupported": true}
	validCapabilities := map[string]bool{
		"cumulative": true, "flow_rate": true, "instant_power": true, "battery": true,
		"temperature": true, "pressure": true, "leak_detection": true, "tamper_detection": true,
		"multi_phase": true, "power_quality": true,
	}

	slugs := map[string]bool{}
	for _, e := range entries {
		t.Run(e.Slug, func(t *testing.T) {
			require.NotEmpty(t, e.Slug)
			require.Equal(t, strings.ToLower(e.Slug), e.Slug, "slug must be lowercase")
			require.False(t, slugs[e.Slug], "duplicate slug %q", e.Slug)
			slugs[e.Slug] = true
			require.NotEmpty(t, e.Name)
			require.NotEmpty(t, e.Vendor)
			require.NotEmpty(t, e.Family)
			require.NotEmpty(t, e.Version)
			require.True(t, semver.IsValid("v"+e.Version), "version must be semver: %q", e.Version)
			require.NotEmpty(t, e.CodecJSPath)
			require.True(t, validCurves[e.BatteryCurve], "unknown battery_curve %q", e.BatteryCurve)
			require.True(t, validAnomalyCompat[e.AnomalyCompatibility], "unknown anomaly_compatibility %q", e.AnomalyCompatibility)
			require.Greater(t, e.ExpectedUplinkIntervalSeconds, 0)
			require.Greater(t, e.OfflineThresholdMultiplier, 0.0)
			require.NotEmpty(t, e.Capabilities)
			for _, cap := range e.Capabilities {
				require.True(t, validCapabilities[cap], "unknown capability %q in %q", cap, e.Slug)
			}
			// D-22: codec_js_path must resolve to an embedded codec
			codecJS := codecs.CodecBySlug(e.Slug)
			require.NotEmpty(t, codecJS, "codec_js_path %q has no embedded JS (CodecBySlug returned empty)", e.CodecJSPath)
		})
	}

	require.Equal(t, 4, len(entries), "expected exactly 4 catalog entries for Phase 7 v1.0.0")
}

func TestGet_NotFound(t *testing.T) {
	_, err := codec.Get("does_not_exist_v0")
	require.ErrorIs(t, err, codec.ErrCatalogEntryNotFound)
}
