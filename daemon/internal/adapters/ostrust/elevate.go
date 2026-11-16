package ostrust

import (
	"encoding/base64"
	"strings"
	"unicode/utf16"
)

// shellQuote makes one word safe inside a POSIX shell command line.
func shellQuote(word string) string {
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// shellLine joins words into one command line, each quoted.
func shellLine(words ...string) string {
	quoted := make([]string, len(words))
	for index, word := range words {
		quoted[index] = shellQuote(word)
	}
	return strings.Join(quoted, " ")
}

// appleScriptString makes text safe inside an AppleScript string literal.
func appleScriptString(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, `"`, `\"`)
	return `"` + text + `"`
}

// encodePowerShell is the base64 of a script's UTF-16 bytes, which is what
// powershell.exe -EncodedCommand reads, so a script crosses a second command
// line without any quoting of its own.
func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		raw = append(raw, byte(unit), byte(unit>>8))
	}
	return base64.StdEncoding.EncodeToString(raw)
}
