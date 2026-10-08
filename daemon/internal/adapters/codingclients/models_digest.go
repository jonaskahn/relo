// Model digest: the compact roster embedded in configs.
package codingclients

import (
	"strconv"
	"strings"
)

// ModelsDigest is the fingerprint of a published model list. An integration
// stores it at setup and repair, and Inspect compares it to the list Relo
// would write now, so a catalog change can offer Repair without rewriting
// a client Relo does not configure.
func ModelsDigest(models []ModelRef) string {
	var builder strings.Builder
	for _, model := range orderedModels(models) {
		builder.WriteString(model.ID)
		builder.WriteByte('\n')
		builder.WriteString(model.Name)
		builder.WriteByte('\n')
		builder.WriteString(model.Connection)
		builder.WriteByte('\n')
		if model.ContextWindow != nil {
			builder.WriteString(strconv.FormatInt(*model.ContextWindow, 10))
		}
		builder.WriteByte('\n')
		writeDigestFlag(&builder, model.Tools)
		writeDigestFlag(&builder, model.Reasoning)
		writeDigestFlag(&builder, model.Vision)
		writeDigestEfforts(&builder, model.ReasoningEfforts)
	}
	return Digest(builder.String())
}

func writeDigestEfforts(builder *strings.Builder, efforts []string) {
	if efforts == nil {
		builder.WriteByte('-')
	} else {
		builder.WriteString(strings.Join(efforts, ","))
	}
	builder.WriteByte('\n')
}

func writeDigestFlag(builder *strings.Builder, flag *bool) {
	switch {
	case flag == nil:
		builder.WriteByte('?')
	case *flag:
		builder.WriteByte('1')
	default:
		builder.WriteByte('0')
	}
	builder.WriteByte('\n')
}
