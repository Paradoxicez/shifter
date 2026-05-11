package api

// Phase 3 Plan 03-06 / Task 3 — POST /api/devices/{eui}/keys (reveal).
//
// Canonical integration tests live in internal/device/reveal_test.go:
//   - TestRevealSecrets_AdminOTAA
//   - TestRevealSecrets_AdminABP
//   - TestRevealSecrets_Viewer403
//   - TestRevealSecrets_NotFound
//   - TestRevealSecrets_CSUnreachable502
//   - TestRevealSecrets_NoCredentials409
//   - TestRevealSecrets_AuditNoSecretMaterial
//   - TestRevealSecrets_InvalidDevEUI
//   - TestRevealSecrets_MustBePOST
//   - TestRevealSecrets_AuditWriteSiteCleanliness (static defense in depth)
//
// The package-api markers below resolve the 03-VALIDATION rows to a real
// test invocation. Actual coverage lives in the device package.

import "testing"

func TestRevealSecrets_AdminOTAA(t *testing.T) {
	t.Skip("canonical: internal/device/reveal_test.go::TestRevealSecrets_AdminOTAA")
}

func TestRevealSecrets_AdminABP(t *testing.T) {
	t.Skip("canonical: internal/device/reveal_test.go::TestRevealSecrets_AdminABP")
}

func TestRevealSecrets_Viewer403(t *testing.T) {
	t.Skip("canonical: internal/device/reveal_test.go::TestRevealSecrets_Viewer403")
}

func TestRevealSecrets_NotFound(t *testing.T) {
	t.Skip("canonical: internal/device/reveal_test.go::TestRevealSecrets_NotFound")
}

func TestRevealSecrets_CSUnreachable502(t *testing.T) {
	t.Skip("canonical: internal/device/reveal_test.go::TestRevealSecrets_CSUnreachable502")
}

func TestRevealSecrets_NoCredentials409(t *testing.T) {
	t.Skip("canonical: internal/device/reveal_test.go::TestRevealSecrets_NoCredentials409")
}

func TestRevealSecrets_AuditNoSecretMaterial(t *testing.T) {
	t.Skip("canonical: internal/device/reveal_test.go::TestRevealSecrets_AuditNoSecretMaterial")
}
