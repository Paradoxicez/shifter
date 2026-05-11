package api

// Phase 3 Wave 2 — POST /api/devices/:eui/keys (reveal secrets).
// D-26 / D-27 / D-28: admin-only, returns OTAA keys or ABP activation,
// writes audit row with NO secret material in `diff`.

import "testing"

// TestRevealSecrets_AdminOTAA — admin POST returns OTAA AppKey + JoinEUI
// from CS GetDeviceKeys.
func TestRevealSecrets_AdminOTAA(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_reveal.go (03-VALIDATION row devices_reveal_test.TestRevealSecrets_AdminOTAA)")
}

// TestRevealSecrets_AdminABP — admin POST returns ABP activation
// (DevAddr + session keys + fcnts) from CS GetDeviceActivation.
func TestRevealSecrets_AdminABP(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_reveal.go (03-VALIDATION row devices_reveal_test.TestRevealSecrets_AdminABP)")
}

// TestRevealSecrets_Viewer403 — viewer role gets HTTP 403; no audit row
// (RBAC denial happens before the handler).
func TestRevealSecrets_Viewer403(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_reveal.go (03-VALIDATION row devices_reveal_test.TestRevealSecrets_Viewer403)")
}

// TestRevealSecrets_NotFound — unknown DevEUI → 404, no audit row.
func TestRevealSecrets_NotFound(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_reveal.go (03-VALIDATION row devices_reveal_test.TestRevealSecrets_NotFound)")
}

// TestRevealSecrets_CSUnreachable502 — CS gRPC dial / RPC fails → 502
// with operator-readable error code.
func TestRevealSecrets_CSUnreachable502(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_reveal.go (03-VALIDATION row devices_reveal_test.TestRevealSecrets_CSUnreachable502)")
}

// TestRevealSecrets_NoCredentials409 — OTAA device that never joined / no
// keys provisioned → 409 with reason `no_credentials`.
func TestRevealSecrets_NoCredentials409(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_reveal.go (03-VALIDATION row devices_reveal_test.TestRevealSecrets_NoCredentials409)")
}

// TestRevealSecrets_AuditNoSecretMaterial — every successful reveal writes
// an audit row with action='device.reveal_secrets'; the `diff` JSONB must
// NOT contain the AppKey / session keys (only the DevEUI + activation mode).
func TestRevealSecrets_AuditNoSecretMaterial(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_reveal.go (03-VALIDATION row devices_reveal_test.TestRevealSecrets_AuditNoSecretMaterial)")
}
