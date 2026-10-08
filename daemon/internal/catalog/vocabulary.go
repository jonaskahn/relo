// Package catalog holds the vocabulary and rules of the model catalog:
// what a connection, a model, and a route are, and how one resolves.
package catalog

import "errors"

// ErrUnsupportedFormat reports an endpoint whose protocol Relo implements no
// codec for, which an operator sees before a request leaves.
var ErrUnsupportedFormat = errors.New("no codec implements this API format")

// APIFormat names the upstream protocol one endpoint speaks, which is what
// selects the codec that builds and reads its requests.
type APIFormat string

// Wire formats name the upstream shapes the relay translates: one per
// vendor protocol the daemon serves.
const (
	// FormatUnsupported marks a provider or model Relo cannot talk to. It
	// carries no codec, so a surface reports it instead of sending.
	FormatUnsupported     APIFormat = ""
	FormatOpenAIChat      APIFormat = "openai-chat"
	FormatOpenAIResp      APIFormat = "openai-responses"
	FormatAnthropic       APIFormat = "anthropic"
	FormatVertexAnthropic APIFormat = "vertex-anthropic"
	FormatGemini          APIFormat = "gemini"
	FormatVertex          APIFormat = "vertex"
	FormatCloudCode       APIFormat = "cloud-code-assist"
	FormatBedrockConverse APIFormat = "bedrock-converse"
	FormatKiro            APIFormat = "kiro"
)

// KeyHeader names where a credential key is placed in request headers.
type KeyHeader string

// Key headers name where a credential travels: the bearer token, the
// vendor header, or no header where the vendor needs none.
const (
	KeyHeaderBearer      KeyHeader = "bearer"
	KeyHeaderXApiKey     KeyHeader = "x-api-key"
	KeyHeaderXGoogApiKey KeyHeader = "x-goog-api-key"
	KeyHeaderApiKey      KeyHeader = "api-key"
	KeyHeaderNone        KeyHeader = "none"
)

// ModelsFormat names the listing dialect a provider publishes its models
// in. None means Relo never asks, so a provider's models stay as declared.
type ModelsFormat string

// Listing dialects name the model-list shape each vendor publishes, so a
// connection reads the roster its provider actually serves.
const (
	ModelsNone        ModelsFormat = "none"
	ModelsOpenAI      ModelsFormat = "openai"
	ModelsAnthropic   ModelsFormat = "anthropic"
	ModelsGemini      ModelsFormat = "gemini"
	ModelsAntigravity ModelsFormat = "antigravity"
	ModelsBedrock     ModelsFormat = "bedrock"
	ModelsKiro        ModelsFormat = "kiro"
	// ModelsCodex reads the ChatGPT Codex roster, which the vendor filters
	// by the client version the caller declares. Only the Codex template
	// uses it, so it stays out of ModelsFormats.
	ModelsCodex ModelsFormat = "codex"
)

// Auth names how a provider proves who is calling, which decides whether an
// operator stores a credential for it at all.
type Auth string

// Credential kinds name how an account proves itself: sign-in, API key,
// cloud keys, or nothing where the vendor needs nothing.
const (
	AuthOAuth  Auth = "oauth"
	AuthAPIKey Auth = "api_key"
	AuthAWS    Auth = "aws"
	AuthGCP    Auth = "gcp"
	AuthNone   Auth = "none"
)

// Origin names where a catalog row came from, which decides who may write
// it: a catalog template, a sign-in flow, or custom.
type Origin string

// Connection origins name where a connection came from: a curated template,
// a sign-in flow, or the operator's own hand.
const (
	OriginTemplate Origin = "template"
	OriginSignIn   Origin = "signin"
	OriginCustom   Origin = "custom"
)

// Category names what a model is for, which is what a model picker filters
// on and what tells an image model from a chat model of the same provider.
type Category string

// Model categories name what a model is for, which is what routing reads
// before offering it to a request that needs tools, vision, or reasoning.
const (
	CategoryChat      Category = "chat"
	CategoryReasoning Category = "reasoning"
	CategoryVision    Category = "vision"
	CategoryImage     Category = "image"
	CategoryAudio     Category = "audio"
	CategoryVideo     Category = "video"
	CategoryEmbedding Category = "embedding"
)

// APIFormats lists every format an operator may choose when adding a
// provider by hand.
func APIFormats() []APIFormat {
	return []APIFormat{
		FormatOpenAIChat,
		FormatOpenAIResp,
		FormatAnthropic,
		FormatVertexAnthropic,
		FormatGemini,
		FormatVertex,
		FormatBedrockConverse,
		FormatKiro,
	}
}

// ModelsFormats lists every listing dialect an operator may choose when
// adding a provider by hand.
func ModelsFormats() []ModelsFormat {
	return []ModelsFormat{
		ModelsNone,
		ModelsOpenAI,
		ModelsAnthropic,
		ModelsGemini,
		ModelsBedrock,
		ModelsKiro,
	}
}

// KeyHeaders lists every header style for credentials.
func KeyHeaders() []KeyHeader {
	return []KeyHeader{
		KeyHeaderBearer,
		KeyHeaderXApiKey,
		KeyHeaderXGoogApiKey,
		KeyHeaderApiKey,
		KeyHeaderNone,
	}
}
