// Package version exposes build-time metadata injected via -ldflags at
// link time and the canonical Docker image tag the binary ships under
// (OPS-01). The compose smoke tests in Plans 20/21 assert that the tag
// pinned in docker-compose.{bundled,external}.yml matches version.ImageTag
// — guarding against `:latest` drift between binary and image.
package version
