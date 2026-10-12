// Declared tool mapping: Cursor-visible names and canonical restoration.
package cursor

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// providerIdentifier is defined in protocol.go.

// nativeToolNames collide with Cursor's native execution channel. A
// colliding tool is declared under a prefixed wire name so the model calls
// the declared external tool instead of the daemon-side native frame.
var nativeToolNames = map[string]bool{
	"bash": true, "shell": true, "read": true, "write": true,
	"delete": true, "ls": true, "grep": true, "todo": true,
	"diagnostics": true, "glob": true, "edit": true,
}

// Tool limits keep one hostile client from exhausting the attempt.
const (
	maxTools      = 64
	maxSchemaJSON = 64 << 10
)

// ToolMap maps Cursor-visible tool names back to canonical tools.
type ToolMap struct {
	byWire map[string]inference.Tool
	names  []string
}

// BuildToolMap declares canonical tools under Cursor-visible names.
func BuildToolMap(tools []inference.Tool) (*ToolMap, error) {
	mapped := &ToolMap{byWire: map[string]inference.Tool{}}
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		wireName := WireName(tool.Name)
		if _, taken := mapped.byWire[wireName]; taken {
			continue
		}
		if len(tool.Parameters) > maxSchemaJSON {
			return nil, wire.ErrUnsupportedFeature
		}
		mapped.byWire[wireName] = tool
		mapped.names = append(mapped.names, wireName)
		if len(mapped.names) > maxTools {
			return nil, wire.ErrUnsupportedFeature
		}
	}
	return mapped, nil
}

// WireName returns the Cursor-visible name of one canonical tool.
func WireName(name string) string {
	if nativeToolNames[name] {
		return "relo_" + name
	}
	return name
}

// Canonical returns the canonical tool one wire name names.
func (m *ToolMap) Canonical(wireName string) (inference.Tool, bool) {
	tool, found := m.byWire[wireName]
	return tool, found
}

// Names returns the declared wire names in declaration order.
func (m *ToolMap) Names() []string {
	return append([]string(nil), m.names...)
}

// Defs encodes every declared tool definition.
func (m *ToolMap) Defs() [][]byte {
	defs := make([][]byte, 0, len(m.names))
	for _, wireName := range m.names {
		tool := m.byWire[wireName]
		defs = append(defs, buildToolDef(wireName, tool.Name, tool.Description, toolSchema(tool)))
	}
	return defs
}

func toolSchema(tool inference.Tool) string {
	raw := strings.TrimSpace(string(tool.Parameters))
	if raw == "" {
		return `{"type":"object"}`
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return `{"type":"object"}`
	}
	compact, err := json.Marshal(parsed)
	if err != nil {
		return `{"type":"object"}`
	}
	return string(compact)
}

// normalizeToolCallID sanitizes one tool-call id into Cursor's charset.
func normalizeToolCallID(id string) string {
	var cleaned strings.Builder
	for _, char := range id {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9', char == '-', char == '_':
			cleaned.WriteRune(char)
		default:
			cleaned.WriteString("_")
		}
	}
	if cleaned.Len() == 0 {
		return "call"
	}
	return cleaned.String()
}

func turnMessageID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "turn"
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return hex.EncodeToString(raw[:4]) + "-" + hex.EncodeToString(raw[4:6]) + "-" +
		hex.EncodeToString(raw[6:8]) + "-" + hex.EncodeToString(raw[8:10]) + "-" + hex.EncodeToString(raw[10:])
}

// buildMCPArgs encodes the McpArgs of one history tool call under its wire
// name, with arguments split into per-value JSON bytes.
func buildMCPArgs(call inference.ToolCall, tools *ToolMap) []byte {
	name := call.Name
	if tools != nil {
		if _, found := tools.byWire[WireName(call.Name)]; found {
			name = WireName(call.Name)
		}
	}
	var args cursor.Writer
	args.String(mcpArgName, name)
	for key, value := range splitArgsJSON(call.Arguments) {
		var entry cursor.Writer
		entry.String(mapEntryKey, key)
		entry.Bytes(mapEntryValue, value)
		args.Message(mcpArgValues, entry.BytesOut())
	}
	args.String(mcpArgCallID, normalizeToolCallID(call.ID))
	args.String(mcpArgProvider, providerIdentifier)
	args.String(mcpArgToolName, name)
	return args.BytesOut()
}

func splitArgsJSON(raw string) map[string][]byte {
	out := map[string][]byte{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		return out
	}
	for key, value := range object {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		out[key] = encoded
	}
	return out
}
