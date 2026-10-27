package cursor

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// imageFixture is a 1x1 PNG: known bytes the decoder must recover verbatim.
const imageFixture = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func imagePart() inference.ContentPart {
	return inference.ContentPart{
		Type:     inference.ContentTypeImage,
		ImageURL: "data:image/png;base64," + imageFixture,
	}
}

func imageBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(imageFixture)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return data
}

// TestSplitDataURL covers the address forms the encoder accepts and the one
// a remote image arrives as, which has no encoding on this wire.
func TestSplitDataURL(t *testing.T) {
	mimeType, data, ok := splitDataURL("data:image/png;base64," + imageFixture)
	if !ok || mimeType != "image/png" || !bytes.Equal(data, imageBytes(t)) {
		t.Fatalf("splitDataURL() = %q %d bytes %v, want the decoded fixture", mimeType, len(data), ok)
	}
	for name, address := range map[string]string{
		"remote":       "https://example.com/a.png",
		"not base64":   "data:image/png,raw-bytes",
		"no mime":      "data:;base64," + imageFixture,
		"bad payload":  "data:image/png;base64,not base64!",
		"no separator": "data:image/png;base64",
		"empty":        "",
	} {
		if _, _, ok := splitDataURL(address); ok {
			t.Fatalf("splitDataURL(%s) accepted %q", name, address)
		}
	}
}

// TestValidateAcceptsInlineImages covers the narrowed refusal: an inline
// image passes, a remote address stays a refusal.
func TestValidateAcceptsInlineImages(t *testing.T) {
	inline := &inference.Request{Model: "sonnet", Messages: []inference.Message{
		{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "look"}, imagePart()}},
	}}
	if err := Validate(inline, validSelection()); err != nil {
		t.Fatalf("Validate() error = %v, want the inline image accepted", err)
	}
	remote := &inference.Request{Model: "sonnet", Messages: []inference.Message{
		{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeImage, ImageURL: "https://example.com/a.png"},
		}},
	}}
	err := Validate(remote, validSelection())
	if err == nil {
		t.Fatal("Validate() accepted a remote image")
	}
	if !isUnsupportedFeature(err) {
		t.Fatalf("Validate() error = %v, want ErrUnsupportedFeature", err)
	}
}

func isUnsupportedFeature(err error) bool {
	for err != nil {
		if err == wire.ErrUnsupportedFeature {
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}

// TestUserActionCarriesSelectedImage covers the trailing user message: the
// image rides UserMessageAction → userMessage → selectedContext →
// selectedImages, with the MIME type and the decoded bytes.
func TestUserActionCarriesSelectedImage(t *testing.T) {
	message := inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{
		{Type: inference.ContentTypeText, Text: "what is this"}, imagePart(),
	}}
	action, err := buildUserAction(message)
	if err != nil {
		t.Fatalf("buildUserAction() error = %v", err)
	}
	fields, err := adapterscursor.Parse(action)
	if err != nil {
		t.Fatalf("parse action: %v", err)
	}
	userAction := nestedOne(t, fields, actionUserMessage)
	user := nestedOne(t, userAction, userActionMessage)
	if text := adapterscursor.FirstString(user, userMessageText); text != "what is this" {
		t.Fatalf("text = %q", text)
	}
	context := nestedOne(t, user, userSelectedCtx)
	images := nestedAll(t, context, selectedCtxImages)
	if len(images) != 1 {
		t.Fatalf("selectedImages = %d, want one", len(images))
	}
	if mime := adapterscursor.FirstString(images[0], selectedImageMimeType); mime != "image/png" {
		t.Fatalf("mimeType = %q", mime)
	}
	// data is a oneof case: the bytes must be present, not dropped by the
	// empty-value skip Writer.Bytes applies.
	if got := rawField(images[0], selectedImageData); !bytes.Equal(got, imageBytes(t)) {
		t.Fatalf("data = %d bytes, want %d decoded", len(got), len(imageBytes(t)))
	}
	if uuid := adapterscursor.FirstString(images[0], selectedImageUUID); uuid == "" {
		t.Fatal("selected image carries no uuid")
	}
}

func nestedOne(t *testing.T, fields []adapterscursor.Field, number int) []adapterscursor.Field {
	t.Helper()
	nested, err := adapterscursor.Nested(fields, number)
	if err != nil || len(nested) != 1 {
		t.Fatalf("field %d = %d nested (%v), want one", number, len(nested), err)
	}
	return nested[0]
}

func nestedAll(t *testing.T, fields []adapterscursor.Field, number int) [][]adapterscursor.Field {
	t.Helper()
	nested, err := adapterscursor.Nested(fields, number)
	if err != nil {
		t.Fatalf("field %d: %v", number, err)
	}
	return nested
}

func rawField(fields []adapterscursor.Field, number int) []byte {
	for _, field := range fields {
		if field.Number == number && field.Wire == 2 {
			return field.Raw
		}
	}
	return nil
}

// TestHistoryTurnCarriesSelectedImage covers a prior user turn: its image
// survives into the turn's user-message blob.
func TestHistoryTurnCarriesSelectedImage(t *testing.T) {
	request := &inference.Request{Model: "sonnet", Messages: []inference.Message{
		{Role: inference.RoleUser, Content: []inference.ContentPart{
			{Type: inference.ContentTypeText, Text: "look at this"}, imagePart(),
		}},
		{Role: inference.RoleAssistant, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "ok"}}},
		{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "and now"}}},
	}}
	tools, err := BuildToolMap(nil)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	history, _, err := BuildHistory(request, tools)
	if err != nil {
		t.Fatalf("BuildHistory() error = %v", err)
	}
	state, err := adapterscursor.Parse(history.State)
	if err != nil {
		t.Fatalf("parse state: %v", err)
	}
	turnIDs := rawFields(state, stateTurns)
	if len(turnIDs) != 1 {
		t.Fatalf("turns = %d, want the first user turn", len(turnIDs))
	}
	// The state field holds a blob id; the stored blob wraps the turn, whose
	// first field is the user message.
	turnBlob, found := history.Store.Get(turnIDs[0])
	if !found {
		t.Fatal("turn blob missing")
	}
	turn, err := adapterscursor.Parse(turnBlob)
	if err != nil {
		t.Fatalf("parse turn: %v", err)
	}
	turn = nestedOne(t, turn, 1)
	var blob []byte
	for _, field := range turn {
		if field.Number == turnUserMessage && field.Wire == 2 {
			blob = field.Raw
		}
	}
	userBlob, found := history.Store.Get(blob)
	if !found {
		t.Fatal("turn user-message blob missing")
	}
	user, err := adapterscursor.Parse(userBlob)
	if err != nil {
		t.Fatalf("parse user blob: %v", err)
	}
	images := nestedAll(t, nestedOne(t, user, userSelectedCtx), selectedCtxImages)
	if len(images) != 1 {
		t.Fatalf("turn selectedImages = %d, want one", len(images))
	}
	if !bytes.Equal(rawField(images[0], selectedImageData), imageBytes(t)) {
		t.Fatal("turn image bytes do not round-trip")
	}
}

// rawFields returns the raw payloads of one length-delimited field. A state
// turn field holds a blob id, which is not itself a message.
func rawFields(fields []adapterscursor.Field, number int) [][]byte {
	var out [][]byte
	for _, field := range fields {
		if field.Number == number && field.Wire == 2 {
			out = append(out, field.Raw)
		}
	}
	return out
}

// TestRootPromptCarriesImagePart covers the JSON root prompt: a prior user
// turn yields a text part and an image part carrying the data URL.
func TestRootPromptCarriesImagePart(t *testing.T) {
	message := inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{
		{Type: inference.ContentTypeText, Text: "look"}, imagePart(),
	}}
	store := NewStore()
	builder := &rootPromptBuilder{store: store, calls: map[string]string{}, emitted: map[string]bool{}}
	builder.replayUser(message)
	if len(builder.ids) != 1 {
		t.Fatalf("root prompts = %d, want one", len(builder.ids))
	}
	blob, found := store.Get(builder.ids[0])
	if !found {
		t.Fatal("root prompt blob missing")
	}
	var entry struct {
		Role    string `json:"role"`
		Content []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Image     string `json:"image"`
			MediaType string `json:"mediaType"`
		} `json:"content"`
	}
	if err := json.Unmarshal(blob, &entry); err != nil {
		t.Fatalf("unmarshal root prompt: %v", err)
	}
	if len(entry.Content) != 2 {
		t.Fatalf("content = %+v, want a text part and an image part", entry.Content)
	}
	if entry.Content[0].Type != "text" || entry.Content[0].Text != "look" {
		t.Fatalf("text part = %+v", entry.Content[0])
	}
	image := entry.Content[1]
	if image.Type != "image" || image.MediaType != "image/png" || image.Image != imagePart().ImageURL {
		t.Fatalf("image part = %+v", image)
	}
}

// TestHistoryResultCarriesImage covers a tool result: an image on a tool
// message becomes a McpToolResultContentItem image case beside the text.
func TestHistoryResultCarriesImage(t *testing.T) {
	result := inference.Message{ToolCallID: "call-1", Content: []inference.ContentPart{
		{Type: inference.ContentTypeText, Text: "screenshot"}, imagePart(),
	}}
	fields, err := adapterscursor.Parse(buildHistoryResult(result))
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	items := nestedAll(t, nestedOne(t, fields, 1), 1)
	if len(items) != 2 {
		t.Fatalf("items = %d, want a text item and an image item", len(items))
	}
	text := nestedAll(t, items[0], mcpTextCase)
	if len(text) != 1 || adapterscursor.FirstString(text[0], mcpContentText) != "screenshot" {
		t.Fatalf("text item = %+v", items[0])
	}
	images := nestedAll(t, items[1], mcpImageCase)
	if len(images) != 1 {
		t.Fatalf("image item = %+v", items[1])
	}
	if !bytes.Equal(rawField(images[0], mcpImageData), imageBytes(t)) {
		t.Fatal("tool-result image bytes do not round-trip")
	}
	if mime := adapterscursor.FirstString(images[0], mcpImageMime); mime != "image/png" {
		t.Fatalf("mimeType = %q", mime)
	}
}

// TestHistoryResultTextOnlyUnchanged guards the no-change path: a text-only
// result still encodes exactly as it did before images were representable.
func TestHistoryResultTextOnlyUnchanged(t *testing.T) {
	result := inference.Message{ToolCallID: "call-1", Content: []inference.ContentPart{
		{Type: inference.ContentTypeText, Text: "contents"},
	}}
	var content adapterscursor.Writer
	content.String(mcpContentText, "contents")
	var item adapterscursor.Writer
	item.Message(mcpTextCase, content.BytesOut())
	var success adapterscursor.Writer
	success.Message(1, item.BytesOut())
	var want adapterscursor.Writer
	want.Message(1, success.BytesOut())
	if got := buildHistoryResult(result); !bytes.Equal(got, want.BytesOut()) {
		t.Fatalf("text-only result = %x, want %x", got, want.BytesOut())
	}
}

func remoteImagePart() inference.ContentPart {
	return inference.ContentPart{Type: inference.ContentTypeImage, ImageURL: "https://example.com/a.png"}
}

// TestBuildUserMessageRefusesRemoteImage covers the encoder path a remote
// address takes: it has no encoding on this wire, so the message is refused
// rather than silently dropped.
func TestBuildUserMessageRefusesRemoteImage(t *testing.T) {
	_, err := buildUserMessage(inference.Message{Role: inference.RoleUser, Content: []inference.ContentPart{remoteImagePart()}})
	if !isUnsupportedFeature(err) {
		t.Fatalf("buildUserMessage() error = %v, want ErrUnsupportedFeature", err)
	}
}

// TestBuildHistoryRefusesRemoteImageInActiveTurn covers the trailing-action
// refusal: the active user message carries a remote image, so buildAction
// propagates the encoder error out of BuildHistory.
func TestBuildHistoryRefusesRemoteImageInActiveTurn(t *testing.T) {
	request := &inference.Request{Model: "sonnet", Messages: []inference.Message{
		{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hi"}}},
		{Role: inference.RoleUser, Content: []inference.ContentPart{remoteImagePart()}},
	}}
	tools, err := BuildToolMap(nil)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	if _, _, err := BuildHistory(request, tools); !isUnsupportedFeature(err) {
		t.Fatalf("BuildHistory() error = %v, want the trailing remote image refused", err)
	}
}

// TestBuildHistoryRefusesRemoteImageInPriorTurn covers the replayed-turn
// path: a prior user turn with a remote image is skipped in the root prompt
// and refused when its turn is stored, which BuildHistory propagates.
func TestBuildHistoryRefusesRemoteImageInPriorTurn(t *testing.T) {
	request := &inference.Request{Model: "sonnet", Messages: []inference.Message{
		{Role: inference.RoleUser, Content: []inference.ContentPart{remoteImagePart()}},
		{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "and now"}}},
	}}
	tools, err := BuildToolMap(nil)
	if err != nil {
		t.Fatalf("BuildToolMap() error = %v", err)
	}
	if _, _, err := BuildHistory(request, tools); !isUnsupportedFeature(err) {
		t.Fatalf("BuildHistory() error = %v, want the replayed remote image refused", err)
	}
}

// TestHistoryResultSkipsRemoteImage covers the tool-result path: a remote
// image has no encoding, so it is skipped and the text item still encodes.
func TestHistoryResultSkipsRemoteImage(t *testing.T) {
	result := inference.Message{ToolCallID: "call-1", Content: []inference.ContentPart{
		{Type: inference.ContentTypeText, Text: "screenshot"},
		remoteImagePart(),
	}}
	fields, err := adapterscursor.Parse(buildHistoryResult(result))
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	items := nestedAll(t, nestedOne(t, fields, 1), 1)
	if len(items) != 1 {
		t.Fatalf("items = %d, want only the text item", len(items))
	}
	if len(nestedAll(t, items[0], mcpTextCase)) != 1 {
		t.Fatalf("text item = %+v, want the text content", items[0])
	}
}
