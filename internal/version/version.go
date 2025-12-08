package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the semantic version (e.g., "1.0.0")
	// Set via ldflags: -X github.com/jtzemp/dogfetch/internal/version.Version=1.0.0
	Version = "dev"

	// Commit is the git commit hash
	// Set via ldflags: -X github.com/jtzemp/dogfetch/internal/version.Commit=abc123
	Commit = "unknown"

	// Date is the build date
	// Set via ldflags: -X github.com/jtzemp/dogfetch/internal/version.Date=2024-01-01
	Date = "unknown"
)

// Info returns formatted version information
func Info() string {
	return fmt.Sprintf("dogfetch %s (commit: %s, built: %s, go: %s)",
		Version,
		Commit,
		Date,
		runtime.Version())
}

// Short returns just the version number
func Short() string {
	if Version == "dev" {
		return fmt.Sprintf("%s-%s", Version, Commit[:7])
	}
	return Version
}
