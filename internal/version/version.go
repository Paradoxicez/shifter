package version

// Build-time variables. Default to "dev" sentinels so an unstamped local
// build is still self-describing rather than printing empty strings. The
// production Justfile recipe (Plan 24) injects real values via:
//
//	go build -ldflags "
//	  -X github.com/shifter-io/shifter/internal/version.Version=$(git describe --always --dirty)
//	  -X github.com/shifter-io/shifter/internal/version.Commit=$(git rev-parse --short HEAD)
//	  -X github.com/shifter-io/shifter/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
//	" -o bin/shifter ./cmd/shifter
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

// ImageTag is the Docker image tag this binary expects to ship as. The
// compose smoke tests (Plans 20/21) assert that the tag in
// docker-compose.{bundled,external}.yml matches this value, defending
// against the `:latest` foot-gun (PITFALL: drift between image tag and
// running binary version when `:latest` is allowed). Initialized from
// Version so a single -ldflags injection updates both.
var ImageTag = Version

// BuildInfo is the JSON shape `shifter version --json` emits and the
// payload `/health` (Plan 18) embeds.
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
