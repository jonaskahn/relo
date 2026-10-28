// Cursor duplex runs: conversing until the turn ends.
package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// Defaults for a turn's liveness: the reference client pings every five
// seconds, and a stream silent past the idle bound is dead, not thinking.
const (
	defaultHeartbeatEvery = 5 * time.Second
	defaultIdleTimeout    = 120 * time.Second
)

// Run errors name why a Cursor turn failed.
var (
	ErrUnauthorized    = errors.New("cursor refused the credentials")
	ErrUnsupportedExec = errors.New("cursor asked for an execution Relo cannot answer")
	ErrNoTurnEnd       = errors.New("cursor ended the stream without finishing the turn")
	ErrIdle            = errors.New("cursor sent nothing within the idle limit")
	ErrInteraction     = errors.New("cursor asked for an interaction Relo cannot answer")
)

// Runner runs one Cursor agent turn over a duplex HTTP stream.
type Runner struct {
	Client    *http.Client
	BaseURL   string
	Token     string
	Request   *inference.Request
	Selection *wire.CursorRequest
	Emit      func([]inference.Event)
	Record    func(frame []byte)
	// RefusesMaxOutputTokens drops the request's output ceiling instead of
	// refusing it, for an upstream whose dialect states no ceiling to send.
	RefusesMaxOutputTokens bool
	ConnectError           func(status int, body []byte) *inference.ErrorInfo
	// Handoff reports that this turn handed a native execution to the calling
	// client, which means the upstream conversation holds a pending call the
	// next request cannot continue.
	Handoff func()
	// HeartbeatEvery and IdleTimeout default to the values above when zero.
	HeartbeatEvery time.Duration
	IdleTimeout    time.Duration
}

// Run converses until the turn ends and returns every canonical event.
func (r *Runner) Run(ctx context.Context) ([]inference.Event, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	turn, err := r.beginTurn(ctx, cancel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = turn.response.Body.Close() }()
	runErr := turn.converse(ctx)
	if turn.idled.Load() && runErr != nil {
		runErr = ErrIdle
	}
	if turn.conversation.externalHandoff {
		if r.Handoff != nil {
			r.Handoff()
		}
		// A handoff is not a failure and does not depend on how the server
		// ended the run, so its stream is not read again.
		return turn.conversation.finish(runErr, nil)
	}
	return turn.conversation.finish(runErr, streamRefusal(turn.reader, turn.response))
}

// turn is one live duplex stream: the conversation, its body and response,
// and the liveness watchdog that bounds silence.
type turn struct {
	conversation *conversation
	reader       *cursor.FrameReader
	writer       *io.PipeWriter
	response     *http.Response
	idled        atomic.Bool
	watchdog     *time.Timer
	stopBeats    func()
}

// beginTurn prepares and opens one duplex stream. cancel is the run's own
// cancellation, which the idle watchdog fires when the stream goes silent.
func (r *Runner) beginTurn(ctx context.Context, cancel context.CancelFunc) (*turn, error) {
	tools, history, action, err := r.prepare()
	if err != nil {
		return nil, err
	}
	initial := cursor.Frame(buildRunRequest(history.State, action, r.Selection))
	if r.Record != nil {
		r.Record(initial)
	}
	reader, writer, response, err := r.openStream(ctx, initial)
	if err != nil {
		return nil, err
	}
	if err := r.checkStreamStatus(response); err != nil {
		_ = writer.Close()
		return nil, err
	}
	conversation := newConversation(tools, history, r.Emit)
	conversation.write = func(payload []byte) error {
		_, err := writer.Write(cursor.Frame(payload))
		return err
	}
	turn := &turn{conversation: conversation, reader: reader, writer: writer, response: response}
	watchdog := time.AfterFunc(orDefault(r.IdleTimeout, defaultIdleTimeout), func() {
		turn.idled.Store(true)
		cancel()
	})
	turn.watchdog = watchdog
	conversation.touch = func() { watchdog.Reset(orDefault(r.IdleTimeout, defaultIdleTimeout)) }
	turn.stopBeats = r.heartbeat(conversation.write)
	return turn, nil
}

// converse runs the turn to completion and releases the stream. The request
// body is closed here so the upstream never queues the next turn behind this
// one; an unclosed body leaves the run live forever.
func (t *turn) converse(ctx context.Context) error {
	runErr := t.conversation.converse(ctx, t.reader)
	t.stopBeats()
	t.watchdog.Stop()
	_ = t.writer.Close()
	if t.conversation.externalHandoff {
		// The handoff answer has to reach the server before the caller's
		// cancel tears the request down, or the transport resets the stream
		// and Cursor keeps waiting on an execution Relo already answered.
		awaitStreamEnd(t.reader, time.Second)
	}
	return runErr
}

// awaitStreamEnd reads until the server ends the stream or the grace runs
// out. Only a handoff turn calls it, and it answers nothing further.
func awaitStreamEnd(reader *cursor.FrameReader, grace time.Duration) {
	if reader == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, ended, err := reader.Next(); ended || err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(grace):
	}
}

func orDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

// heartbeat pings the server until the returned stop is called. The pipe
// serializes whole writes, so pings never interleave with replies.
func (r *Runner) heartbeat(write func([]byte) error) (stop func()) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(orDefault(r.HeartbeatEvery, defaultHeartbeatEvery))
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if write(buildHeartbeat()) != nil {
					return
				}
			}
		}
	}()
	// A ping blocked on a stalled stream ends when Run closes the writer.
	return func() { close(done) }
}

// streamRefusal reads why a stream that ended without finishing its turn was
// stopped. Cursor answers with an end-of-stream frame and repeats the verdict
// in gRPC trailers; whichever arrived names the refusal.
func streamRefusal(reader *cursor.FrameReader, response *http.Response) error {
	if payload, ended := reader.End(); ended {
		if refusal, ok := cursor.ParseEndStream(payload); ok {
			return refusal
		}
	}
	if refusal, ok := cursor.TrailerRefusal(response.Trailer); ok {
		return refusal
	}
	return nil
}

func (r *Runner) prepare() (*ToolMap, History, []byte, error) {
	request := r.Request
	if r.RefusesMaxOutputTokens && request.MaxTokens != 0 {
		dropped := *request
		dropped.MaxTokens = 0
		request = &dropped
	}
	if err := Validate(request, r.Selection); err != nil {
		return nil, History{}, nil, err
	}
	tools, err := BuildToolMap(request.Tools)
	if err != nil {
		return nil, History{}, nil, err
	}
	history, action, err := BuildHistory(request, tools)
	if err != nil {
		return nil, History{}, nil, err
	}
	return tools, history, action, nil
}

func (r *Runner) checkStreamStatus(response *http.Response) error {
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	default:
		return r.statusError(response)
	}
}

func (r *Runner) openStream(ctx context.Context, initial []byte) (*cursor.FrameReader, *io.PipeWriter, *http.Response, error) {
	request, writer, err := streamRequest(ctx, r.BaseURL, r.Token)
	if err != nil {
		return nil, nil, nil, err
	}
	done := make(chan streamOutcome, 1)
	go func() {
		response, err := r.Client.Do(request)
		done <- streamOutcome{response: response, err: err}
	}()
	// The initial frame goes out while the server buffers its response
	// headers: a handler that reads before writing would otherwise wait
	// for a body the client only sends after those headers arrive.
	if _, err := writer.Write(initial); err != nil {
		_ = writer.Close()
		return nil, nil, nil, err
	}
	result, err := awaitStream(ctx, writer, done)
	if err != nil {
		return nil, nil, nil, err
	}
	return cursor.NewFrameReader(result.response.Body), writer, result.response, nil
}

func streamRequest(ctx context.Context, baseURL, token string) (*http.Request, *io.PipeWriter, error) {
	base := strings.TrimSuffix(baseURL, "/")
	if base == "" {
		base = cursor.DefaultBaseURL
	}
	body, writer := io.Pipe()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+cursor.RunPath, body)
	if err != nil {
		_ = writer.Close()
		return nil, nil, err
	}
	// A pipe body reports no length, which the transport would read as an
	// empty body. Unknown length streams the turn until the pipe closes.
	request.ContentLength = -1
	headers := cursor.Headers(token, "", newRequestID())
	headers["Content-Type"] = "application/connect+proto"
	headers["Te"] = "trailers"
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	return request, writer, nil
}

type streamOutcome struct {
	response *http.Response
	err      error
}

func awaitStream(ctx context.Context, writer *io.PipeWriter, done <-chan streamOutcome) (streamOutcome, error) {
	select {
	case <-ctx.Done():
		_ = writer.Close()
		finished := <-done
		if finished.response != nil {
			_ = finished.response.Body.Close()
		}
		return streamOutcome{}, ctx.Err()
	case result := <-done:
		if result.err != nil {
			_ = writer.Close()
			return streamOutcome{}, result.err
		}
		return result, nil
	}
}

func (r *Runner) statusError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if r.ConnectError != nil {
		if info := r.ConnectError(response.StatusCode, body); info != nil {
			return fmt.Errorf("cursor answered %d: %s", response.StatusCode, info.Message)
		}
	}
	return fmt.Errorf("cursor answered %d", response.StatusCode)
}

func newRequestID() string {
	return turnMessageID()
}

// conversation holds the state of one Cursor turn.
type conversation struct {
	tools     *ToolMap
	history   History
	emit      func([]inference.Event)
	write     func([]byte) error
	touch     func()
	events    []inference.Event
	calls     map[string]bool
	callIndex int
	output    int
	usage     inference.UsageReport
	usedUsage bool
	toolUse   bool
	turnDone  bool
	// externalHandoff reports that a native execution was handed to the
	// calling client. The turn is over for Relo even though Cursor never sent
	// turnEnded: the result arrives in the client's next request.
	externalHandoff bool
}

func newConversation(tools *ToolMap, history History, emit func([]inference.Event)) *conversation {
	return &conversation{tools: tools, history: history, emit: emit, calls: map[string]bool{}}
}

func (c *conversation) publish(events ...inference.Event) {
	if len(events) == 0 {
		return
	}
	c.events = append(c.events, events...)
	if c.emit != nil {
		c.emit(events)
	}
}

// converse reads server frames and answers protocol requests until the
// turn ends or the stream fails.
func (c *conversation) converse(ctx context.Context, reader *cursor.FrameReader) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		payload, ended, err := reader.Next()
		if err != nil {
			return err
		}
		if c.touch != nil {
			c.touch()
		}
		if ended {
			return nil
		}
		if err := c.handle(payload); err != nil {
			return err
		}
		if c.turnDone {
			return nil
		}
		if c.externalHandoff {
			return nil
		}
	}
}

func (c *conversation) handle(payload []byte) error {
	message, err := ParseServerMessage(payload)
	if err != nil {
		return err
	}
	switch {
	case message.Query:
		return ErrInteraction
	case message.Control:
		return errors.New("cursor aborted the turn")
	case message.Checkpoint:
		return nil
	case message.Update != nil:
		return c.handleUpdate(message)
	case message.Exec != nil:
		return c.handleExec(message)
	case message.KV != nil:
		return c.handleKV(message)
	default:
		return nil
	}
}

func (c *conversation) handleUpdate(message ServerMessage) error {
	switch message.UpdateKind {
	case updateTextDelta:
		text := cursor.FirstString(message.Update, textDeltaText)
		if text != "" {
			c.publish(inference.Event{Kind: inference.EventTextDelta, Text: text})
		}
	case updateThinkingDelta:
		text := cursor.FirstString(message.Update, thinkingText)
		if text != "" {
			c.publish(inference.Event{Kind: inference.EventReasoningDelta, Text: text})
		}
	case updateTokenDelta:
		c.output += int(cursor.Uint(message.Update, tokenDeltaTokens))
	case updateTurnEnded:
		c.applyTurnEnded(message.Update)
	case updateToolStarted, updateToolCompleted:
		return c.handleToolUpdate(message)
	case updatePartialToolCall:
		return c.handlePartialUpdate(message)
	case updateToolDelta:
		// Structured deltas cover native shell/edit calls only; declared
		// MCP calls always arrive whole over the exec channel.
		return nil
	default:
		return nil
	}
	return nil
}

func (c *conversation) applyTurnEnded(update []cursor.Field) {
	usage := inference.UsageReport{
		InputTokens:      int(optionalInt(update, turnedInput)),
		OutputTokens:     int(optionalInt(update, turnedOutput)),
		CacheReadTokens:  int(optionalInt(update, turnedCacheIn)),
		CacheWriteTokens: int(optionalInt(update, turnedCacheOut)),
	}
	if usage == (inference.UsageReport{}) {
		usage.OutputTokens = c.output
	}
	c.usage = usage
	c.usedUsage = true
	c.turnDone = true
}

func optionalInt(fields []cursor.Field, number int) int64 {
	for _, field := range fields {
		if field.Number == number && field.Wire == 0 {
			return int64(field.Varint)
		}
	}
	return 0
}

func (c *conversation) handleToolUpdate(message ServerMessage) error {
	idField, callField := toolStartedID, toolStartedCall
	if message.UpdateKind == updateToolCompleted {
		idField, callField = toolCompletedID, toolCompletedCall
	}
	callID := cursor.FirstString(message.Update, idField)
	calls, err := cursor.Nested(message.Update, callField)
	if err != nil || len(calls) == 0 {
		return err
	}
	call, found, err := ParseToolCall(mustEncode(calls[0]))
	if err != nil || !found {
		if err != nil {
			return err
		}
		return c.handleNativeToolUpdate(mustEncode(calls[0]), callID)
	}
	if call.ToolCallID == "" {
		call.ToolCallID = callID
	}
	return c.emitMCPCall(call, 0, "", false)
}

// handleNativeToolUpdate publishes a native call announced as an update. It
// never answers on the exec channel: the execution frame that follows is what
// the server waits on, and only that frame moves the work to the client.
func (c *conversation) handleNativeToolUpdate(payload []byte, callID string) error {
	call, found, err := ParseNativeToolCall(payload)
	if err != nil || !found {
		return err
	}
	call.callID = firstNonEmpty(call.callID, callID)
	tool, declared := c.resolveNativeCall(call)
	if !declared {
		return nil
	}
	id := c.nativeCallID(call)
	if c.calls[id] {
		return nil
	}
	c.calls[id] = true
	c.toolUse = true
	arguments := nativeArgsJSON(call.arguments(tool.Name))
	c.publish(
		inference.Event{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id, Name: tool.Name}},
		inference.Event{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id, Arguments: arguments}},
		inference.Event{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id}},
	)
	c.callIndex++
	return nil
}

func mustEncode(fields []cursor.Field) []byte {
	var out cursor.Writer
	for _, field := range fields {
		switch field.Wire {
		case 0:
			out.Uint32(field.Number, uint32(field.Varint))
		case 2:
			out.Bytes(field.Number, field.Raw)
		}
	}
	return out.BytesOut()
}

func (c *conversation) handlePartialUpdate(message ServerMessage) error {
	callID := cursor.FirstString(message.Update, partialID)
	delta := cursor.FirstString(message.Update, partialArgsDelta)
	calls, err := cursor.Nested(message.Update, partialCall)
	if err != nil {
		return err
	}
	name := ""
	if len(calls) > 0 {
		if call, found, err := ParseToolCall(mustEncode(calls[0])); err != nil {
			return err
		} else if found {
			name = call.Name
		}
	}
	if callID == "" || name == "" {
		return nil
	}
	c.emitPartialCall(callID, name, delta)
	return nil
}

// resolveNativeCall finds the declared tool a native execution maps onto, in
// the order the caller's own tool list prefers it.
func (c *conversation) resolveNativeCall(call nativeCall) (inference.Tool, bool) {
	for _, name := range call.candidates {
		if tool, found := c.tools.Canonical(WireName(name)); found {
			return tool, true
		}
	}
	return inference.Tool{}, false
}

// nativeCallID derives the stable id one native execution is emitted under,
// so an update and its execution frame become one canonical call.
func (c *conversation) nativeCallID(call nativeCall) string {
	if call.callID != "" {
		return normalizeToolCallID(call.callID)
	}
	return normalizeToolCallID("native-" + itoa(c.callIndex))
}

// emitMCPCall publishes one declared tool call for external execution.
// Update-observed calls publish events only; exec-channel calls always get
// their acknowledgement reply, which the server waits on.
func (c *conversation) emitMCPCall(call MCPCall, numericID uint32, execID string, reply bool) error {
	id := normalizeToolCallID(call.ToolCallID)
	name := call.ToolName
	if name == "" {
		name = call.Name
	}
	tool, found := c.tools.Canonical(name)
	if !found {
		return c.replyUnknown(name, numericID, execID, reply)
	}
	if c.calls[id] {
		return c.replyHandoff(numericID, execID, reply)
	}
	c.calls[id] = true
	c.toolUse = true
	arguments := mcpArgsJSON(call.Args)
	c.publish(
		inference.Event{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id, Name: tool.Name}},
		inference.Event{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id, Arguments: arguments}},
		inference.Event{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id}},
	)
	c.callIndex++
	return c.replyHandoff(numericID, execID, reply)
}

func (c *conversation) replyUnknown(name string, numericID uint32, execID string, reply bool) error {
	if reply {
		return c.reply(buildClientExec(numericID, execID, resultMCP, buildMCPNotFound(name, c.tools.Names())))
	}
	return nil
}

func (c *conversation) replyHandoff(numericID uint32, execID string, reply bool) error {
	if reply {
		return c.reply(buildClientExec(numericID, execID, resultMCP, buildMCPHandoff()))
	}
	return nil
}

func (c *conversation) emitPartialCall(callID, name, delta string) {
	id := normalizeToolCallID(callID)
	tool, found := c.tools.Canonical(name)
	if !found || c.calls[id] {
		return
	}
	c.calls[id] = true
	c.toolUse = true
	c.publish(
		inference.Event{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id, Name: tool.Name}},
		inference.Event{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id, Arguments: delta}},
	)
	c.callIndex++
}

func mcpArgsJSON(args map[string][]byte) string {
	decoded := make(map[string]any, len(args))
	for key, raw := range args {
		decoded[key] = decodeArgValue(raw)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func (c *conversation) handleExec(message ServerMessage) error {
	switch message.ExecKind {
	case execContext:
		return c.reply(buildClientExec(message.ExecNumericID, message.ExecID, resultContext, buildRequestContext(c.history.Rules, c.tools.Defs())))
	case execMCPState:
		return c.reply(buildClientExec(message.ExecNumericID, message.ExecID, resultMCPState, buildMCPState(c.tools.Defs())))
	case 11:
		return c.handleMCPExec(message)
	default:
		if call, ok := translateNativeExec(message); ok {
			return c.handleNativeExec(call, message.ExecNumericID, message.ExecID)
		}
		field, result := buildNativeRejection(message.ExecKind)
		if field == 0 {
			return fmt.Errorf("%w: exec variant %d", ErrUnsupportedExec, message.ExecKind)
		}
		return c.reply(buildClientExec(message.ExecNumericID, message.ExecID, field, result))
	}
}

// handleNativeExec hands a native execution to the calling client. A declared
// tool becomes a canonical tool call and the turn ends; an undeclared one is
// refused in the variant the server is waiting on, so the model can pick a
// declared tool instead.
func (c *conversation) handleNativeExec(call nativeCall, numericID uint32, execID string) error {
	tool, declared := c.resolveNativeCall(call)
	if !declared {
		field, result := buildNativeRejection(call.variant)
		if field == 0 {
			return fmt.Errorf("%w: exec variant %d", ErrUnsupportedExec, call.variant)
		}
		return c.reply(buildClientExec(numericID, execID, field, result))
	}
	call.callID = firstNonEmpty(call.callID, execID)
	id := c.nativeCallID(call)
	if !c.calls[id] {
		c.calls[id] = true
		c.toolUse = true
		arguments := nativeArgsJSON(call.arguments(tool.Name))
		c.publish(
			inference.Event{Kind: inference.EventToolCallStart, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id, Name: tool.Name}},
			inference.Event{Kind: inference.EventToolCallDelta, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id, Arguments: arguments}},
			inference.Event{Kind: inference.EventToolCallEnd, ToolCall: &inference.ToolCallDelta{Index: c.callIndex, ID: id}},
		)
		c.callIndex++
	}
	c.externalHandoff = true
	field, result := buildNativeHandoff(call)
	return c.reply(buildClientExec(numericID, execID, field, result))
}

func (c *conversation) handleMCPExec(message ServerMessage) error {
	call, err := ParseMCPCall(mustEncode(message.Exec))
	if err != nil {
		return err
	}
	call.ToolCallID = firstNonEmpty(call.ToolCallID, message.ExecID)
	return c.emitMCPCall(call, message.ExecNumericID, message.ExecID, true)
}

func (c *conversation) handleKV(message ServerMessage) error {
	switch message.KVKind {
	case kvGetArgs:
		return c.reply(buildKVGet(message.KVID, blobFor(message.KV, c.history.Store)))
	case kvSetArgs:
		c.history.Store.PutID(blobIDOf(message.KV), blobBytes(message.KV))
		return c.reply(buildKVSet(message.KVID))
	default:
		return nil
	}
}

func blobFor(args []cursor.Field, store *Store) []byte {
	for _, field := range args {
		if field.Number == blobIDField && field.Wire == 2 {
			blob, _ := store.Get(field.Raw)
			return blob
		}
	}
	return nil
}

func blobIDOf(args []cursor.Field) []byte {
	for _, field := range args {
		if field.Number == blobIDField && field.Wire == 2 {
			return field.Raw
		}
	}
	return nil
}

func blobBytes(args []cursor.Field) []byte {
	for _, field := range args {
		if field.Number == 2 && field.Wire == 2 {
			return field.Raw
		}
	}
	return nil
}

func (c *conversation) reply(payload []byte) error {
	if c.write == nil {
		return nil
	}
	return c.write(payload)
}

// finish ends the turn: usage first, then exactly one terminal event. A
// refusal the server sent explains a stream that stopped without finishing, so
// it is reported instead of the bare fact that the turn ended early.
func (c *conversation) finish(runErr, refusal error) ([]inference.Event, error) {
	if c.externalHandoff {
		return c.finishHandoff()
	}
	if runErr != nil {
		if c.turnDone {
			return c.events, nil
		}
		return c.events, runErr
	}
	if !c.turnDone {
		if refusal != nil {
			return c.events, refusal
		}
		return c.events, ErrNoTurnEnd
	}
	c.publishUsage()
	reason := inference.EventReasonStop
	if c.toolUse {
		reason = inference.EventReasonToolUse
	}
	c.publish(inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: reason}})
	return c.events, nil
}

// finishHandoff ends a turn whose work moved to the calling client: the usage
// Cursor supplied, then tool_use. It is not an upstream failure, so the run
// error the stream teardown produced is not reported.
func (c *conversation) finishHandoff() ([]inference.Event, error) {
	c.publishUsage()
	c.publish(inference.Event{Kind: inference.EventTerminal, Terminal: &inference.TerminalInfo{Reason: inference.EventReasonToolUse}})
	return c.events, nil
}

func (c *conversation) publishUsage() {
	if !c.usedUsage {
		return
	}
	c.publish(inference.Event{Kind: inference.EventUsage, Usage: &inference.UsageReport{
		InputTokens: c.usage.InputTokens, OutputTokens: c.usage.OutputTokens,
		CacheReadTokens: c.usage.CacheReadTokens, CacheWriteTokens: c.usage.CacheWriteTokens,
	}})
}
