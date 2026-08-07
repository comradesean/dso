//go:build !windows

// memscan reads another Windows process's memory, so it only builds there.
// This stub keeps `go build ./...` working on the Linux side of the project.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "memscan only runs on Windows; cross-compile with:\n"+
		"  GOOS=windows GOARCH=amd64 go build -o tools/memscan/memscan.exe ./tools/memscan")
	os.Exit(1)
}
