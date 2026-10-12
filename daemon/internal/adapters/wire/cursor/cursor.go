// Package cursor translates canonical requests into the Cursor agent
// protocol and its responses back into canonical events. Tool execution
// stays with the calling client: native execution frames are refused, and
// declared tools are handed back as canonical tool calls.
package cursor
