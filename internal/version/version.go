package version

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Full() string {
	return fmt.Sprintf("git-wrangler %s\ncommit: %s\nbuilt: %s", Version, Commit, Date)
}
