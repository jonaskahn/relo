// Anthropic request encoding.
package anthropic

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

const (
	defaultMaxTokens = 4096
	// replyHeadroom is the output space a thinking request keeps for its
	// visible reply, beyond the effort's thinking budget.
	replyHeadroom  = 4096
	instructionGap = "\n\n"
	continueText   = "(continue)"

	blockText       = "text"
	blockThinking   = "thinking"
	blockToolUse    = "tool_use"
	blockToolResult = "tool_result"
	blockImage      = "image"

	sourceURL    = "url"
	sourceBase64 = "base64"

	// claudeCodeIdentity is the first system block a Claude subscription
	// token requires. Without it the Messages API answers 429 rate_limit_error
	// with the body {"type":"error","error":{"type":"rate_limit_error","message":"Error"}}.
	claudeCodeIdentity = "You are Claude Code, Anthropic's official CLI for Claude."
)

var thinkingBudgets = map[string]int{
	"minimal": 1024,
	"low":     4096,
	"medium":  8192,
	"high":    16384,
	"xhigh":   24576,
	"max":     32000,
}

var adaptiveFamilies = map[string][2]int{
	"sonnet": {5, 0},
	"opus":   {4, 7},
	"fable":  {0, 0},
}

var claudeModelID = regexp.MustCompile(`(?i)(?:^|/)claude-([a-z]+)-(\d+)(?:[.-](\d{1,2}))?(?:\D|$)`)

var builtinTools = map[string]bool{
	"web_search":     true,
	"code_execution": true,
	"text_editor":    true,
	"computer":       true,
}

type requestPayload struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens"`
	System       json.RawMessage `json:"system,omitempty"`
	Messages     []wireMessage   `json:"messages"`
	Tools        []wireTool      `json:"tools,omitempty"`
	ToolChoice   *toolChoiceDef  `json:"tool_choice,omitempty"`
	Stream       bool            `json:"stream,omitempty"`
	Temperature  *float64        `json:"temperature,omitempty"`
	Thinking     *thinkingDef    `json:"thinking,omitempty"`
	OutputConfig *outputConfig   `json:"output_config,omitempty"`
}

type toolChoiceDef struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type outputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

type wireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  *string         `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Source    *imageSource    `json:"source,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Result    string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`
	URL       string `json:"url,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type thinkingDef struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

func newPayload(req *inference.Request, oauth bool, opts wire.CodecOpts) requestPayload {
	payload := requestPayload{
		Model:       req.Model,
		MaxTokens:   tokenLimit(req.MaxTokens),
		System:      systemField(systemPrompt(req.Messages), oauth, firstUserText(req.Messages)),
		Messages:    encodeMessages(req.Messages, oauth),
		Tools:       encodeTools(req.Tools, oauth),
		ToolChoice:  encodeToolChoice(req.ToolChoice),
		Stream:      req.Stream,
		Temperature: req.Temperature,
	}
	applyThinking(&payload, req, opts)
	return payload
}

// encodeToolChoice renders the canonical choice the way the Messages wire names
// it. A client that said nothing about it gets no field, which this wire
// already treats as its own default.
func encodeToolChoice(choice *inference.ToolChoice) *toolChoiceDef {
	if choice == nil {
		return nil
	}
	switch choice.Mode {
	case inference.ToolChoiceAuto:
		return &toolChoiceDef{Type: choiceAuto}
	case inference.ToolChoiceNone:
		return &toolChoiceDef{Type: choiceNone}
	case inference.ToolChoiceRequired:
		return &toolChoiceDef{Type: choiceAny}
	case inference.ToolChoiceTool:
		if choice.Name == "" {
			return nil
		}
		return &toolChoiceDef{Type: choiceToolType, Name: choice.Name}
	default:
		return nil
	}
}

func tokenLimit(maxTokens int) int {
	if maxTokens <= 0 {
		return defaultMaxTokens
	}
	return maxTokens
}

func systemPrompt(messages []inference.Message) string {
	var prompts []string
	for _, message := range messages {
		if message.Role == inference.RoleSystem {
			prompts = append(prompts, textOf(message.Content))
		}
	}
	return strings.Join(prompts, instructionGap)
}

func systemField(prompt string, oauth bool, firstUser string) json.RawMessage {
	if oauth {
		encoded, _ := json.Marshal(oauthSystem(prompt, firstUser))
		return encoded
	}
	if prompt == "" {
		return nil
	}
	encoded, _ := json.Marshal(prompt)
	return encoded
}

func oauthSystem(prompt, firstUser string) []wireBlock {
	lead := billingHeader(firstUser)
	if strings.HasPrefix(strings.TrimSpace(prompt), claudeCodeIdentity) {
		return []wireBlock{{Type: blockText, Text: lead + prompt}}
	}
	blocks := []wireBlock{{Type: blockText, Text: lead + claudeCodeIdentity}}
	if prompt != "" {
		blocks = append(blocks, wireBlock{Type: blockText, Text: prompt})
	}
	return blocks
}

func encodeMessages(messages []inference.Message, oauth bool) []wireMessage {
	var encoded []wireMessage
	var results []wireBlock
	for _, message := range messages {
		if message.Role == inference.RoleSystem {
			continue
		}
		if message.Role == inference.RoleTool {
			results = append(results, toolResultBlock(message))
			continue
		}
		encoded = append(encoded, flushResults(&results)...)
		if blocks := messageBlocks(message, oauth); len(blocks) > 0 {
			encoded = append(encoded, wireMessage{Role: messageRole(message.Role), Content: blocks})
		}
	}
	encoded = append(encoded, flushResults(&results)...)
	return continueTurn(encoded)
}

func continueTurn(encoded []wireMessage) []wireMessage {
	if len(encoded) == 0 || encoded[len(encoded)-1].Role != inference.RoleAssistant {
		return encoded
	}
	return append(encoded, wireMessage{
		Role:    inference.RoleUser,
		Content: []wireBlock{{Type: blockText, Text: continueText}},
	})
}

func flushResults(results *[]wireBlock) []wireMessage {
	if len(*results) == 0 {
		return nil
	}
	message := wireMessage{Role: inference.RoleUser, Content: *results}
	*results = nil
	return []wireMessage{message}
}

func messageRole(role string) string {
	if role == inference.RoleAssistant {
		return inference.RoleAssistant
	}
	return inference.RoleUser
}

func messageBlocks(message inference.Message, oauth bool) []wireBlock {
	blocks := make([]wireBlock, 0, len(message.Content)+len(message.ToolCalls))
	for _, part := range message.Content {
		if block, ok := contentBlock(part); ok {
			blocks = append(blocks, block)
		}
	}
	return append(blocks, toolUseBlocks(message.ToolCalls, oauth)...)
}

func contentBlock(part inference.ContentPart) (wireBlock, bool) {
	switch part.Type {
	case inference.ContentTypeText:
		spoken := inference.SpokenText([]inference.ContentPart{part})
		if len(spoken) == 0 {
			return wireBlock{}, false
		}
		return wireBlock{Type: blockText, Text: spoken[0].Text}, true
	case inference.ContentTypeImage:
		return wireBlock{Type: blockImage, Source: &imageSource{Type: sourceURL, URL: part.ImageURL}}, true
	case inference.ContentTypeThinking:
		// The Messages API rejects a thinking block without a signature, and
		// only the family that produced one can mint it. A client that carries
		// reasoning as plain text (OpenAI chat, OpenAI responses) has none, so
		// the part is dropped rather than replayed.
		if part.Signature == "" {
			return wireBlock{}, false
		}
		return wireBlock{Type: blockThinking, Thinking: &part.Text, Signature: part.Signature}, true
	default:
		return wireBlock{}, false
	}
}

func toolUseBlocks(calls []inference.ToolCall, oauth bool) []wireBlock {
	blocks := make([]wireBlock, 0, len(calls))
	for _, call := range calls {
		blocks = append(blocks, wireBlock{
			Type:  blockToolUse,
			ID:    call.ID,
			Name:  namedForUpstream(call.Name, oauth),
			Input: toolInput(call.Arguments),
		})
	}
	return blocks
}

func toolInput(arguments string) json.RawMessage {
	if strings.TrimSpace(arguments) == "" {
		return json.RawMessage("{}")
	}
	if !json.Valid([]byte(arguments)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(arguments)
}

func toolResultBlock(message inference.Message) wireBlock {
	return wireBlock{Type: blockToolResult, ToolUseID: message.ToolCallID, Result: textOf(message.Content)}
}

func encodeTools(tools []inference.Tool, oauth bool) []wireTool {
	encoded := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		encoded = append(encoded, wireTool{
			Name:        namedForUpstream(tool.Name, oauth),
			Description: tool.Description,
			InputSchema: tool.Parameters,
		})
	}
	return encoded
}

func namedForUpstream(name string, oauth bool) string {
	if !oauth || builtinTools[name] || strings.HasPrefix(name, customToolPrefix) {
		return name
	}
	return customToolPrefix + name
}

func applyThinking(payload *requestPayload, req *inference.Request, opts wire.CodecOpts) {
	if req.Reasoning == nil {
		return
	}
	level, off := inference.ChooseThinking(req.Reasoning.Effort, opts.ReasoningEfforts, opts.ReasoningToggle)
	if off {
		payload.Thinking = &thinkingDef{Type: "disabled"}
		return
	}
	if level == "" {
		return
	}
	if level == "none" && len(opts.ReasoningEfforts) == 0 {
		payload.Thinking = &thinkingDef{Type: "disabled"}
		return
	}
	applyThinkingLevel(payload, req, opts, level)
}

func applyThinkingLevel(payload *requestPayload, req *inference.Request, opts wire.CodecOpts, level string) {
	adaptive := adaptiveThinking(req.Model)
	usesEffort := len(opts.ReasoningEfforts) > 0 || adaptive
	if usesEffort && level == "minimal" {
		level = "low"
	}
	if req.MaxTokens <= 0 {
		level, payload.MaxTokens = omittedLimit(level, opts.MaxOutput)
	}
	if usesEffort {
		payload.OutputConfig = &outputConfig{Effort: level}
		if adaptive {
			payload.Thinking = &thinkingDef{Type: "adaptive"}
		}
		return
	}
	budget := inference.ClampBudget(budgetFor(level, payload.MaxTokens), opts.ReasoningBudgetMin, opts.ReasoningBudgetMax)
	payload.Thinking = &thinkingDef{Type: "enabled", BudgetTokens: budget}
}

func omittedLimit(level string, maxOutput *int64) (string, int) {
	limit := effortLimit(level, maxOutput)
	for !limitFits(level, limit) {
		lower, found := inference.LowerEffort(level)
		if !found {
			break
		}
		level = lower
	}
	return level, limit
}

func effortLimit(level string, maxOutput *int64) int {
	budget, _ := inference.EffortBudget(level)
	limit := budget + replyHeadroom
	if maxOutput != nil && *maxOutput > 0 && int(*maxOutput) < limit {
		return int(*maxOutput)
	}
	return limit
}

func limitFits(level string, limit int) bool {
	budget, _ := inference.EffortBudget(level)
	return budget+replyHeadroom <= limit
}

func adaptiveThinking(modelID string) bool {
	match := claudeModelID.FindStringSubmatch(modelID)
	if match == nil {
		return false
	}
	minimum, found := adaptiveFamilies[strings.ToLower(match[1])]
	if !found {
		return false
	}
	major, _ := strconv.Atoi(match[2])
	minor := 0
	if match[3] != "" {
		minor, _ = strconv.Atoi(match[3])
	}
	return major > minimum[0] || (major == minimum[0] && minor >= minimum[1])
}

func budgetFor(effort string, maxTokens int) int {
	budget := thinkingBudgets[effort]
	if maxTokens > 0 && budget >= maxTokens {
		return maxTokens / 2
	}
	return budget
}

func textOf(parts []inference.ContentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == inference.ContentTypeText {
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

type vertexRequestPayload struct {
	AnthropicVersion string        `json:"anthropic_version"`
	MaxTokens        int           `json:"max_tokens"`
	System           string        `json:"system,omitempty"`
	Messages         []wireMessage `json:"messages"`
	Tools            []wireTool    `json:"tools,omitempty"`
	Stream           bool          `json:"stream,omitempty"`
	Temperature      *float64      `json:"temperature,omitempty"`
	Thinking         *thinkingDef  `json:"thinking,omitempty"`
	OutputConfig     *outputConfig `json:"output_config,omitempty"`
}

func newVertexPayload(req *inference.Request, oauth bool, opts wire.CodecOpts) vertexRequestPayload {
	p := newPayload(req, oauth, opts)
	return vertexRequestPayload{
		AnthropicVersion: "vertex-2023-10-16",
		MaxTokens:        p.MaxTokens,
		System:           systemPrompt(req.Messages),
		Messages:         p.Messages,
		Tools:            p.Tools,
		Stream:           p.Stream,
		Temperature:      p.Temperature,
		Thinking:         p.Thinking,
		OutputConfig:     p.OutputConfig,
	}
}
