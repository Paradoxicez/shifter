package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersion_HasDefault(t *testing.T) {
	require.NotEmpty(t, Version)
	require.NotEmpty(t, ImageTag)
	require.NotEmpty(t, Commit)
	require.NotEmpty(t, BuildTime)
}

// TestImageTagPinned is the OPS-01 unit-level check from VALIDATION.md. It
// asserts the package-level ImageTag constant matches the linker-injected
// Version, so the Docker image tag the binary expects to ship as is the
// same string `shifter version` reports. Plans 20/21 (compose) layer the
// runtime check that the running container's reported tag equals the tag
// in docker-compose.{bundled,external}.yml — defending against the
// `:latest` foot-gun.
func TestImageTagPinned(t *testing.T) {
	require.Equal(t, Version, ImageTag, "OPS-01: container image tag must equal binary version (no :latest)")
}

func TestInfo_ReturnsBuildShape(t *testing.T) {
	info := Info()
	require.Equal(t, Version, info.Version)
	require.Equal(t, Commit, info.Commit)
	require.Equal(t, BuildTime, info.BuildTime)
}
