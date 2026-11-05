// Claude Desktop behavior mapping: the requested behavior and the compatible
// value written into a profile row.
package codingclients

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

func TestDesktopBehavesAsClassifiesModelIdentity(t *testing.T) {
	cases := []struct {
		name  string
		model ModelRef
		want  string
	}{
		{name: "sonnet 4.6", model: ModelRef{ID: "claude-relo-a--claude-sonnet-4-6"}, want: "claude-sonnet-4-6"},
		{name: "sonnet 5", model: ModelRef{ID: "claude-sonnet-5"}, want: "claude-sonnet-5"},
		{name: "sonnet 4.5", model: ModelRef{ID: "claude-sonnet-4-5"}, want: "claude-sonnet-4-5"},
		{name: "sonnet 5.5 falls back", model: ModelRef{ID: "claude-sonnet-5-5"}, want: "claude-sonnet-5"},
		{name: "haiku 4.5", model: ModelRef{ID: "claude-haiku-4-5"}, want: "claude-haiku-4-5"},
		{name: "haiku 5.5 falls back", model: ModelRef{ID: "claude-haiku-5-5"}, want: "claude-haiku-4-5"},
		{name: "opus 4.6", model: ModelRef{ID: "claude-opus-4-6"}, want: "claude-opus-4-6"},
		{name: "opus 4.7", model: ModelRef{ID: "claude-opus-4-7"}, want: "claude-opus-4-7"},
		{name: "opus 4.8", model: ModelRef{ID: "claude-opus-4-8"}, want: "claude-opus-4-8"},
		{name: "opus 5", model: ModelRef{ID: "claude-opus-5"}, want: "claude-opus-5"},
		{name: "fable 5", model: ModelRef{ID: "claude-fable-5"}, want: "claude-fable-5"},
		{name: "fable 5.1", model: ModelRef{ID: "claude-fable-5-1"}, want: "claude-fable-5-1"},
		{name: "unversioned fable keeps its family", model: ModelRef{ID: "claude-fable"}, want: "claude-fable-5"},
		{name: "mythos 5.1", model: ModelRef{ID: "claude-mythos-5-1"}, want: "claude-mythos-5-1"},
		{name: "mythos 5", model: ModelRef{ID: "claude-mythos-5"}, want: "claude-mythos-5"},
		{name: "unversioned mythos keeps its family", model: ModelRef{ID: "claude-mythos"}, want: "claude-mythos-5"},
		{name: "unlisted opus keeps its family", model: ModelRef{ID: "claude-opus-9-9"}, want: "claude-opus-5"},
		{name: "opus 5.5 falls back", model: ModelRef{ID: "claude-opus-5-5"}, want: "claude-opus-5"},
		{name: "unlisted haiku keeps its family", model: ModelRef{ID: "claude-haiku-9"}, want: "claude-haiku-4-5"},
		{name: "version first", model: ModelRef{ID: "claude-3-5-sonnet"}, want: "claude-sonnet-5"},
		{name: "namespace", model: ModelRef{ID: "anthropic/claude-sonnet-4.6"}, want: "claude-sonnet-4-6"},
		{name: "bedrock wrapper", model: ModelRef{ID: "anthropic.claude-sonnet-4-6-v1:0"}, want: "claude-sonnet-4-6"},
		{name: "bedrock region", model: ModelRef{ID: "us.anthropic.claude-opus-4-8-v1"}, want: "claude-opus-4-8"},
		{name: "dated release", model: ModelRef{ID: "claude-haiku-4-5-20251001"}, want: "claude-haiku-4-5"},
		{name: "context marker", model: ModelRef{ID: "claude-opus-4-8[1m]"}, want: "claude-opus-4-8"},
		{name: "case and whitespace", model: ModelRef{ID: "  CLAUDE-Sonnet-5  "}, want: "claude-sonnet-5"},
		{name: "revision suffix", model: ModelRef{ID: "gpt-5.5-pro@2026-01-01"}, want: "claude-opus-4-6"},
		{name: "similar but unrelated", model: ModelRef{ID: "claude-ish-model"}, want: "claude-sonnet-5"},
		{name: "gpt pro", model: ModelRef{ID: "gpt-5.5-pro"}, want: "claude-opus-4-6"},
		{name: "namespaced gpt pro", model: ModelRef{ID: "openai/gpt-5.5-pro"}, want: "claude-opus-4-6"},
		{name: "gpt astra", model: ModelRef{ID: "gpt-5.5-astra"}, want: "claude-fable-5"},
		{name: "gpt sol", model: ModelRef{ID: "gpt-5.5-sol"}, want: "claude-sonnet-5"},
		{name: "gpt terra", model: ModelRef{ID: "gpt-5.5-terra"}, want: "claude-haiku-4-5"},
		{name: "gpt luna", model: ModelRef{ID: "gpt-5.5-luna"}, want: "claude-haiku-4-5"},
		{name: "gpt mini", model: ModelRef{ID: "gpt-5.5-mini"}, want: "claude-haiku-4-5"},
		{name: "gpt nano", model: ModelRef{ID: "gpt-5.5-nano"}, want: "claude-haiku-4-5"},
		{name: "gpt codex", model: ModelRef{ID: "gpt-5.5-codex"}, want: "claude-sonnet-4-6"},
		{name: "pro wins over mini", model: ModelRef{ID: "gpt-5.5-pro-mini"}, want: "claude-opus-4-6"},
		{name: "astra wins over sol", model: ModelRef{ID: "gpt-5.5-astra-sol"}, want: "claude-fable-5"},
		{name: "professional is not pro", model: ModelRef{ID: "gpt-5.5-professional"}, want: "claude-sonnet-4-6"},
		{name: "gpt flash stays gpt", model: ModelRef{ID: "gpt-5.5-flash"}, want: "claude-sonnet-4-6"},
		{name: "gemini flash", model: ModelRef{ID: "google/gemini-3.8-flash"}, want: "claude-sonnet-5"},
		{name: "deepseek flash", model: ModelRef{ID: "deepseek-flash"}, want: "claude-sonnet-5"},
		{name: "other family", model: ModelRef{ID: "qwen/qwen-4"}, want: "claude-sonnet-5"},
		{name: "a non-gpt mini is not a gpt variant", model: ModelRef{ID: "qwen-mini"}, want: "claude-sonnet-5"},
		{name: "opaque id", model: ModelRef{ID: "some-model"}, want: "claude-sonnet-5"},
		{name: "an undecoded relo id is opaque", model: ModelRef{ID: "relo-flash-gateway-gpt-5.5-pro"}, want: "claude-sonnet-5"},
		{name: "legacy desktop alias is opaque", model: ModelRef{ID: "claude-opus-4-8-rabcdef123456"}, want: "claude-sonnet-5"},
		{name: "desktop alias is opaque", model: ModelRef{ID: "claude-relo-abcdef123456"}, want: "claude-sonnet-5"},
		{name: "route flash", model: ModelRef{ID: "reloc-fast-flash", Connection: "Relo"}, want: "claude-sonnet-5"},
		{name: "route default", model: ModelRef{ID: "reloc-combo", Connection: "Relo"}, want: "claude-sonnet-5"},
		{name: "route named gpt pro", model: ModelRef{ID: "reloc-combo", Name: "GPT Pro", Connection: "Relo"}, want: "claude-sonnet-5"},
		{name: "claude route alias", model: ModelRef{ID: "claude-reloc-flash"}, want: "claude-sonnet-5"},
		{name: "upstream wins over alias", model: ModelRef{ID: "claude-relo-x--local-clone", Upstream: "gpt-5.5-pro"}, want: "claude-opus-4-6"},
		{name: "a clone is classified by its upstream", model: ModelRef{ID: "claude-relo-x--my-alias", Upstream: "claude-haiku-4-5"}, want: "claude-haiku-4-5"},
		{name: "changing the upstream changes the behavior", model: ModelRef{ID: "claude-relo-x--my-alias", Upstream: "gpt-5.5-sol"}, want: "claude-sonnet-5"},
		{name: "the same alias with another upstream", model: ModelRef{ID: "claude-relo-x--my-alias", Upstream: "gpt-5.5-pro"}, want: "claude-opus-4-6"},
		{name: "misleading label ignored", model: ModelRef{ID: "some-model", Name: "Claude Opus 4.6"}, want: "claude-sonnet-5"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := desktopBehavesAs(testCase.model); got != testCase.want {
				t.Fatalf("desktopBehavesAs(%+v) = %q, want %q", testCase.model, got, testCase.want)
			}
		})
	}
}

func TestDesktopModelsCarryBehavesAs(t *testing.T) {
	reasoningOff := false
	models := []ModelRef{
		{ID: "claude-relo-a--claude-sonnet-5-5", Upstream: "claude-sonnet-5-5", Reasoning: &reasoningOff},
		{ID: "claude-relo-b--gpt-5.5-pro", Upstream: "gpt-5.5-pro"},
		{ID: "claude-reloc-combo", Connection: "Relo"},
	}
	rows := desktopModels(models)
	if len(rows) != len(models) {
		t.Fatalf("rows = %d, want %d", len(rows), len(models))
	}
	want := []string{"claude-sonnet-5", "claude-opus-4-6", "claude-sonnet-5"}
	for index, row := range rows {
		if row.BehavesAs != want[index] {
			t.Fatalf("row %d behavesAs = %q, want %q", index, row.BehavesAs, want[index])
		}
	}
	if !rows[0].IsFamilyDefault {
		t.Fatal("the first row is no longer the family default")
	}
}

func TestDesktopRowNamePublishesExactIdsOnlyWhenDesktopKeepsThem(t *testing.T) {
	million := int64(catalog.MillionContext)
	cases := []struct {
		name  string
		model ModelRef
		want  string
	}{
		{name: "gpt model is opaque", model: ModelRef{ID: "gpt-5.5"}},
		{name: "kimi model is opaque", model: ModelRef{ID: "kimi-k3"}},
		{name: "glm model is opaque", model: ModelRef{ID: "glm-5"}},
		{name: "family word is opaque", model: ModelRef{ID: "claude-relo-anthropic--claude-opus-4-8"}},
		{name: "standalone digit segment is opaque", model: ModelRef{ID: "claude-reloc-2"}},
		{name: "a claude route keeps its name", model: ModelRef{ID: "claude-reloc-fast"}, want: "claude-reloc-fast"},
		{name: "the 1M marker is stripped", model: ModelRef{ID: "claude-reloc-fast[1m]", ContextWindow: &million}, want: "claude-reloc-fast"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := desktopRowName(testCase.model)
			if testCase.want != "" {
				if got != testCase.want {
					t.Fatalf("desktopRowName() = %q, want %q", got, testCase.want)
				}
				return
			}
			if got != catalog.DesktopModelAlias(gatewayPickerID(testCase.model)) {
				t.Fatalf("desktopRowName() = %q, want the opaque alias", got)
			}
		})
	}
}

func TestDesktopAliasesNeverLookLikeAClaudeVersion(t *testing.T) {
	versioned := regexp.MustCompile(`claude-(sonnet|opus|haiku|fable|mythos)-[0-9]`)
	for suffix := 0; suffix < 500; suffix++ {
		alias := catalog.DesktopModelAlias("claude-relo-x--model-" + string(rune('a'+suffix%26)) + string(rune('a'+suffix/26)))
		if desktopRefusedVendor.MatchString(alias) || strings.Contains(alias, "abab") || versioned.MatchString(alias) {
			t.Fatalf("alias %q would be dropped or misread by Desktop", alias)
		}
	}
}

func TestDesktopModelsTierFollowsBehavesAs(t *testing.T) {
	rows := desktopModels([]ModelRef{
		{ID: "claude-relo-a--claude-sonnet-5-5", Upstream: "claude-sonnet-5-5"},
		{ID: "claude-relo-b--gpt-5.5-pro", Upstream: "gpt-5.5-pro"},
		{ID: "claude-relo-c--gpt-5.5-sol", Upstream: "gpt-5.5-sol"},
	})
	wantTiers := []string{"sonnet", "opus", "sonnet"}
	defaults := map[string]int{}
	for index, row := range rows {
		if row.AnthropicFamilyTier != wantTiers[index] {
			t.Fatalf("row %d tier = %q, want %q", index, row.AnthropicFamilyTier, wantTiers[index])
		}
		if row.IsFamilyDefault {
			defaults[row.AnthropicFamilyTier]++
		}
	}
	if !rows[0].IsFamilyDefault || rows[0].AnthropicFamilyTier != "sonnet" {
		t.Fatal("the first-listed row is no longer the sonnet default")
	}
	if defaults["opus"] != 1 || defaults["sonnet"] != 1 {
		t.Fatalf("family defaults = %v, want one per tier", defaults)
	}
}

func TestDesktopModelsCollapseAStrippedNameOntoOneRow(t *testing.T) {
	million := int64(catalog.MillionContext)
	base := ModelRef{ID: "claude-reloc-fast", Name: "Fast"}
	long := ModelRef{ID: "claude-reloc-fast", Name: "Fast", ContextWindow: &million}

	rows := desktopModels([]ModelRef{base, long})
	if len(rows) != 1 || !rows[0].Supports1m || rows[0].Prefer1m {
		t.Fatalf("base first = %#v, want one row that supports but does not prefer 1M", rows)
	}

	rows = desktopModels([]ModelRef{long, base})
	if len(rows) != 1 || !rows[0].Supports1m || !rows[0].Prefer1m {
		t.Fatalf("1M first = %#v, want the kept row to prefer 1M", rows)
	}
}
