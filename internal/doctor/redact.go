// Package doctor provides the diagnostic bundle and PII redaction utilities
// for the `shifter doctor` CLI command (Plan 06-11 / D-50).
//
// The redact sub-package is kept in the same package for cohesion.
// RedactJSON is applied to the serialized bundle bytes before output so
// email addresses in audit rows, config snippets, and any other JSON fields
// are masked to the "j***@example.com" form before the bundle leaves the
// install.
package doctor

import (
	"regexp"
	"strings"
)

// emailRegex matches email-shaped tokens in serialized text / JSON. The
// pattern is intentionally broad — it may match "user@hostname" in connection
// strings but that is acceptable for a support bundle where the goal is to
// avoid PII leakage rather than preserve every string verbatim.
var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// MaskEmail returns the "j***@example.com" form of email. The first character
// of the local part is preserved; the rest is replaced with "***". Returns
// "[redacted]" for tokens that don't match the expected "local@domain" shape.
//
// Examples:
//
//	MaskEmail("john.doe@example.com") → "j***@example.com"
//	MaskEmail("a@x.com")             → "a***@x.com"
//	MaskEmail("not-an-email")        → "[redacted]"
func MaskEmail(email string) string {
	at := strings.Index(email, "@")
	// Require at least one char before "@" and at least one char after.
	if at <= 0 || at == len(email)-1 {
		return "[redacted]"
	}
	return string(email[0]) + "***" + email[at:]
}

// RedactJSON walks the serialized JSON bytes and replaces every email-shaped
// token with its masked form via MaskEmail. The replacement is a pure
// byte-level regex substitution — no JSON parsing overhead, guaranteed to
// cover every field regardless of nesting depth.
func RedactJSON(b []byte) []byte {
	return emailRegex.ReplaceAllFunc(b, func(match []byte) []byte {
		return []byte(MaskEmail(string(match)))
	})
}
