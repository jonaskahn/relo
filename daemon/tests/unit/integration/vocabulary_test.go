package integration_test

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/application/integration"
)

// TestClientVocabularyMatchesTheAdapter pins the identifiers the workflows
// repeat so the application never imports the adapter: a rename on either
// side has to be deliberate on both.
func TestClientVocabularyMatchesTheAdapter(t *testing.T) {
	for _, pair := range [][2]string{
		{integration.ClientOpenCode, codingclients.ClientOpenCode},
		{integration.ClientHermes, codingclients.ClientHermes},
		{integration.ClientOpenClaw, codingclients.ClientOpenClaw},
		{integration.FileOpenCode, codingclients.FileOpenCode},
		{integration.FileHermes, codingclients.FileHermes},
		{integration.FileOpenClaw, codingclients.FileOpenClaw},
		{integration.FileAside, codingclients.FileAside},
		{integration.FilePi, codingclients.FilePi},
		{integration.FileOMP, codingclients.FileOMP},
		{integration.ClaudeGatewayDiscoveryEnv, codingclients.ClaudeGatewayDiscoveryEnv},
		{string(integration.ClaudeAuthLogin), string(codingclients.ClaudeAuthLogin)},
		{string(integration.ClaudeAuthProxy), string(codingclients.ClaudeAuthProxy)},
	} {
		if pair[0] != pair[1] {
			t.Errorf("vocabulary = %q, adapter = %q, want the same identifier", pair[0], pair[1])
		}
	}
}
