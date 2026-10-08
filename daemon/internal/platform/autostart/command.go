//go:build !darwin

// The managers whose registration writes a quoted command line share this
// parser: the XDG Exec line on Linux and the Run value on Windows. A Darwin
// launch agent names the executable in a property of its own, so this file is
// not part of that build.
package autostart

import "strings"

func splitQuotedExecutable(command string) (string, bool) {
	if len(command) == 0 || command[0] != '"' {
		return "", false
	}
	end := strings.IndexByte(command[1:], '"')
	if end < 0 {
		return "", false
	}
	return command[1 : 1+end], true
}
