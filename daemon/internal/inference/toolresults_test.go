package inference

import "testing"

func TestMergeToolResults(t *testing.T) {
	result := func(id, text string) Message {
		return Message{
			Role:       RoleTool,
			ToolCallID: id,
			Content:    []ContentPart{{Type: ContentTypeText, Text: text}},
		}
	}
	call := func(id string) Message {
		return Message{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: id, Name: "exec"}}}
	}
	texts := func(message Message) []string {
		parts := make([]string, 0, len(message.Content))
		for _, part := range message.Content {
			parts = append(parts, part.Text)
		}
		return parts
	}

	t.Run("two results for one call become one", func(t *testing.T) {
		merged := MergeToolResults([]Message{call("call_0"), result("call_0", "status"), result("call_0", "output")})
		if len(merged) != 2 {
			t.Fatalf("messages = %+v, want the call and one result", merged)
		}
		if got := texts(merged[1]); len(got) != 2 || got[0] != "status" || got[1] != "output" {
			t.Fatalf("result = %v, want both outputs in arrival order", got)
		}
	})

	t.Run("five results for one call become one", func(t *testing.T) {
		messages := []Message{call("call_0")}
		for _, text := range []string{"a", "b", "c", "d", "e"} {
			messages = append(messages, result("call_0", text))
		}
		merged := MergeToolResults(messages)
		if len(merged) != 2 {
			t.Fatalf("messages = %+v, want the call and one result", merged)
		}
		got := texts(merged[1])
		if len(got) != 5 {
			t.Fatalf("result = %v, want every output kept", got)
		}
		for index, want := range []string{"a", "b", "c", "d", "e"} {
			if got[index] != want {
				t.Fatalf("result = %v, want %q at %d", got, want, index)
			}
		}
	})

	t.Run("results for different calls are untouched", func(t *testing.T) {
		merged := MergeToolResults([]Message{call("call_0"), result("call_0", "one"), call("call_1"), result("call_1", "two")})
		if len(merged) != 4 {
			t.Fatalf("messages = %+v, want every message kept", merged)
		}
		if got := texts(merged[1]); len(got) != 1 || got[0] != "one" {
			t.Fatalf("result = %v, want its own output", got)
		}
		if got := texts(merged[3]); len(got) != 1 || got[0] != "two" {
			t.Fatalf("result = %v, want its own output", got)
		}
	})

	t.Run("a result that arrives late joins the first one", func(t *testing.T) {
		merged := MergeToolResults([]Message{
			call("call_0"), result("call_0", "status"),
			{Role: RoleUser, Content: []ContentPart{{Type: ContentTypeText, Text: "between"}}},
			result("call_0", "output"),
		})
		if len(merged) != 3 {
			t.Fatalf("messages = %+v, want the call, the result and the turn between", merged)
		}
		if got := texts(merged[1]); len(got) != 2 || got[0] != "status" || got[1] != "output" {
			t.Fatalf("result = %v, want both outputs on the first result", got)
		}
		if merged[2].Role != RoleUser {
			t.Fatalf("messages = %+v, want the turn between kept", merged)
		}
	})

	t.Run("results out of call order are untouched", func(t *testing.T) {
		merged := MergeToolResults([]Message{call("call_0"), call("call_1"), result("call_1", "one"), result("call_0", "zero")})
		if len(merged) != 4 {
			t.Fatalf("messages = %+v, want every message kept", merged)
		}
		if merged[2].ToolCallID != "call_1" || merged[3].ToolCallID != "call_0" {
			t.Fatalf("messages = %+v, want the results in place", merged)
		}
	})

	t.Run("a result with no call id is never folded in", func(t *testing.T) {
		merged := MergeToolResults([]Message{result("", "one"), result("", "two")})
		if len(merged) != 2 {
			t.Fatalf("messages = %+v, want two unrelated results", merged)
		}
	})

	t.Run("a result with no content is kept", func(t *testing.T) {
		merged := MergeToolResults([]Message{call("call_0"), result("call_0", "")})
		if len(merged) != 2 {
			t.Fatalf("messages = %+v, want the call and the result", merged)
		}
		if len(merged[1].Content) != 1 {
			t.Fatalf("result = %+v, want its content kept", merged[1])
		}
	})

	t.Run("the input is not modified", func(t *testing.T) {
		messages := []Message{call("call_0"), result("call_0", "status"), result("call_0", "output")}
		MergeToolResults(messages)
		if len(messages) != 3 {
			t.Fatalf("messages = %+v, want the input untouched", messages)
		}
		if got := texts(messages[1]); len(got) != 1 || got[0] != "status" {
			t.Fatalf("result = %v, want the input content untouched", got)
		}
	})
}
