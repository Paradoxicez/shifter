// Package config provides Shifter's layered configuration loader (D-05) and
// the Compose-secrets file reader (D-06).
//
// Source order (highest precedence first):
//
//  1. SHIFTER_* environment variables (dotted YAML keys map to underscored env
//     names — `db.host` → `SHIFTER_DB_HOST`).
//  2. config.yaml at $SHIFTER_CONFIG_FILE (default `/etc/shifter/config.yaml`).
//  3. Built-in defaults (also documented in `config/config.example.yaml`).
//
// Secrets (DB password, ChirpStack API token, MQTT password, session signing
// key) are resolved by ReadSecret / ReadSecretOrEmpty: a direct SHIFTER_<NAME>
// env var wins for dev paths, otherwise the SHIFTER_<NAME>_FILE path is read
// from disk and CRLF-trimmed (PITFALL #8). Secrets are never bound to YAML
// keys so config.yaml never carries plaintext credentials.
//
// Validate() enforces D-22 (`tls.mode: none` is refused), D-25 (log_level
// whitelist), and the OWASP minimum session-key length (32 bytes).
package config
