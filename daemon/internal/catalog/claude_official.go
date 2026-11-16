// The Claude models Anthropic publishes under its own ids.
package catalog

import "strings"

// Claude model families, as a console groups them.
const (
	FamilyOpus   = "opus"
	FamilySonnet = "sonnet"
	FamilyHaiku  = "haiku"
	FamilyFable  = "fable"
	FamilyMythos = "mythos"
	FamilyOther  = "other"
)

// ClaudeOfficialSeed lists the native Claude ids this build knows, newest
// first within a family. It is only the starting point: the ids an account is
// actually offered come from the catalog the vendor serves it.
func ClaudeOfficialSeed() []string {
	return []string{
		"claude-fable-5-1", "claude-fable-5",
		"claude-mythos-5-1", "claude-mythos-5",
		"claude-opus-5-5", "claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6",
		"claude-sonnet-5-5", "claude-sonnet-5", "claude-sonnet-4-6", "claude-sonnet-4-5",
		"claude-haiku-5-5", "claude-haiku-4-5",
	}
}

// ClaudeFamily names the family one native Claude id belongs to.
func ClaudeFamily(id string) string {
	lower := strings.ToLower(id)
	for _, family := range []string{FamilyOpus, FamilySonnet, FamilyHaiku, FamilyFable, FamilyMythos} {
		if strings.Contains(lower, "-"+family) {
			return family
		}
	}
	return FamilyOther
}
