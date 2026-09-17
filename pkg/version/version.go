package version

import (
	"fmt"
	"runtime"
	"strings"
)

// DefaultVersion is the fallback canonical version string.
const DefaultVersion = "1.5.29"

// Build variables populated by Go linker flags (-ldflags) during compilation.
var (
	// Version holds the semantic version of Toron.
	Version = DefaultVersion

	// GitCommit holds the short git SHA of the build.
	GitCommit = "dev"

	// BuildDate holds the ISO-8601 build timestamp.
	BuildDate = "unknown"
)

// Info represents comprehensive build and runtime version metadata.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Compiler  string `json:"compiler"`
	Platform  string `json:"platform"`
}

// Get returns the semantic version string.
func Get() string {
	if Version == "" {
		return DefaultVersion
	}
	return Version
}

// Full returns a formatted string containing version, commit, and build date.
func Full() string {
	return fmt.Sprintf("Toron v%s (commit: %s, built: %s, %s/%s)", Get(), GitCommit, BuildDate, runtime.GOOS, runtime.GOARCH)
}

// GetInfo returns structured version and runtime environment information.
func GetInfo() Info {
	return Info{
		Version:   Get(),
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Compiler:  runtime.Compiler,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// ShortString returns a clean string like "v1.5.29".
func ShortString() string {
	v := Get()
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}
