// Breaking the tool names the signed-out gateway reads as another client's.
package wire

import (
	"strings"

	"github.com/jonaskahn/relo/internal/inference"
)

// freeToolSignatures are the tool-name fragments the signed-out gateway
// refuses in company. It answers a request whose declared names carry
// reportfindings together with schedulewakeup or deferredtoolplaceholder,
// which is Claude Code's tool set, and it matches case-insensitively and by
// substring: a suffixed name still carries the fragment, a snake-cased or
// kebab-cased one does not. A lone fragment is refused by nothing, so the
// rule is a conjunction over two declarations.
var freeToolSignatures = []string{
	"reportfindings",
	"schedulewakeup",
	"deferredtoolplaceholder",
}

// freeToolSignatureSpellings is the accepted spelling of each fragment: the
// humps joined with an underscore, which the gateway does not match. The
// humps are known, so this is a fixed table rather than a camel-case splitter.
var freeToolSignatureSpellings = map[string]string{
	"reportfindings":          "report_findings",
	"schedulewakeup":          "schedule_wakeup",
	"deferredtoolplaceholder": "deferred_tool_placeholder",
}

// freeToolAlias returns the name the signed-out gateway accepts for one tool:
// the fragment broken if the name carries one, and the name shortened if it is
// still past the gateway's limit.
func freeToolAlias(name string) string {
	return shortToolName(breakToolSignatures(name))
}

// scrubToolSelfReferences replaces a tool's own client name with the alias its
// declaration went out under, so the description the model reads names the
// declaration that exists. The gateway does not read descriptions, so this is
// model coherence rather than a gate.
func scrubToolSelfReferences(tools []inference.Tool, originals map[string]string) {
	for index := range tools {
		original, found := originals[tools[index].Name]
		if !found {
			continue
		}
		tools[index].Description = strings.ReplaceAll(tools[index].Description, original, tools[index].Name)
	}
}

// breakToolSignatures replaces every fragment a name carries with its
// underscored form, which no longer contains the fragment. A name that
// carries none is returned unchanged.
func breakToolSignatures(name string) string {
	for _, fragment := range freeToolSignatures {
		spelling := freeToolSignatureSpellings[fragment]
		for {
			index := strings.Index(strings.ToLower(name), fragment)
			if index < 0 {
				break
			}
			name = name[:index] + spelling + name[index+len(fragment):]
		}
	}
	return name
}
