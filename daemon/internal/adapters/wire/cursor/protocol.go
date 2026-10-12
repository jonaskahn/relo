// Cursor agent message construction.
package cursor

import (
	"github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// Field numbers verified against the pinned Cursor reference schemas.
const (
	clientRunRequest  = 1
	clientExecMessage = 2
	clientKVMessage   = 3

	runConversationState = 1
	runAction            = 2
	runModelDetails      = 3
	runConversationID    = 5
	runRequestedModel    = 9

	detailsModelID   = 1
	detailsDisplayID = 3
	detailsName      = 4

	requestedModelID = 1
	requestedMaxMode = 2
	requestedParams  = 3
	paramPairID      = 1
	paramPairValue   = 2

	actionUserMessage = 1
	actionResume      = 2
	userMessageText   = 1
	// userActionMessage is the userMessage field of UserMessageAction: the
	// action carries the wrapper, and the wrapper carries the message.
	userActionMessage = 1

	execIDField = 1
	execExecID  = 15

	execContext  = 10
	execMCPState = 36

	kvIDField = 1
	kvGetArgs = 2
	kvSetArgs = 3

	blobIDField = 1

	resultShell    = 2
	resultWrite    = 3
	resultDelete   = 4
	resultGrep     = 5
	resultRead     = 7
	resultLs       = 8
	resultDiag     = 9
	resultContext  = 10
	resultMCP      = 11
	resultMCPState = 36

	mcpSuccessCase  = 1
	rejectedNoRead  = 3
	rejectedNoWrite = 6
	rejectedNoDel   = 6
	rejectedNoShell = 4
	rejectedNoLs    = 3
	rejectedNoDiag  = 3
	grepErrorNo     = 2

	mcpStateSuccess = 1
	mcpStateServer  = 1
	stateServerName = 1
	stateServerID   = 2
	stateTools      = 5

	defName        = 1
	defDescription = 2
	defSchema      = 3
	defProvider    = 4
	defToolName    = 5

	kvGetResult = 2
	kvSetResult = 3
	blobData    = 1

	rulePath    = 1
	ruleContent = 2
	ruleType    = 3
	ruleGlobal  = 1
	ruleSource  = 4
	ruleUser    = 1

	contextRules = 2
	contextTools = 7
)

// providerIdentifier names the server the declared tools belong to.
const providerIdentifier = "relo"

// RejectionReason is the reason native execution frames are refused with.
// Execution stays with the calling client; the daemon never runs it.
const RejectionReason = "Relo does not execute tools on the daemon: the calling client runs every tool"

// HandoffText acknowledges a declared tool call Cursor must not retry. The
// result arrives in the caller's next request.
const HandoffText = "Tool call received and handed off to the external client for execution. Do not retry or call it again; end the turn. The result will be provided in the next request."

// buildRunRequest encodes the initial AgentClientMessage carrying the run
// request.
func buildRunRequest(state, action []byte, selection *wire.CursorRequest) []byte {
	var details cursor.Writer
	details.String(detailsModelID, selection.DetailsID)
	details.String(detailsDisplayID, selection.DetailsID)
	details.String(detailsName, selection.DetailsID)
	var requested cursor.Writer
	requested.String(requestedModelID, selection.ModelID)
	requested.Bool(requestedMaxMode, selection.MaxMode)
	for _, param := range selection.Params {
		var pair cursor.Writer
		pair.String(paramPairID, param.ID)
		pair.String(paramPairValue, param.Value)
		requested.Message(requestedParams, pair.BytesOut())
	}
	var run cursor.Writer
	if len(state) == 0 {
		// The server requires the field to be present, so an empty state is
		// sent as an empty message rather than omitted.
		run.Empty(runConversationState)
	} else {
		run.Message(runConversationState, state)
	}
	run.Message(runAction, action)
	run.Message(runModelDetails, details.BytesOut())
	run.Message(runRequestedModel, requested.BytesOut())
	run.String(runConversationID, selection.ConversationID)
	var client cursor.Writer
	client.Message(clientRunRequest, run.BytesOut())
	return client.BytesOut()
}

// buildUserAction encodes a user-message conversation action. The text
// rides three levels down: the action holds the user-message action, which
// holds the user message, which holds the text. Fewer levels put the text
// where a length prefix belongs, and the upstream cannot parse the turn.
func buildUserAction(text string) []byte {
	var user cursor.Writer
	user.String(userMessageText, text)
	var userAction cursor.Writer
	userAction.Message(userActionMessage, user.BytesOut())
	var action cursor.Writer
	action.Message(actionUserMessage, userAction.BytesOut())
	return action.BytesOut()
}

// buildResumeAction encodes an empty resume conversation action.
func buildResumeAction() []byte {
	var action cursor.Writer
	action.Empty(actionResume)
	return action.BytesOut()
}

// buildRule encodes one system rule.
func buildRule(path, content string) []byte {
	var rule cursor.Writer
	rule.String(rulePath, path)
	rule.String(ruleContent, content)
	var typ cursor.Writer
	typ.Empty(ruleGlobal)
	rule.Message(ruleType, typ.BytesOut())
	rule.Uint32(ruleSource, ruleUser)
	return rule.BytesOut()
}

// buildToolDef encodes one declared MCP tool definition.
func buildToolDef(wireName, canonical, description, schema string) []byte {
	var def cursor.Writer
	def.String(defName, wireName)
	def.String(defDescription, description)
	// inputSchema is a google.protobuf.Value on the wire, so the JSON text
	// has to be encoded rather than copied into the field.
	def.Bytes(defSchema, encodeSchemaValue([]byte(schema)))
	def.String(defProvider, providerIdentifier)
	def.String(defToolName, wireName)
	_ = canonical
	return def.BytesOut()
}

// buildRequestContext encodes the request context carrying rules and tools.
func buildRequestContext(rules, tools [][]byte) []byte {
	var context cursor.Writer
	for _, rule := range rules {
		context.Message(contextRules, rule)
	}
	for _, tool := range tools {
		context.Message(contextTools, tool)
	}
	var success cursor.Writer
	success.Message(1, context.BytesOut())
	var result cursor.Writer
	result.Message(1, success.BytesOut())
	return result.BytesOut()
}

// buildMCPState encodes the MCP state answer listing the one server Relo
// serves its declared tools from.
func buildMCPState(tools [][]byte) []byte {
	var server cursor.Writer
	server.String(stateServerName, providerIdentifier)
	server.String(stateServerID, providerIdentifier)
	for _, tool := range tools {
		server.Message(stateTools, tool)
	}
	var success cursor.Writer
	success.Message(mcpStateServer, server.BytesOut())
	var result cursor.Writer
	result.Message(mcpStateSuccess, success.BytesOut())
	return result.BytesOut()
}

// buildExecMessage wraps one execution result for one execution id.
func buildExecMessage(numericID uint32, execID string, resultField int, result []byte) []byte {
	var message cursor.Writer
	message.Uint32(execIDField, numericID)
	message.String(execExecID, execID)
	message.Message(resultField, result)
	return message.BytesOut()
}

// buildClientExec wraps one execution result in an AgentClientMessage.
func buildClientExec(numericID uint32, execID string, resultField int, result []byte) []byte {
	var client cursor.Writer
	client.Message(clientExecMessage, buildExecMessage(numericID, execID, resultField, result))
	return client.BytesOut()
}

func buildNativeRejection(execField int) (int, []byte) {
	switch execField {
	case 2, 14, 52:
		var rejected cursor.Writer
		rejected.String(3, RejectionReason)
		var result cursor.Writer
		result.Message(rejectedNoShell, rejected.BytesOut())
		return resultShell, result.BytesOut()
	case 5:
		var failure cursor.Writer
		failure.String(1, RejectionReason)
		var result cursor.Writer
		result.Message(grepErrorNo, failure.BytesOut())
		return resultGrep, result.BytesOut()
	case 7, 29:
		return resultRead, buildPathRejection(rejectedNoRead)
	case 8:
		return resultLs, buildPathRejection(rejectedNoLs)
	case 9:
		return resultDiag, buildPathRejection(rejectedNoDiag)
	case 3:
		return resultWrite, buildPathRejection(rejectedNoWrite)
	case 4:
		return resultDelete, buildPathRejection(rejectedNoDel)
	default:
		return 0, nil
	}
}

func buildPathRejection(rejectedField int) []byte {
	var rejected cursor.Writer
	rejected.String(2, RejectionReason)
	var result cursor.Writer
	result.Message(rejectedField, rejected.BytesOut())
	return result.BytesOut()
}

// buildMCPHandoff encodes the external-handoff acknowledgement for one
// declared tool call.
func buildMCPHandoff() []byte {
	var text cursor.Writer
	text.String(1, HandoffText)
	var item cursor.Writer
	item.Message(1, text.BytesOut())
	var success cursor.Writer
	success.Message(1, item.BytesOut())
	var result cursor.Writer
	result.Message(mcpSuccessCase, success.BytesOut())
	return result.BytesOut()
}

// buildMCPNotFound encodes the unknown-tool answer naming the tools the
// attempt declared.
func buildMCPNotFound(name string, available []string) []byte {
	var notFound cursor.Writer
	notFound.String(1, name)
	for _, tool := range available {
		notFound.String(2, tool)
	}
	var result cursor.Writer
	result.Message(5, notFound.BytesOut())
	return result.BytesOut()
}

// buildKVGet encodes one blob answer, empty when the store holds no blob.
// The answer is the result message itself: no extra level rides inside,
// because the server parses the bytes it asked for, not a wrapper.
func buildKVGet(numericID uint32, blob []byte) []byte {
	var message cursor.Writer
	message.Uint32(kvIDField, numericID)
	if len(blob) > 0 {
		var answered cursor.Writer
		answered.Bytes(blobData, blob)
		message.Message(kvGetResult, answered.BytesOut())
	} else {
		message.Empty(kvGetResult)
	}
	var client cursor.Writer
	client.Message(clientKVMessage, message.BytesOut())
	return client.BytesOut()
}

// buildKVSet encodes one blob-store acknowledgement: the result case set,
// with nothing inside it.
func buildKVSet(numericID uint32) []byte {
	var message cursor.Writer
	message.Uint32(kvIDField, numericID)
	message.Empty(kvSetResult)
	var client cursor.Writer
	client.Message(clientKVMessage, message.BytesOut())
	return client.BytesOut()
}
