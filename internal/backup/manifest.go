package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
)

// Manifest is the JSON structure written as the last entry inside every
// backup tarball (manifest.json).  The schema is locked at version "1.0"
// for the v1 release cycle.  Future additions MUST be backward-compatible
// (new optional fields only).
//
// Schema reference: RESEARCH §Decision A "Manifest Schema (D-39)".
type Manifest struct {
	ManifestVersion      string            `json:"manifest_version"`        // always "1.0"
	ShifterVersion       string            `json:"shifter_version"`          // e.g. "0.1.0"
	DBSchemaVersion      string            `json:"db_schema_version"`        // e.g. "46"
	ChirpStackMode       string            `json:"chirpstack_mode"`          // "bundled" | "external"
	ChirpStackDBIncluded bool              `json:"chirpstack_db_included"`   // true only in bundled mode
	InstallID            string            `json:"install_id"`               // UUID of install_identity row
	InstallSlug          string            `json:"install_slug"`             // human-readable slug
	StartedAt            time.Time         `json:"started_at"`
	FinishedAt           time.Time         `json:"finished_at"`
	Included             []string          `json:"included"`                 // ordered list of archive paths
	SHA256Sums           map[string]string `json:"sha256_sums"`              // archive-path → hex sha256
}

// ComputeFileSHA256 computes the SHA-256 checksum of the file at path and
// returns the result as a lowercase hex string.
func ComputeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()
	return computeReaderSHA256(f)
}

// ComputeReaderSHA256 computes SHA-256 of all bytes available from r.
func ComputeReaderSHA256(r io.Reader) (string, error) {
	return computeReaderSHA256(r)
}

func computeReaderSHA256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("sha256: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyFileSHA256 checks that the file at path matches expected (hex).
// Returns a descriptive error if the file cannot be read or the checksum
// does not match.
func VerifyFileSHA256(path, expected string) error {
	got, err := ComputeFileSHA256(path)
	if err != nil {
		return err
	}
	if got != expected {
		return fmt.Errorf("sha256 mismatch for %q: got %s, expected %s", path, got, expected)
	}
	return nil
}
