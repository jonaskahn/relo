package cursor

import (
	"strings"
	"testing"

	adapterscursor "github.com/jonaskahn/relo/internal/adapters/cursor"
)

// argsFrame encodes one native variant's argument message from string and
// varint fields.
func argsFrame(text map[int]string, numbers map[int]int64) []byte {
	var args adapterscursor.Writer
	for number, value := range text {
		args.String(number, value)
	}
	for number, value := range numbers {
		args.Int64(number, value)
	}
	return args.BytesOut()
}

func translateFrame(t *testing.T, variant int, args []byte, execID string) nativeCall {
	t.Helper()
	fields, err := adapterscursor.Parse(args)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	call, ok := translateNativeArgs(variant, fields, execID)
	if !ok {
		t.Fatalf("translateNativeArgs(%d) refused a known variant", variant)
	}
	return call
}

// TestNativeArgumentsFollowEachVariant pins the arguments one native execution
// becomes on the client side: the keys, and the defaults a missing field takes.
func TestNativeArgumentsFollowEachVariant(t *testing.T) {
	t.Run("write takes the text, else the inline bytes", func(t *testing.T) {
		text := translateFrame(t, execWrite, argsFrame(map[int]string{writePath: "/tmp/a.go", writeText: "package a"}, nil), "exec-1")
		if args := text.arguments("write"); args["path"] != "/tmp/a.go" || args["content"] != "package a" {
			t.Fatalf("arguments = %v, want the path and the text", args)
		}
		var raw adapterscursor.Writer
		raw.Bytes(writeBytes, []byte("package b"))
		inline := translateFrame(t, execWrite, raw.BytesOut(), "exec-2")
		if args := inline.arguments("write"); args["content"] != "package b" {
			t.Fatalf("arguments = %v, want the inline bytes", args)
		}
	})

	t.Run("read carries the offset and the limit only when stated", func(t *testing.T) {
		call := translateFrame(t, execRedactedRead, argsFrame(
			map[int]string{readPath: "/tmp/a.go"},
			map[int]int64{readOffset: 40, readLimit: 20}), "exec-1")
		args := call.arguments("read")
		if args["path"] != "/tmp/a.go" || args["offset"] != int64(40) || args["limit"] != int64(20) {
			t.Fatalf("arguments = %v, want the path, offset, and limit", args)
		}
		bare := translateFrame(t, execRead, argsFrame(map[int]string{readPath: "/tmp/a.go"}, nil), "exec-2")
		if args := bare.arguments("read"); args["offset"] != nil || args["limit"] != nil {
			t.Fatalf("arguments = %v, want no offset or limit", args)
		}
	})

	t.Run("grep defaults the search root and maps its filters", func(t *testing.T) {
		call := translateFrame(t, execGrep, argsFrame(map[int]string{grepPattern: "TODO"}, nil), "exec-1")
		args := call.arguments("grep")
		if args["pattern"] != "TODO" || args["path"] != "." {
			t.Fatalf("arguments = %v, want the pattern and the default root", args)
		}
		filtered := translateFrame(t, execGrep, argsFrame(
			map[int]string{grepPattern: "todo", grepPath: "/src", grepGlob: "*.go"},
			map[int]int64{grepCaseInsensitive: 1, grepHeadLimit: 5}), "exec-2")
		args = filtered.arguments("grep")
		if args["include"] != "*.go" || args["caseSensitive"] != false || args["limit"] != int64(5) {
			t.Fatalf("arguments = %v, want the glob, the case flag, and the limit", args)
		}
	})

	t.Run("the path-only variants carry their path", func(t *testing.T) {
		for _, variant := range []int{execDelete, execLs, execDiagnostics} {
			call := translateFrame(t, variant, argsFrame(map[int]string{1: "/src"}, nil), "exec-1")
			if args := call.arguments("delete"); args["path"] != "/src" || len(args) != 1 {
				t.Fatalf("variant %d arguments = %v, want the path alone", variant, args)
			}
		}
	})
}

// TestShellDeadlineFollowsTheDeclaredTool pins the two clocks: the shell tool
// takes milliseconds, and the bash tool takes seconds and only treats a wait
// past its own default as a kill deadline.
func TestShellDeadlineFollowsTheDeclaredTool(t *testing.T) {
	parse := func(args []byte) []adapterscursor.Field {
		t.Helper()
		fields, err := adapterscursor.Parse(args)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return fields
	}
	cases := []struct {
		name   string
		fields []int64
		millis bool
		want   int64
		ok     bool
	}{
		{name: "shell hard timeout", fields: []int64{shellHardTimeout, 5000}, millis: true, want: 5000, ok: true},
		{name: "shell wait", fields: []int64{shellTimeout, 1000}, millis: true, want: 1000, ok: true},
		{name: "shell without timing", fields: nil, millis: true},
		{name: "bash hard timeout in seconds", fields: []int64{shellHardTimeout, 4500}, want: 5, ok: true},
		{name: "bash wait past its default", fields: []int64{shellTimeout, 400000}, want: 400, ok: true},
		{name: "bash wait inside its default", fields: []int64{shellTimeout, 100000}},
		{name: "bash hard timeout of zero", fields: []int64{shellHardTimeout, 0}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var fields []adapterscursor.Field
			if len(test.fields) == 2 {
				fields = parse(argsFrame(nil, map[int]int64{int(test.fields[0]): test.fields[1]}))
			}
			got, ok := shellDeadline(fields, test.millis)
			if got != test.want || ok != test.ok {
				t.Fatalf("shellDeadline() = %d %v, want %d %v", got, ok, test.want, test.ok)
			}
		})
	}
	if value, ok := seconds(400); !ok || value != 1 {
		t.Fatalf("seconds(400) = %d %v, want the one second floor", value, ok)
	}
	if _, ok := seconds(0); ok {
		t.Fatal("seconds(0) reported a deadline")
	}
}

// TestShellArgumentsCarryTheDeadline pins the timeout key a shell call gains
// from Cursor's timing, in the unit each declared tool expects.
func TestShellArgumentsCarryTheDeadline(t *testing.T) {
	call := translateFrame(t, execShell, argsFrame(
		map[int]string{shellCommand: "make", shellWorkingDir: "/src"},
		map[int]int64{shellHardTimeout: 9000}), "exec-1")
	if args := call.arguments("bash"); args["timeout"] != int64(9) {
		t.Fatalf("arguments = %v, want the deadline in seconds", args)
	}
	if args := call.arguments("shell"); args["timeout"] != int64(9000) {
		t.Fatalf("arguments = %v, want the deadline in milliseconds", args)
	}
}

// TestToolCallVariantsCoverEveryDeclaredCall pins the update variants that map
// onto a native execution the client can run.
func TestToolCallVariantsCoverEveryDeclaredCall(t *testing.T) {
	cases := map[int]int{
		callShell: execShell, callDelete: execDelete,
		callGrep: execGrep, callRead: execRead, callLs: execLs,
	}
	for number, want := range cases {
		variant, known := toolCallVariant(number)
		if !known || variant != want {
			t.Fatalf("toolCallVariant(%d) = %d %v, want %d", number, variant, known, want)
		}
	}
	if _, known := toolCallVariant(99); known {
		t.Fatal("toolCallVariant() accepted an unknown update")
	}
}

// TestNativeHandoffAnswersTheWaitingVariant pins the answer each handed-off
// execution returns, so the server is never left blocked on one Relo moved to
// the client.
func TestNativeHandoffAnswersTheWaitingVariant(t *testing.T) {
	cases := []struct {
		name    string
		variant int
		args    []byte
		result  int
		inner   int
		// textField is where the variant carries its message: the shell and
		// grep variants hold it in the reason, the path variants beside the
		// path.
		textField int
	}{
		{name: "shell", variant: execShell, args: argsFrame(map[int]string{shellCommand: "ls"}, nil), result: resultShell, inner: rejectedNoShell, textField: 3},
		{name: "streamed shell", variant: execShellStream, args: argsFrame(map[int]string{shellCommand: "ls"}, nil), result: resultShell, inner: rejectedNoShell, textField: 3},
		{name: "grep", variant: execGrep, args: argsFrame(map[int]string{grepPattern: "x"}, nil), result: resultGrep, inner: grepErrorNo, textField: 1},
		{name: "write", variant: execWrite, args: argsFrame(map[int]string{writePath: "/a"}, nil), result: resultWrite, inner: rejectedNoWrite, textField: 2},
		{name: "delete", variant: execDelete, args: argsFrame(map[int]string{deletePath: "/a"}, nil), result: resultDelete, inner: rejectedNoDel, textField: 2},
		{name: "ls", variant: execLs, args: argsFrame(map[int]string{lsPath: "/a"}, nil), result: resultLs, inner: rejectedNoLs, textField: 2},
		{name: "diagnostics", variant: execDiagnostics, args: argsFrame(map[int]string{diagnosticsPath: "/a"}, nil), result: resultDiag, inner: rejectedNoDiag, textField: 2},
		{name: "read", variant: execRead, args: argsFrame(map[int]string{readPath: "/a"}, nil), result: resultRead, inner: rejectedNoRead, textField: 2},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			call := translateFrame(t, test.variant, test.args, "exec-1")
			field, payload := buildNativeHandoff(call)
			if field != test.result {
				t.Fatalf("result field = %d, want %d", field, test.result)
			}
			nested, err := adapterscursor.Nested(mustParseForTest(t, payload), test.inner)
			if err != nil || len(nested) != 1 {
				t.Fatalf("handoff = %v %v, want the %d rejection", nested, err, test.inner)
			}
			if reason := adapterscursor.FirstString(nested[0], test.textField); reason != HandoffText {
				t.Fatalf("reason = %q, want the handoff text", reason)
			}
		})
	}
}

// TestParseNativeToolCallRefusesWhatItCannotRead covers the updates that are
// not a native call: a malformed payload, an unknown variant, and a variant
// whose args do not nest where the shell's do.
func TestParseNativeToolCallRefusesWhatItCannotRead(t *testing.T) {
	t.Run("a malformed payload is an error", func(t *testing.T) {
		if _, _, err := ParseNativeToolCall([]byte{0x08}); err == nil {
			t.Fatal("ParseNativeToolCall() accepted a truncated payload")
		}
	})

	t.Run("a shell update whose args do not parse is an error", func(t *testing.T) {
		var call adapterscursor.Writer
		call.Bytes(callShell, []byte{0x08})
		if _, _, err := ParseNativeToolCall(call.BytesOut()); err == nil {
			t.Fatal("ParseNativeToolCall() accepted unreadable shell args")
		}
	})

	t.Run("an unknown variant and its envelope fields are skipped", func(t *testing.T) {
		var call adapterscursor.Writer
		call.Uint32(callIDField, 7)
		call.Message(99, argsFrame(map[int]string{1: "x"}, nil))
		if _, ok, err := ParseNativeToolCall(call.BytesOut()); ok || err != nil {
			t.Fatalf("ParseNativeToolCall() = %v %v, want neither a call nor an error", ok, err)
		}
	})

	// A non-shell variant carries no wrapper field 1 for the args to be read
	// out of, so the update is left to the execution frame that follows it.
	t.Run("a variant without the shell's args wrapper reports no call", func(t *testing.T) {
		var call adapterscursor.Writer
		call.String(callIDField, "call-1")
		call.Message(callRead, argsFrame(map[int]string{readPath: "/a"}, nil))
		if _, ok, err := ParseNativeToolCall(call.BytesOut()); ok || err != nil {
			t.Fatalf("ParseNativeToolCall() = %v %v, want neither a call nor an error", ok, err)
		}
	})
}

// TestNativeFieldReaders covers the accessors the translation reads its
// arguments with: a field of the wrong wire type is not that field.
func TestNativeFieldReaders(t *testing.T) {
	var writer adapterscursor.Writer
	writer.String(1, "text")
	writer.Int64(2, 7)
	fields, err := adapterscursor.Parse(writer.BytesOut())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if value, ok := intField(fields, 2); !ok || value != 7 {
		t.Fatalf("intField() = %d %v, want the varint", value, ok)
	}
	if _, ok := intField(fields, 1); ok {
		t.Fatal("intField() read a length-delimited field as a varint")
	}
	if insensitive, ok := boolField(fields, 2); !ok || !insensitive {
		t.Fatalf("boolField() = %v %v, want a set flag", insensitive, ok)
	}
	if raw := bytesField(fields, 1); string(raw) != "text" {
		t.Fatalf("bytesField() = %q, want the bytes", raw)
	}
	if bytesField(fields, 9) != nil {
		t.Fatal("bytesField() invented a field")
	}
}

// TestNativeArgsJSONKeepsAnEncodingFailureOutOfTheWire covers an argument the
// encoder cannot render: the client gets an empty object rather than half of
// one.
func TestNativeArgsJSONKeepsAnEncodingFailureOutOfTheWire(t *testing.T) {
	encoded := nativeArgsJSON(map[string]any{"callback": func() {}})
	if encoded != "{}" {
		t.Fatalf("nativeArgsJSON() = %q, want an empty object", encoded)
	}
	if encoded := nativeArgsJSON(map[string]any{"path": "/a"}); !strings.Contains(encoded, `"path":"/a"`) {
		t.Fatalf("nativeArgsJSON() = %q, want the arguments", encoded)
	}
}
