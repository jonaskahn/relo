package cli

import "strings"

// spawnedDaemon reports whether the arguments are the background daemon that
// SpawnDaemon starts. Only that start passes the hidden --startup-log flag,
// so a foreground `daemon run` keeps its console output.
func spawnedDaemon(args []string) bool {
	for _, arg := range args {
		if arg == "--startup-log" || strings.HasPrefix(arg, "--startup-log=") {
			return true
		}
	}
	return false
}
