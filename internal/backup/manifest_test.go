package backup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestManifest_RoundTrip: build a manifest, marshal to JSON, unmarshal, assert
// all 11 fields survive the round trip.
func TestManifest_RoundTrip(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	finished := started.Add(30 * time.Second)
	orig := Manifest{
		ManifestVersion:      "1.0",
		ShifterVersion:       "0.1.0",
		DBSchemaVersion:      "46",
		ChirpStackMode:       "bundled",
		ChirpStackDBIncluded: true,
		InstallID:            "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		InstallSlug:          "acme-water",
		StartedAt:            started,
		FinishedAt:           finished,
		Included:             []string{"db/shifter.dump", "db/chirpstack.dump", "floor-plans/"},
		SHA256Sums:           map[string]string{"db/shifter.dump": "abc123", "db/chirpstack.dump": "def456"},
	}

	b, err := json.Marshal(orig)
	require.NoError(t, err)

	var got Manifest
	require.NoError(t, json.Unmarshal(b, &got))

	require.Equal(t, orig.ManifestVersion, got.ManifestVersion)
	require.Equal(t, orig.ShifterVersion, got.ShifterVersion)
	require.Equal(t, orig.DBSchemaVersion, got.DBSchemaVersion)
	require.Equal(t, orig.ChirpStackMode, got.ChirpStackMode)
	require.Equal(t, orig.ChirpStackDBIncluded, got.ChirpStackDBIncluded)
	require.Equal(t, orig.InstallID, got.InstallID)
	require.Equal(t, orig.InstallSlug, got.InstallSlug)
	require.True(t, orig.StartedAt.Equal(got.StartedAt))
	require.True(t, orig.FinishedAt.Equal(got.FinishedAt))
	require.Equal(t, orig.Included, got.Included)
	require.Equal(t, orig.SHA256Sums, got.SHA256Sums)
}

// TestManifest_VerifyChecksums: write two files, compute sha256_sums, verify
// each checksum matches the actual file content.
func TestManifest_VerifyChecksums(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	files := map[string][]byte{
		"db/shifter.dump":     []byte("PG custom format bytes"),
		"floor-plans/img.png": []byte("PNG image content"),
	}
	sha256s := make(map[string]string, len(files))
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, content, 0o644))

		sha, err := ComputeReaderSHA256(bytes.NewReader(content))
		require.NoError(t, err)
		sha256s[name] = sha
	}

	// Verify via file path.
	for name, expected := range sha256s {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, VerifyFileSHA256(path, expected), "checksum mismatch for %s", name)
	}
}
