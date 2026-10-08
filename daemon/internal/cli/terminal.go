// Where an uninstall can ask its one question.
package cli

import (
	"io"
	"os"
)

func findTerminal() (io.Reader, bool) {
	if !isTerminal(os.Stdin) {
		return nil, false
	}
	return os.Stdin, true
}
