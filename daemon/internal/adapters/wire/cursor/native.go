// Native Cursor executions handed to the calling client.
package cursor

import (
	"encoding/json"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
)

// Exec variant numbers of the native executions Relo can hand off. The
// context, MCP, and state variants live in protocol.go.
const (
	execShell        = 2
	execWrite        = 3
	execDelete       = 4
	execGrep         = 5
	execRead         = 7
	execLs           = 8
	execDiagnostics  = 9
	execShellStream  = 14
	execRedactedRead = 29
)

// ToolCall variant numbers of the same calls when they arrive as updates.
const (
	callShell   = 1
	callDelete  = 3
	callGrep    = 5
	callRead    = 8
	callLs      = 13
	callIDField = 57
)

// Argument fields of the native execution messages.
const (
	shellCommand     = 1
	shellWorkingDir  = 2
	shellTimeout     = 3
	shellCallID      = 4
	shellHardTimeout = 14
	readPath         = 1
	readCallID       = 2
	readOffset       = 4
	readLimit        = 5
	// The read tool call carries its own argument message, which numbers the
	// offset and the limit differently from the execution channel's read.
	readToolOffset      = 2
	readToolLimit       = 3
	grepPattern         = 1
	grepPath            = 2
	grepGlob            = 3
	grepCaseInsensitive = 8
	grepHeadLimit       = 10
	grepCallID          = 14
	writePath           = 1
	writeText           = 2
	writeCallID         = 3
	writeBytes          = 5
	deletePath          = 1
	deleteCallID        = 2
	lsPath              = 1
	lsCallID            = 3
	diagnosticsPath     = 1
	diagnosticsCallID   = 2
	// toolCallArgsField is the args field every ToolCall variant nests.
	toolCallArgsField = 1
)

// nativeCall is one native execution translated to a client tool call.
type nativeCall struct {
	variant int
	// canonical names the execution may have been declared under, in
	// preference order: a caller may declare bash, shell, or both.
	candidates []string
	callID     string
	fields     []cursor.Field
	path       string
	command    string
	workDir    string
	// offsetField and limitField are where this call's own message keeps the
	// read's offset and limit, which differ between the execution channel
	// and the tool call update.
	offsetField int
	limitField  int
}

// translateNativeExec decodes one native ExecServerMessage variant into a
// client tool call. It reports false for variants Relo cannot hand off.
func translateNativeExec(message ServerMessage) (nativeCall, bool) {
	return translateNativeArgs(message.ExecKind, message.Exec, message.ExecID)
}

// translateNativeArgs decodes one variant's arguments. Exec frames and tool
// call updates carry the same messages, so both enter here.
func translateNativeArgs(variant int, fields []cursor.Field, execID string) (nativeCall, bool) {
	spec, known := nativeSpecs[variant]
	if !known {
		return nativeCall{}, false
	}
	return translateNativeSpec(spec, variant, fields, execID), true
}

// translateNativeSpec decodes one variant's arguments under a given reading
// of the message, which differs between the execution channel and the tool
// call update.
func translateNativeSpec(spec nativeSpec, variant int, fields []cursor.Field, execID string) nativeCall {
	call := nativeCall{variant: variant, fields: fields, candidates: spec.candidates, callID: execID}
	call.path = cursor.FirstString(fields, spec.pathField)
	call.offsetField, call.limitField = spec.offsetField, spec.limitField
	if spec.shell {
		call.command = cursor.FirstString(fields, shellCommand)
		call.workDir = cursor.FirstString(fields, shellWorkingDir)
	}
	call.callID = firstNonEmpty(cursor.FirstString(fields, spec.callField), call.callID)
	return call
}

// nativeSpec says how one native variant's arguments are read. The native
// messages differ only in which fields carry the path, the call id, and the
// shell command, so one table replaces eight near-identical decoders.
type nativeSpec struct {
	candidates []string
	pathField  int
	callField  int
	shell      bool
	// offsetField and limitField are the read tool call's own numbers, which
	// differ from the execution channel's read.
	offsetField int
	limitField  int
}

var nativeSpecs = map[int]nativeSpec{
	execShell:        {candidates: []string{"bash", "shell"}, callField: shellCallID, shell: true},
	execShellStream:  {candidates: []string{"bash", "shell"}, callField: shellCallID, shell: true},
	execRead:         {candidates: []string{"read"}, pathField: readPath, callField: readCallID, offsetField: readOffset, limitField: readLimit},
	execRedactedRead: {candidates: []string{"read"}, pathField: readPath, callField: readCallID, offsetField: readOffset, limitField: readLimit},
	execWrite:        {candidates: []string{"write"}, pathField: writePath, callField: writeCallID},
	execDelete:       {candidates: []string{"delete"}, pathField: deletePath, callField: deleteCallID},
	execGrep:         {candidates: []string{"grep"}, pathField: grepPath, callField: grepCallID},
	execLs:           {candidates: []string{"ls"}, pathField: lsPath, callField: lsCallID},
	execDiagnostics:  {candidates: []string{"diagnostics"}, pathField: diagnosticsPath, callField: diagnosticsCallID},
}

// nativeReadToolCallSpec is the read tool call's own argument message. It
// numbers the offset and the limit differently from the execution channel's
// read arguments, and it carries no call id: the update's own call id names
// the call.
var nativeReadToolCallSpec = nativeSpec{
	candidates:  []string{"read"},
	pathField:   readPath,
	offsetField: readToolOffset,
	limitField:  readToolLimit,
}

// ParseNativeToolCall decodes a native ToolCall update into the same shape
// the execution channel carries. It reports false for declared MCP calls,
// which travel the exec channel instead.
func ParseNativeToolCall(payload []byte) (nativeCall, bool, error) {
	fields, err := cursor.Parse(payload)
	if err != nil {
		return nativeCall{}, false, err
	}
	callID := cursor.FirstString(fields, callIDField)
	for _, field := range fields {
		if field.Wire != 2 || field.Number == callIDField {
			continue
		}
		variant, known := toolCallVariant(field.Number)
		if !known {
			continue
		}
		// A tool call update carries the variant's own message, whose args
		// field holds the arguments. Reading the variant message as the
		// arguments themselves puts the whole message into the command.
		variantFields, err := cursor.Parse(field.Raw)
		if err != nil {
			return nativeCall{}, false, err
		}
		args, err := cursor.Nested(variantFields, toolCallArgsField)
		if err != nil {
			return nativeCall{}, false, err
		}
		if len(args) == 0 {
			return nativeCall{}, false, nil
		}
		spec := nativeSpecs[variant]
		if variant == execRead {
			spec = nativeReadToolCallSpec
		}
		return translateNativeSpec(spec, variant, args[0], callID), true, nil
	}
	return nativeCall{}, false, nil
}

func toolCallVariant(number int) (int, bool) {
	switch number {
	case callShell:
		return execShell, true
	case callDelete:
		return execDelete, true
	case callGrep:
		return execGrep, true
	case callRead:
		return execRead, true
	case callLs:
		return execLs, true
	default:
		return 0, false
	}
}

// arguments translates one native execution into the JSON the named canonical
// tool expects. The shell directory key follows that tool, because OpenCode
// names it workdir and the bash tool names it cwd.
func (call nativeCall) arguments(canonical string) map[string]any {
	switch call.variant {
	case execShell, execShellStream:
		return call.shellArguments(canonical == "shell")
	case execRead, execRedactedRead:
		return call.readArguments()
	case execWrite:
		return call.writeArguments()
	case execGrep:
		return call.grepArguments()
	default:
		return map[string]any{"path": call.path}
	}
}

func (call nativeCall) shellArguments(shell bool) map[string]any {
	args := map[string]any{"command": call.command}
	if call.workDir != "" {
		key := "cwd"
		if shell {
			key = "workdir"
		}
		args[key] = call.workDir
	}
	if timeout, ok := shellDeadline(call.fields, shell); ok {
		args["timeout"] = timeout
	}
	return args
}

func (call nativeCall) readArguments() map[string]any {
	args := map[string]any{"path": call.path}
	if offset, ok := intField(call.fields, call.offsetField); ok {
		args["offset"] = offset
	}
	if limit, ok := intField(call.fields, call.limitField); ok {
		args["limit"] = limit
	}
	return args
}

func (call nativeCall) writeArguments() map[string]any {
	content := cursor.FirstString(call.fields, writeText)
	if content == "" {
		content = string(bytesField(call.fields, writeBytes))
	}
	return map[string]any{"path": call.path, "content": content}
}

func (call nativeCall) grepArguments() map[string]any {
	path := call.path
	if path == "" {
		path = "."
	}
	args := map[string]any{"pattern": cursor.FirstString(call.fields, grepPattern), "path": path}
	if glob := cursor.FirstString(call.fields, grepGlob); glob != "" {
		args["include"] = glob
	}
	if insensitive, ok := boolField(call.fields, grepCaseInsensitive); ok {
		args["caseSensitive"] = !insensitive
	}
	if limit, ok := intField(call.fields, grepHeadLimit); ok {
		args["limit"] = limit
	}
	return args
}

// shellDeadline maps Cursor's shell timing onto the client tool. The shell
// tool takes milliseconds; the bash tool takes seconds and treats Cursor's
// timeout as a block-until wait, so only a hard timeout or a wait past the
// bash default becomes a kill deadline.
func shellDeadline(fields []cursor.Field, millis bool) (int64, bool) {
	hard, hasHard := intField(fields, shellHardTimeout)
	wait, hasWait := intField(fields, shellTimeout)
	if millis {
		if hasHard && hard > 0 {
			return hard, true
		}
		if hasWait && wait > 0 {
			return wait, true
		}
		return 0, false
	}
	if hasHard {
		return seconds(hard)
	}
	if hasWait && wait > 300_000 {
		return seconds(wait)
	}
	return 0, false
}

func seconds(millis int64) (int64, bool) {
	if millis <= 0 {
		return 0, false
	}
	value := (millis + 500) / 1000
	if value < 1 {
		value = 1
	}
	return value, true
}

func intField(fields []cursor.Field, number int) (int64, bool) {
	for _, field := range fields {
		if field.Number == number && field.Wire == 0 {
			return int64(field.Varint), true
		}
	}
	return 0, false
}

func boolField(fields []cursor.Field, number int) (bool, bool) {
	value, ok := intField(fields, number)
	return value != 0, ok
}

func bytesField(fields []cursor.Field, number int) []byte {
	for _, field := range fields {
		if field.Number == number && field.Wire == 2 {
			return field.Raw
		}
	}
	return nil
}

// buildNativeHandoff answers a native execution whose work moved to the
// client. The result fills the variant the server is waiting on, so the turn
// is not left blocked, and it never claims the daemon ran anything.
func buildNativeHandoff(call nativeCall) (int, []byte) {
	switch call.variant {
	case execShell, execShellStream:
		var rejected cursor.Writer
		rejected.String(1, call.command)
		rejected.String(2, call.workDir)
		rejected.String(3, HandoffText)
		var result cursor.Writer
		result.Message(rejectedNoShell, rejected.BytesOut())
		return resultShell, result.BytesOut()
	case execGrep:
		var failure cursor.Writer
		failure.String(1, HandoffText)
		var result cursor.Writer
		result.Message(grepErrorNo, failure.BytesOut())
		return resultGrep, result.BytesOut()
	default:
		var rejected cursor.Writer
		rejected.String(1, call.path)
		rejected.String(2, HandoffText)
		var result cursor.Writer
		result.Message(nativeRejectedField(call.variant), rejected.BytesOut())
		return nativeResultField(call.variant), result.BytesOut()
	}
}

func nativeRejectedField(variant int) int {
	switch variant {
	case execWrite:
		return rejectedNoWrite
	case execDelete:
		return rejectedNoDel
	case execLs:
		return rejectedNoLs
	case execDiagnostics:
		return rejectedNoDiag
	default:
		return rejectedNoRead
	}
}

func nativeResultField(variant int) int {
	switch variant {
	case execWrite:
		return resultWrite
	case execDelete:
		return resultDelete
	case execLs:
		return resultLs
	case execDiagnostics:
		return resultDiag
	default:
		return resultRead
	}
}

func nativeArgsJSON(args map[string]any) string {
	encoded, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
