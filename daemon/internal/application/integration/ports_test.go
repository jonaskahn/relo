package integration

import (
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/inference"
)

type testPaths struct {
	PathResolver
	home          string
	anthropicBase string
}

func (p testPaths) Home() string {
	return p.home
}

func (p testPaths) BaseURL(protocol string) string {
	if protocol == inference.ProtocolAnthropic {
		return p.anthropicBase
	}
	return ""
}

func (p testPaths) ClaudeConfigDir() string {
	return p.home
}

type testFiles struct {
	ClientFiles
}

func (testFiles) ModelsDigest(models []ModelRef) string {
	return codingclients.ModelsDigest(clientRefs(models))
}

func (testFiles) WriteGatewayCache(configDir, baseURL string, models []ModelRef) error {
	return codingclients.WriteGatewayCache(configDir, baseURL, clientRefs(models))
}

func (testFiles) MergeKind(kind, existing, baseURL string, models []ModelRef) (string, error) {
	return codingclients.MergeKind(kind, existing, baseURL, clientRefs(models))
}

func clientRefs(models []ModelRef) []codingclients.ModelRef {
	converted := make([]codingclients.ModelRef, 0, len(models))
	for _, ref := range models {
		converted = append(converted, codingclients.ModelRef{
			ID: ref.ID, Name: ref.Name, ContextWindow: ref.ContextWindow,
			MaxOutput: ref.MaxOutput, Tools: ref.Tools, Reasoning: ref.Reasoning,
			Vision: ref.Vision, Connection: ref.Connection, Upstream: ref.Upstream,
			ChatGPT: ref.ChatGPT, ReasoningEfforts: ref.ReasoningEfforts,
		})
	}
	return converted
}
