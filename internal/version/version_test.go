package version

import "testing"

// TestImageTagPinned — Both `docker-compose.bundled.yml` and
// `docker-compose.external.yml` reference `shifter:<version>` (never
// `shifter:latest`). Build-time `version.Version` must be non-empty in
// release artifacts.
// Implementation: Plan 20/21 (compose-bundled, compose-external) + this
// package's Version constant.
func TestImageTagPinned(t *testing.T) {
	t.Skip("Plan 20/21: pinned image tag check pending")
}
