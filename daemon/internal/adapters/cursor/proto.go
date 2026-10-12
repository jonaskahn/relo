// Minimal protobuf encoding for the verified Cursor subset.
package cursor

import (
	"errors"
	"fmt"
)

// Proto errors name the malformed inputs the decoder refuses.
var (
	ErrProtoTruncated = errors.New("truncated protobuf message")
	ErrProtoTooLarge  = errors.New("protobuf message exceeds the size limit")
	ErrProtoNesting   = errors.New("protobuf message nests too deeply")
	ErrProtoVarint    = errors.New("malformed protobuf varint")
)

// Wire types the Cursor subset uses; fixed types are skipped, never decoded.
const (
	wireVarint  = 0
	wireFixed   = 1
	wireLength  = 2
	wireFixed32 = 5

	maxProtoDepth = 16
)

// Writer encodes the Cursor message subset: strings, bytes, bools,
// varints, and nested messages.
type Writer struct {
	buf []byte
}

// BytesOut returns the encoded message.
func (w *Writer) BytesOut() []byte {
	return w.buf
}

func (w *Writer) tag(field, wire int) {
	w.varint(uint64(field<<3 | wire))
}

func (w *Writer) varint(value uint64) {
	for value >= 0x80 {
		w.buf = append(w.buf, byte(value)|0x80)
		value >>= 7
	}
	w.buf = append(w.buf, byte(value))
}

// String appends a string field, skipping it when empty.
func (w *Writer) String(field int, value string) {
	if value == "" {
		return
	}
	w.Bytes(field, []byte(value))
}

// Bytes appends a bytes field, skipping it when empty.
func (w *Writer) Bytes(field int, value []byte) {
	if len(value) == 0 {
		return
	}
	w.tag(field, wireLength)
	w.varint(uint64(len(value)))
	w.buf = append(w.buf, value...)
}

// Bool appends a bool field, skipping it when false.
func (w *Writer) Bool(field int, value bool) {
	if !value {
		return
	}
	w.tag(field, wireVarint)
	w.buf = append(w.buf, 1)
}

// Uint32 appends a uint32 field, skipping it when zero.
func (w *Writer) Uint32(field int, value uint32) {
	if value == 0 {
		return
	}
	w.tag(field, wireVarint)
	w.varint(uint64(value))
}

// Int64 appends an int64 field, skipping it when zero.
func (w *Writer) Int64(field int, value int64) {
	if value == 0 {
		return
	}
	w.tag(field, wireVarint)
	w.varint(uint64(value))
}

// Message appends a nested message field, skipping it when empty.
func (w *Writer) Message(field int, nested []byte) {
	w.Bytes(field, nested)
}

// Empty appends a present-but-empty nested message, which marks presence
// the way Cursor's empty thinking details do.
func (w *Writer) Empty(field int) {
	w.tag(field, wireLength)
	w.buf = append(w.buf, 0)
}

// Field is one decoded protobuf field.
type Field struct {
	Number int
	Wire   int
	Varint uint64
	Raw    []byte
}

// Parse decodes one message into its fields, skipping nothing: callers
// read what they need and ignore the rest.
func Parse(body []byte) ([]Field, error) {
	if len(body) > MaxProtoBytes {
		return nil, ErrProtoTooLarge
	}
	fields := make([]Field, 0, 8)
	for len(body) > 0 {
		field, rest, err := parseField(body)
		if err != nil {
			return nil, err
		}
		body = rest
		fields = append(fields, field)
	}
	return fields, nil
}

func parseField(body []byte) (Field, []byte, error) {
	field, wire, rest, err := readTag(body)
	if err != nil {
		return Field{}, nil, err
	}
	switch wire {
	case wireVarint:
		value, rest, err := readVarint(rest)
		if err != nil {
			return Field{}, nil, err
		}
		return Field{Number: field, Wire: wire, Varint: value}, rest, nil
	case wireLength:
		return parseDelimited(field, wire, rest)
	case wireFixed, wireFixed32:
		return parseFixed(field, wire, rest)
	default:
		return Field{}, nil, fmt.Errorf("unknown wire type %d", wire)
	}
}

func parseDelimited(field, wire int, body []byte) (Field, []byte, error) {
	size, rest, err := readVarint(body)
	if err != nil {
		return Field{}, nil, err
	}
	if size > uint64(MaxProtoBytes) {
		return Field{}, nil, ErrProtoTooLarge
	}
	if uint64(len(rest)) < size {
		return Field{}, nil, ErrProtoTruncated
	}
	return Field{Number: field, Wire: wire, Raw: rest[:size]}, rest[size:], nil
}

func parseFixed(field, wire int, body []byte) (Field, []byte, error) {
	width := 8
	if wire == wireFixed32 {
		width = 4
	}
	if len(body) < width {
		return Field{}, nil, ErrProtoTruncated
	}
	return Field{Number: field, Wire: wire, Raw: body[:width]}, body[width:], nil
}

func readTag(body []byte) (int, int, []byte, error) {
	tag, rest, err := readVarint(body)
	if err != nil {
		return 0, 0, nil, err
	}
	field := int(tag >> 3)
	if field <= 0 {
		return 0, 0, nil, ErrProtoVarint
	}
	return field, int(tag & 7), rest, nil
}

func readVarint(body []byte) (uint64, []byte, error) {
	var value uint64
	for shift := uint(0); ; shift += 7 {
		if len(body) == 0 {
			return 0, nil, ErrProtoTruncated
		}
		if shift >= 64 {
			return 0, nil, ErrProtoVarint
		}
		next := body[0]
		body = body[1:]
		value |= uint64(next&0x7f) << shift
		if next&0x80 == 0 {
			return value, body, nil
		}
	}
}

// Strings returns every string value of one length-delimited field.
func Strings(fields []Field, number int) []string {
	out := make([]string, 0, 1)
	for _, field := range fields {
		if field.Number == number && field.Wire == wireLength {
			out = append(out, string(field.Raw))
		}
	}
	return out
}

// FirstString returns the first string value of one field.
func FirstString(fields []Field, number int) string {
	values := Strings(fields, number)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Nested returns every nested message of one field, parsed one level down.
func Nested(fields []Field, number int) ([][]Field, error) {
	return NestedDepth(fields, number, 0)
}

// NestedDepth parses nested messages with an explicit depth budget, so a
// hostile listing cannot recurse without bound.
func NestedDepth(fields []Field, number, depth int) ([][]Field, error) {
	if depth >= maxProtoDepth {
		return nil, ErrProtoNesting
	}
	out := make([][]Field, 0, 1)
	for _, field := range fields {
		if field.Number != number || field.Wire != wireLength {
			continue
		}
		parsed, err := Parse(field.Raw)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}

// Flag returns the bool value of one varint field.
func Flag(fields []Field, number int) bool {
	for _, field := range fields {
		if field.Number == number && field.Wire == wireVarint {
			return field.Varint != 0
		}
	}
	return false
}

// Uint returns the varint value of one field, or zero when absent.
func Uint(fields []Field, number int) uint64 {
	for _, field := range fields {
		if field.Number == number && field.Wire == wireVarint {
			return field.Varint
		}
	}
	return 0
}
