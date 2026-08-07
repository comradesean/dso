//go:build !windows

// keydump attaches to the Windows DS2 SOTFS client, so it only builds there.
// This stub keeps `go build ./...` working on the Linux side of the project.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "keydump only runs on Windows; cross-compile with:\n"+
		"  GOOS=windows GOARCH=amd64 go build -o keydump.exe ./cmd/keydump")
	os.Exit(1)
}
