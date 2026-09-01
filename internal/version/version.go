package version

import "fmt"

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func String() string {
	v := Version
	if v == "" {
		v = "dev"
	}
	if Commit == "" && Date == "" {
		return "brainwash-cli " + v
	}
	if Date == "" {
		return fmt.Sprintf("brainwash-cli %s (%s)", v, Commit)
	}
	if Commit == "" {
		return fmt.Sprintf("brainwash-cli %s (%s)", v, Date)
	}
	return fmt.Sprintf("brainwash-cli %s (%s %s)", v, Commit, Date)
}
