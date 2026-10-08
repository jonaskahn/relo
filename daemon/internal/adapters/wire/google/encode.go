// Gemini request encoding.
package google

import (
	"encoding/json"
	"fmt"
	"github.com/jonaskahn/relo/internal/inference"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

const (
	roleModel = "model"
	roleUser  = "user"

	dataURLPrefix = "data:"
	dataURLBase64 = "base64"

	instructionGap = "\n\n"
)

type requestPayload struct {
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"systemInstruction,omitempty"`
	Tools             []toolGroup       `json:"tools,omitempty"`
	ToolConfig        *toolConfig       `json:"toolConfig,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
	SessionID         string            `json:"sessionId,omitempty"`
}

type envelope struct {
	Project     string         `json:"project"`
	RequestID   string         `json:"requestId,omitempty"`
	Request     requestPayload `json:"request"`
	Model       string         `json:"model"`
	UserAgent   string         `json:"userAgent,omitempty"`
	RequestType string         `json:"requestType,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	InlineData       *inlineData       `json:"inlineData,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type functionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type functionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type toolGroup struct {
	FunctionDeclarations []functionDecl `json:"functionDeclarations"`
}

type functionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type generationConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	ThinkingConfig  *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type thinkingConfig struct {
	ThinkingBudget  *int   `json:"thinkingBudget,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
	IncludeThoughts bool   `json:"includeThoughts"`
}

func newPayload(req *inference.Request, assist bool, anchor string, opts wire.CodecOpts) (*requestPayload, error) {
	var contents []content
	var err error
	if assist {
		contents, err = encodeAssist(req, anchor)
	} else {
		contents, err = encodeMessages(req.Messages)
	}
	if err != nil {
		return nil, err
	}
	return &requestPayload{
		Contents:          contents,
		SystemInstruction: systemInstruction(req.Messages),
		Tools:             encodeTools(req.Tools),
		GenerationConfig:  generationConfigFor(req, opts),
	}, nil
}

func systemInstruction(messages []inference.Message) *content {
	prompts := systemPrompts(messages)
	if len(prompts) == 0 {
		return nil
	}
	return &content{Parts: []part{{Text: strings.Join(prompts, instructionGap)}}}
}

func systemPrompts(messages []inference.Message) []string {
	var prompts []string
	for _, message := range messages {
		if message.Role == inference.RoleSystem {
			prompts = append(prompts, textOf(message.Content))
		}
	}
	return prompts
}

func encodeMessages(messages []inference.Message) ([]content, error) {
	encoded := make([]content, 0, len(messages))
	names := map[string]string{}
	for _, message := range messages {
		if message.Role == inference.RoleSystem {
			continue
		}
		rememberCalls(message, names)
		entry, err := encodeMessage(message, names)
		if err != nil {
			return nil, err
		}
		if len(entry.Parts) > 0 {
			encoded = append(encoded, entry)
		}
	}
	return encoded, nil
}

func rememberCalls(message inference.Message, names map[string]string) {
	for _, call := range message.ToolCalls {
		names[call.ID] = call.Name
	}
}

func encodeMessage(message inference.Message, names map[string]string) (content, error) {
	if message.Role == inference.RoleTool {
		return toolContent(message, names), nil
	}
	parts, err := messageParts(message)
	if err != nil {
		return content{}, err
	}
	return content{Role: contentRole(message.Role), Parts: parts}, nil
}

func contentRole(role string) string {
	if role == inference.RoleAssistant {
		return roleModel
	}
	return roleUser
}

func toolContent(message inference.Message, names map[string]string) content {
	response := &functionResponse{Name: toolName(message, names), Response: functionResult(textOf(message.Content))}
	return content{Role: roleUser, Parts: []part{{FunctionResponse: response}}}
}

func toolName(message inference.Message, names map[string]string) string {
	if message.Name != "" {
		return message.Name
	}
	if name, found := names[message.ToolCallID]; found {
		return name
	}
	return message.ToolCallID
}

func functionResult(text string) map[string]any {
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err == nil && result != nil {
		return result
	}
	return map[string]any{"result": text}
}

func messageParts(message inference.Message) ([]part, error) {
	parts := make([]part, 0, len(message.Content)+len(message.ToolCalls))
	for _, block := range message.Content {
		encoded, err := encodePart(block)
		if err != nil {
			return nil, err
		}
		if encoded != nil {
			parts = append(parts, *encoded)
		}
	}
	return append(parts, callParts(message.ToolCalls)...), nil
}

func encodePart(block inference.ContentPart) (*part, error) {
	switch block.Type {
	case inference.ContentTypeText:
		spoken := inference.SpokenText([]inference.ContentPart{block})
		if len(spoken) == 0 {
			return nil, nil
		}
		return &part{Text: spoken[0].Text}, nil
	case inference.ContentTypeImage:
		return imagePart(block.ImageURL)
	default:
		return nil, nil
	}
}

func imagePart(address string) (*part, error) {
	mimeType, data, ok := splitDataURL(address)
	if !ok {
		return nil, fmt.Errorf("%w: google takes inline image bytes, not %q", wire.ErrUnsupportedFeature, address)
	}
	return &part{InlineData: &inlineData{MimeType: mimeType, Data: data}}, nil
}

func splitDataURL(address string) (string, string, bool) {
	remainder, found := strings.CutPrefix(address, dataURLPrefix)
	if !found {
		return "", "", false
	}
	header, data, found := strings.Cut(remainder, ",")
	if !found {
		return "", "", false
	}
	mimeType, encoding, found := strings.Cut(header, ";")
	if !found || encoding != dataURLBase64 || mimeType == "" {
		return "", "", false
	}
	return mimeType, data, true
}

func callParts(calls []inference.ToolCall) []part {
	parts := make([]part, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, part{FunctionCall: &functionCall{Name: call.Name, Args: toolArguments(call.Arguments)}})
	}
	return parts
}

func toolArguments(arguments string) json.RawMessage {
	if strings.TrimSpace(arguments) == "" || !json.Valid([]byte(arguments)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(arguments)
}

func encodeTools(tools []inference.Tool) []toolGroup {
	if len(tools) == 0 {
		return nil
	}
	declarations := make([]functionDecl, 0, len(tools))
	for _, tool := range tools {
		declarations = append(declarations, functionDecl{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  SanitizeSchema(tool.Parameters),
		})
	}
	return []toolGroup{{FunctionDeclarations: declarations}}
}

func generationConfigFor(req *inference.Request, opts wire.CodecOpts) *generationConfig {
	config := &generationConfig{
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		ThinkingConfig:  thinkingFor(req.Reasoning, opts),
	}
	if config.MaxOutputTokens == 0 && config.Temperature == nil && config.ThinkingConfig == nil {
		return nil
	}
	return config
}

func thinkingFor(reasoning *inference.ReasoningConfig, opts wire.CodecOpts) *thinkingConfig {
	if reasoning == nil {
		return nil
	}
	level, off := inference.ChooseThinking(reasoning.Effort, opts.ReasoningEfforts, opts.ReasoningToggle)
	if off {
		return geminiOff(opts)
	}
	if len(opts.ReasoningEfforts) > 0 && level != "" {
		return &thinkingConfig{ThinkingLevel: level, IncludeThoughts: true}
	}
	name := level
	if name == "" {
		var ok bool
		name, ok = inference.NormalizeEffort(reasoning.Effort)
		if !ok || (opts.ReasoningEfforts != nil && !opts.ReasoningBudget) {
			return nil
		}
	}
	budget, ok := inference.EffortBudget(name)
	if !ok {
		return nil
	}
	if opts.ReasoningBudget {
		budget = inference.ClampBudget(budget, opts.ReasoningBudgetMin, opts.ReasoningBudgetMax)
	}
	return &thinkingConfig{ThinkingBudget: &budget, IncludeThoughts: true}
}

func geminiOff(opts wire.CodecOpts) *thinkingConfig {
	if opts.ReasoningBudget || opts.ReasoningEfforts == nil {
		budget := inference.ClampBudget(0, opts.ReasoningBudgetMin, opts.ReasoningBudgetMax)
		return &thinkingConfig{ThinkingBudget: &budget, IncludeThoughts: false}
	}
	return &thinkingConfig{ThinkingLevel: "minimal", IncludeThoughts: false}
}

func textOf(parts []inference.ContentPart) string {
	var builder strings.Builder
	for _, block := range parts {
		if block.Type == inference.ContentTypeText {
			builder.WriteString(block.Text)
		}
	}
	return builder.String()
}
