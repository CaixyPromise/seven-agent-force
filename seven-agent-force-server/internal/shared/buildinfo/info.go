package buildinfo

import "fmt"

var (
	// Version is the semantic version injected by the release build.
	Version = "dev"
	// Commit is the source revision injected by the release build.
	Commit = "unknown"
	// BuildDate is the UTC build timestamp injected by the release build.
	BuildDate = "unknown"
)

// String returns the public build identity without runtime filesystem details.
func String() string {
	return fmt.Sprintf("seven-agent-force-server version=%s commit=%s buildDate=%s", Version, Commit, BuildDate)
}
