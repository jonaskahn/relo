// Cursor server message parsing.
package cursor

import (
	"github.com/jonaskahn/relo/internal/adapters/cursor"
)

// Server envelope fields.
const (
	serverInteraction = 1
	serverExec        = 2
	serverCheckpoint  = 3
	serverKV          = 4
	serverControl     = 5
	serverQuery       = 7
)

// Interaction update variants.
const (
	updateTextDelta       = 1
	updateToolStarted     = 2
	updateToolCompleted   = 3
	updateThinkingDelta   = 4
	updateTokenDelta      = 8
	updateTurnEnded       = 14
	updateToolDelta       = 15
	updatePartialToolCall = 7
)

// Update payload fields.
const (
	textDeltaText     = 1
	thinkingText      = 1
	tokenDeltaTokens  = 1
	toolStartedID     = 1
	toolStartedCall   = 2
	toolCompletedID   = 1
	toolCompletedCall = 2
	partialID         = 1
	partialCall       = 2
	partialArgsDelta  = 3

	turnedInput    = 1
	turnedOutput   = 2
	turnedCacheIn  = 3
	turnedCacheOut = 4

	mcpCallArgs   = 1
	mcpCallResult = 2
)

// ServerMessage is one parsed AgentServerMessage.
type ServerMessage struct {
	// Update holds an interaction-update payload, nil otherwise.
	Update []cursor.Field
	// UpdateKind is the interaction-update variant number, zero otherwise.
	UpdateKind int
	// Exec holds an exec-server-message payload, nil otherwise.
	Exec []cursor.Field
	// ExecKind is the exec variant number, zero otherwise.
	ExecKind int
	// ExecNumericID and ExecID correlate the execution.
	ExecNumericID uint32
	ExecID        string
	// KV holds a kv-server-message payload, nil otherwise.
	KV []cursor.Field
	// KVKind is the kv variant number, zero otherwise.
	KVKind int
	// KVID correlates the blob request.
	KVID uint32
	// Checkpoint reports a conversation-checkpoint update.
	Checkpoint bool
	// Control reports an exec control message, which aborts the turn.
	Control bool
	// Query reports an interaction query Relo cannot answer.
	Query bool
}

// ParseServerMessage decodes one AgentServerMessage payload.
func ParseServerMessage(payload []byte) (ServerMessage, error) {
	fields, err := cursor.Parse(payload)
	if err != nil {
		return ServerMessage{}, err
	}
	message := ServerMessage{}
	for _, field := range fields {
		if field.Wire != 2 {
			continue
		}
		nested, err := cursor.Parse(field.Raw)
		if err != nil {
			return ServerMessage{}, err
		}
		switch field.Number {
		case serverInteraction:
			message.Update, message.UpdateKind = parseUpdate(nested)
		case serverExec:
			message.Exec, message.ExecKind, message.ExecNumericID, message.ExecID = parseExec(nested), execKindOf(nested), execNumericID(nested), execStringID(nested)
		case serverKV:
			message.KV, message.KVKind, message.KVID = parseKV(nested)
		case serverCheckpoint:
			message.Checkpoint = true
		case serverControl:
			message.Control = true
		case serverQuery:
			message.Query = true
		}
	}
	return message, nil
}

func parseUpdate(fields []cursor.Field) ([]cursor.Field, int) {
	for _, field := range fields {
		if field.Wire != 2 {
			continue
		}
		return mustParse(field.Raw), field.Number
	}
	return nil, 0
}

func mustParse(raw []byte) []cursor.Field {
	fields, err := cursor.Parse(raw)
	if err != nil {
		return nil
	}
	return fields
}

func execKindOf(fields []cursor.Field) int {
	for _, field := range fields {
		if field.Wire != 2 || field.Number == execIDField || field.Number == execExecID {
			continue
		}
		return field.Number
	}
	return 0
}

func execNumericID(fields []cursor.Field) uint32 {
	return uint32(cursor.Uint(fields, execIDField))
}

func execStringID(fields []cursor.Field) string {
	return cursor.FirstString(fields, execExecID)
}

func parseExec(fields []cursor.Field) []cursor.Field {
	for _, field := range fields {
		if field.Wire != 2 || field.Number == execIDField || field.Number == execExecID {
			continue
		}
		return mustParse(field.Raw)
	}
	return nil
}

func parseKV(fields []cursor.Field) ([]cursor.Field, int, uint32) {
	var numeric uint32
	for _, field := range fields {
		if field.Number == kvIDField && field.Wire == 0 {
			numeric = uint32(field.Varint)
		}
	}
	for _, field := range fields {
		if field.Wire != 2 || (field.Number != kvGetArgs && field.Number != kvSetArgs) {
			continue
		}
		return mustParse(field.Raw), field.Number, numeric
	}
	return nil, 0, numeric
}

// MCPCall is one parsed mcpToolCall: the call arguments and wire identity.
type MCPCall struct {
	Name       string
	ToolName   string
	Provider   string
	ToolCallID string
	Args       map[string][]byte
	HasResult  bool
}

// ParseMCPCall decodes one McpArgs payload.
func ParseMCPCall(payload []byte) (MCPCall, error) {
	fields, err := cursor.Parse(payload)
	if err != nil {
		return MCPCall{}, err
	}
	call := MCPCall{
		Name:       cursor.FirstString(fields, mcpArgName),
		ToolName:   cursor.FirstString(fields, mcpArgToolName),
		Provider:   cursor.FirstString(fields, mcpArgProvider),
		ToolCallID: cursor.FirstString(fields, mcpArgCallID),
		Args:       map[string][]byte{},
	}
	argMaps, err := cursor.Nested(fields, mcpArgValues)
	if err != nil {
		return MCPCall{}, err
	}
	for _, entry := range argMaps {
		key := cursor.FirstString(entry, mapEntryKey)
		if key == "" {
			continue
		}
		for _, field := range entry {
			if field.Number == mapEntryValue && field.Wire == 2 {
				call.Args[key] = field.Raw
			}
		}
	}
	return call, nil
}

// ParseToolCall extracts the MCP call of one ToolCall payload. It reports
// the wire tool name and whether the call is an MCP call at all.
func ParseToolCall(payload []byte) (MCPCall, bool, error) {
	fields, err := cursor.Parse(payload)
	if err != nil {
		return MCPCall{}, false, err
	}
	callID := cursor.FirstString(fields, toolCallIDField)
	for _, field := range fields {
		if field.Number != toolCallMCP || field.Wire != 2 {
			continue
		}
		mcpFields, err := cursor.Parse(field.Raw)
		if err != nil {
			return MCPCall{}, false, err
		}
		for _, mcp := range mcpFields {
			if mcp.Number != mcpArgsField || mcp.Wire != 2 {
				continue
			}
			call, err := ParseMCPCall(mcp.Raw)
			if err != nil {
				return MCPCall{}, false, err
			}
			if call.ToolCallID == "" {
				call.ToolCallID = callID
			}
			return call, true, nil
		}
		return MCPCall{}, true, nil
	}
	return MCPCall{}, false, nil
}
