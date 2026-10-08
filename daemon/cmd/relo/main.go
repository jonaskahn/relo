// Package main is the executable entry point: it parses the operator's
// command and delegates to the platform for daemon lifecycle and work.
package main

import (
	"os"
	"runtime"

	"github.com/jonaskahn/relo/internal/cli"
)

func main() {
	// AppKit and the Windows message loop want the process main thread, and a
	// Go goroutine only starts out there: lock before anything can schedule
	// this one onto another thread. The whole desktop — AppKit, the tray, the
	// event loop — runs on this goroutine.
	runtime.LockOSThread()
	if err := cli.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
