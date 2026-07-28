package main

import (
	"fmt"
	"runtime"
)

// Build metadata. Version/Commit/Date default sensibly and are overridable at
// build time via -ldflags "-X main.Version=... -X main.Commit=... -X main.Date=...".
var (
	Version = AppVersion
	Commit  = "unknown"
	Date    = "unknown"
)

func printVersion() {
	fmt.Printf("hedioum-tunnel %s (commit %s, built %s, %s/%s)\n",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
}
