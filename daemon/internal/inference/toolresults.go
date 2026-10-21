// One result per call, which is what every upstream expects.
package inference

// MergeToolResults folds the results a client sent for one call into a single
// result, so the canonical form holds one result per call.
//
// A client may answer the same call more than once. Codex sends a status line
// and then the output of the same exec call, and an upstream that takes
// exactly one result per call refuses the request. The results are joined in
// arrival order, so nothing the client sent is lost.
//
// The first result keeps its place in the list, which is where it belongs:
// right after the call that made it. Results for different calls are untouched,
// and a result with no call id is never folded into another, because two
// unrelated results must not collapse into one.
//
// The input is not modified. The canonical request's messages are shared
// between route candidates, so appending to a caller's slice would corrupt a
// sibling attempt.
func MergeToolResults(messages []Message) []Message {
	merged := make([]Message, 0, len(messages))
	positions := make(map[string]int, len(messages))
	for _, message := range messages {
		if message.Role != RoleTool || message.ToolCallID == "" {
			merged = append(merged, message)
			continue
		}
		position, found := positions[message.ToolCallID]
		if !found {
			positions[message.ToolCallID] = len(merged)
			merged = append(merged, message)
			continue
		}
		content := make([]ContentPart, 0, len(merged[position].Content)+len(message.Content))
		content = append(content, merged[position].Content...)
		content = append(content, message.Content...)
		merged[position].Content = content
	}
	return merged
}
