// Package main is the executable entry point: it parses the operator's
// command and delegates to the platform for daemon lifecycle and work.
package main

import (
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/cli"
	"github.com/jonaskahn/relo/internal/platform"
)

func main() {
	// AppKit and the Windows message loop want the process main thread, and a
	// Go goroutine only starts out there: lock before anything can schedule
	// this one onto another thread. The whole desktop — AppKit, the tray, the
	// event loop — runs on this goroutine.
	runtime.LockOSThread()
	var exiting sync.Once
	onStopping := func() { exiting.Do(func() { time.AfterFunc(platform.ExitTimeout, func() { os.Exit(1) }) }) }
	if err := cli.ExecuteWithShutdown(onStopping); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
