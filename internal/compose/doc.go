// Package compose contains automated lint tests for the Shifter compose files.
//
// The tests in conventions_test.go assert that both compose/bundled.yml and
// compose/external.yml conform to the Shifter compose conventions defined in
// Plan 06-11 / docs/operator-runbook.md:
//
//   - All image tags are pinned (no :latest)
//   - Every service block has a json-file logging section
//   - No credentials are leaked via environment variables (must use Compose secrets)
//   - All referenced secrets have a corresponding secrets: declaration
//   - The backups named volume exists and is mounted in the shifter service
//   - No internal services (postgres, mosquitto, redis, chirpstack) bind host ports
//
// Run: go test ./internal/compose/...
//
// These tests run in CI on every PR so convention drift fails the build before
// it reaches the operator's install (OPS-05/06/07 enforcement).
package compose
