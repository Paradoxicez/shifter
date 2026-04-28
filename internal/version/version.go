// Package version exposes build-time metadata injected via -ldflags at link
// time. Plan 04 (config-secrets) sets these vars; Plan 05 (cobra-cli) prints
// them from `shifter version`; Plans 20/21 (compose) consume Image() to pin
// the Docker image tag.
//
// Production build:
//
//	go build -ldflags "-X github.com/shifter-io/shifter/internal/version.Version=$(git describe) \
//	                   -X github.com/shifter-io/shifter/internal/version.Commit=$(git rev-parse --short HEAD) \
//	                   -X github.com/shifter-io/shifter/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
//	  ./cmd/shifter
package version

// Build-time variables. Default to "dev" sentinels so an unstamped local build
// is still self-describing rather than printing empty strings.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

// BuildInfo is the JSON shape `shifter version --json` emits.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// Info returns the current build metadata.
func Info() BuildInfo {
	return BuildInfo{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}
}
