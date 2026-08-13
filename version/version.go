// Package version carries build-time identity so a deployed binary can be
// traced back to the exact source it was built from. The values are injected
// at build time via -ldflags -X; without them they fall back to dev/unknown.
package version

import "fmt"

var (
	// Version is a human-readable version tag (e.g. a git tag or "dev").
	Version = "dev"
	// GitCommit is the short git commit the binary was built from.
	GitCommit = "unknown"
	// BuildTime is the UTC timestamp the binary was built at.
	BuildTime = "unknown"
)

// String returns a single-line identity string suitable for startup logs.
func String() string {
	return fmt.Sprintf("%s (commit=%s, built=%s)", Version, GitCommit, BuildTime)
}
