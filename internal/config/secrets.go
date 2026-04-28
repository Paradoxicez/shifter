package config

import (
	"fmt"
	"os"
	"strings"
)

// ReadSecret returns a secret value. Resolution order per D-06 + RESEARCH
// §Pattern 12:
//
//  1. Direct env var (e.g. SHIFTER_DB_PASSWORD="...") — dev only, never used
//     in production Compose deploys because the value would be visible in
//     `docker inspect`.
//  2. File-backed env var (e.g. SHIFTER_DB_PASSWORD_FILE=/run/secrets/...) —
//     the canonical Compose `secrets:` idiom, mounts a tmpfs at
//     /run/secrets/<name>.
//
// Returns an error if neither is set so the caller can decide whether the
// secret is required (Session.Key) or optional (MQTT.Password — see
// ReadSecretOrEmpty).
//
// PITFALL #8: Windows-edited secret files have CRLF line endings and the
// trailing CR ends up in the password, silently corrupting the value. We
// trim BOTH \r and \n.
func ReadSecret(name string) (string, error) {
	if v := os.Getenv(name); v != "" {
		return v, nil
	}
	if path := os.Getenv(name + "_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s_FILE: %w", name, err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	return "", fmt.Errorf("secret %s not set (neither %s nor %s_FILE)", name, name, name)
}

// ReadSecretOrEmpty is ReadSecret with a tolerated "neither set" path: it
// returns ("", nil) when neither env var is defined, but still propagates
// real read errors (e.g. _FILE points at an unreadable path). Use for
// optional secrets such as the MQTT broker password when the broker is
// configured for anonymous access.
//
// "Not set" is detected by the substring "not set" in the error message —
// the alternative would be defining a sentinel error type, but the simpler
// substring check keeps ReadSecret's error messages operator-readable.
func ReadSecretOrEmpty(name string) (string, error) {
	v, err := ReadSecret(name)
	if err != nil {
		if strings.Contains(err.Error(), "not set") {
			return "", nil
		}
		return "", err
	}
	return v, nil
}
