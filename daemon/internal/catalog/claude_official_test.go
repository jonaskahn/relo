package catalog

import "testing"

func TestNativeModelKeyDropsOnlyATrailingReleaseDate(t *testing.T) {
	cases := map[string]string{
		"claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
		"claude-sonnet-4-5":          "claude-sonnet-4-5",
		"claude-opus-5":              "claude-opus-5",
		" claude-haiku-4-5-20251001": "claude-haiku-4-5",
		"claude-3-5-20240101x":       "claude-3-5-20240101x",
	}
	for input, want := range cases {
		if got := NativeModelKey(input); got != want {
			t.Fatalf("NativeModelKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestClaudeFamilyReadsTheFamilyOutOfTheID(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5-5":   FamilyOpus,
		"claude-sonnet-4-6": FamilySonnet,
		"claude-haiku-4-5":  FamilyHaiku,
		"claude-fable-5-1":  FamilyFable,
		"claude-mythos-5":   FamilyMythos,
		"claude-3-5-foo":    FamilyOther,
	}
	for input, want := range cases {
		if got := ClaudeFamily(input); got != want {
			t.Fatalf("ClaudeFamily(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestClaudeOfficialSeedHoldsOnlyNativeIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range ClaudeOfficialSeed() {
		if !NativeClaudeID(id) {
			t.Fatalf("seed id %q is not a native Claude id", id)
		}
		if seen[id] {
			t.Fatalf("seed id %q is listed twice", id)
		}
		seen[id] = true
	}
}
