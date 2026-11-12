// OpenCode environment instructions satisfy the free lane's system-message gate.
package wire

import (
	"os"
	"strings"

	"github.com/jonaskahn/relo/internal/inference"
)

// HasOpenCodeSystemPrompt reports whether a system message carries the environment
// block the free gateway expects, regardless of message order.
func HasOpenCodeSystemPrompt(req *inference.Request) bool {
	if req == nil {
		return false
	}
	for _, message := range req.Messages {
		if message.Role == inference.RoleSystem && hasOpenCodeEnvironment(spokenText(message.Content)) {
			return true
		}
	}
	return false
}

func hasOpenCodeEnvironment(text string) bool {
	for {
		_, rest, found := strings.Cut(text, "<env>")
		if !found {
			return false
		}
		body, tail, closed := strings.Cut(rest, "</env>")
		if !closed {
			return false
		}
		for line := range strings.SplitSeq(body, "\n") {
			if value, ok := strings.CutPrefix(strings.TrimSpace(line), "Working directory:"); ok && strings.TrimSpace(value) != "" {
				return true
			}
		}
		text = tail
	}
}

func spokenText(parts []inference.ContentPart) string {
	var text strings.Builder
	for _, part := range parts {
		if part.Type == inference.ContentTypeText {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

func openCodeEnvironmentInstruction(directory string) string {
	if strings.TrimSpace(directory) == "" || strings.ContainsAny(directory, "\r\n<>") {
		var err error
		directory, err = os.Getwd()
		if err != nil {
			// A removed daemon cwd has no absolute name; use its relative spelling.
			directory = "."
		}
	}
	return "<env>\n  Working directory: " + directory + "\n</env>"
}

func withOpenCodeSystemPrompt(messages []inference.Message, directory string) []inference.Message {
	instruction := inference.ContentPart{Type: inference.ContentTypeText, Text: "\n\n" + openCodeEnvironmentInstruction(directory)}
	copied := append([]inference.Message(nil), messages...)
	for index, message := range copied {
		if message.Role != inference.RoleSystem {
			continue
		}
		copied[index].Content = append(append([]inference.ContentPart(nil), message.Content...), instruction)
		return copied
	}
	return append([]inference.Message{{Role: inference.RoleSystem, Content: []inference.ContentPart{instruction}}}, copied...)
}
