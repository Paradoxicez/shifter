package doctor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRedact_MaskEmailSimple — standard multi-part local address.
func TestRedact_MaskEmailSimple(t *testing.T) {
	require.Equal(t, "j***@example.com", MaskEmail("john.doe@example.com"))
}

// TestRedact_MaskEmailShort — single-char local part.
func TestRedact_MaskEmailShort(t *testing.T) {
	require.Equal(t, "a***@x.com", MaskEmail("a@x.com"))
}

// TestRedact_MaskEmailMalformed — no "@" present.
func TestRedact_MaskEmailMalformed(t *testing.T) {
	require.Equal(t, "[redacted]", MaskEmail("not-an-email"))
}

// TestRedact_MaskEmailAtStart — "@" at position 0 (empty local).
func TestRedact_MaskEmailAtStart(t *testing.T) {
	require.Equal(t, "[redacted]", MaskEmail("@example.com"))
}

// TestRedact_MaskEmailAtEnd — "@" at last position (empty domain).
func TestRedact_MaskEmailAtEnd(t *testing.T) {
	require.Equal(t, "[redacted]", MaskEmail("user@"))
}

// TestRedact_JSONReplacesEmailTokens — single email in a JSON object.
func TestRedact_JSONReplacesEmailTokens(t *testing.T) {
	input := []byte(`{"user":{"email":"jdoe@acme.io"}}`)
	out := RedactJSON(input)
	require.Contains(t, string(out), "j***@acme.io")
	require.NotContains(t, string(out), "jdoe@acme.io")
}

// TestRedact_JSONHandlesMultipleEmails — array of 3 audit rows each with an
// email; all 3 must be masked.
func TestRedact_JSONHandlesMultipleEmails(t *testing.T) {
	input := []byte(`[
		{"user_email":"alice@x.com","action":"create"},
		{"user_email":"bob@y.org","action":"update"},
		{"user_email":"carol@z.net","action":"archive"}
	]`)
	out := string(RedactJSON(input))
	require.Contains(t, out, "a***@x.com")
	require.Contains(t, out, "b***@y.org")
	require.Contains(t, out, "c***@z.net")
	require.NotContains(t, out, "alice@x.com")
	require.NotContains(t, out, "bob@y.org")
	require.NotContains(t, out, "carol@z.net")
}

// TestRedact_JSONNoEmailsPassthrough — JSON with no email-shaped tokens is
// returned unchanged.
func TestRedact_JSONNoEmailsPassthrough(t *testing.T) {
	input := []byte(`{"status":"ok","version":"0.6.0"}`)
	out := RedactJSON(input)
	require.Equal(t, string(input), string(out))
}
