// The text view of a request, used to estimate what it costs to send.
package inference

// TextualView is the request a text tokenizer can measure: every image part
// keeps its place but loses its payload, and the number of images is returned
// separately so the caller can charge them what a provider charges.
//
// An image part carries a whole data URL. Feeding that to a text tokenizer
// measures base64 at about 1.5 characters per token, which is two orders of
// magnitude above the pixels a provider actually bills, so it must never be
// counted as text. The input is not modified: the canonical request is shared
// between route candidates.
func TextualView(request *Request) (*Request, int) {
	if request == nil {
		return nil, 0
	}
	images := 0
	messages := make([]Message, len(request.Messages))
	for index, message := range request.Messages {
		content := make([]ContentPart, len(message.Content))
		for partIndex, part := range message.Content {
			if part.Type == ContentTypeImage {
				images++
				part.ImageURL = ""
			}
			content[partIndex] = part
		}
		message.Content = content
		messages[index] = message
	}
	view := *request
	view.Messages = messages
	return &view, images
}
