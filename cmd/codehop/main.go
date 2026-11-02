// Command codehop opens any project by name from the terminal.
package main

import (
	"os"
	"runtime/debug"

	"github.com/ardacoskun/codehop/internal/cli"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = ""

func main() {
	os.Exit(cli.Execute(resolveVersion()))
}

// resolveVersion prefers the release version, then the module version that
// `go install ...@vX` records, then "dev".
func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
