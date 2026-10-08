// JSON splicing: key insertion and deletion by path.
package codingclients

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type member struct {
	found      bool
	keyStart   int
	valueStart int
	valueEnd   int
	// comma is the comma that belongs to this member: before it when another
	// member precedes it, after it when it is first and a sibling follows.
	comma int
	first bool
	close int
	empty bool
}

func spliceKey(content string, path []string, raw string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return nest(path, raw) + "\n", nil
	}
	if !json.Valid([]byte(content)) {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	start := skipSpace(content, 0)
	if start >= len(content) || content[start] != '{' {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	return spliceAt(content, start, path, raw)
}

func spliceAt(content string, objStart int, path []string, raw string) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	loc, err := findMember(content, objStart, path[0])
	if err != nil {
		return "", err
	}
	if len(path) == 1 {
		if loc.found {
			return content[:loc.valueStart] + raw + content[loc.valueEnd:], nil
		}
		return insertMember(content, loc, path[0], raw), nil
	}
	if !loc.found {
		return insertMember(content, loc, path[0], nest(path[1:], raw)), nil
	}
	valueAt := skipSpace(content, loc.valueStart)
	if valueAt >= len(content) || content[valueAt] != '{' {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	return spliceAt(content, valueAt, path[1:], raw)
}

func deleteKey(content string, path []string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return content, nil
	}
	if !json.Valid([]byte(content)) {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	start := skipSpace(content, 0)
	if start >= len(content) || content[start] != '{' {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	updated, err := deleteAt(content, start, path)
	if err != nil {
		return "", err
	}
	return pruneEmpty(updated, path[:len(path)-1])
}

func deleteAt(content string, objStart int, path []string) (string, error) {
	if len(path) == 0 {
		return content, nil
	}
	loc, err := findMember(content, objStart, path[0])
	if err != nil {
		return "", err
	}
	if !loc.found {
		return content, nil
	}
	if len(path) == 1 {
		return removeMember(content, loc), nil
	}
	valueAt := skipSpace(content, loc.valueStart)
	if valueAt >= len(content) || content[valueAt] != '{' {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	return deleteAt(content, valueAt, path[1:])
}

func pruneEmpty(content string, path []string) (string, error) {
	if len(path) == 0 || strings.TrimSpace(content) == "" {
		return content, nil
	}
	start := skipSpace(content, 0)
	loc, err := memberAt(content, start, path)
	if err != nil || !loc.found {
		return content, err
	}
	if !emptyObject(content[loc.valueStart:loc.valueEnd]) {
		return content, nil
	}
	removed := removeMember(content, loc)
	return pruneEmpty(removed, path[:len(path)-1])
}

func memberAt(content string, objStart int, path []string) (member, error) {
	loc, err := findMember(content, objStart, path[0])
	if err != nil || !loc.found || len(path) == 1 {
		return loc, err
	}
	valueAt := skipSpace(content, loc.valueStart)
	if valueAt >= len(content) || content[valueAt] != '{' {
		return member{}, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	return memberAt(content, valueAt, path[1:])
}

func jsonRaw(content string, path []string) (string, bool, error) {
	if strings.TrimSpace(content) == "" {
		return "", false, nil
	}
	if !json.Valid([]byte(content)) {
		return "", false, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	start := skipSpace(content, 0)
	if start >= len(content) || content[start] != '{' {
		return "", false, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	loc, err := memberAt(content, start, path)
	if err != nil || !loc.found {
		return "", false, err
	}
	return content[loc.valueStart:loc.valueEnd], true, nil
}

func jsonEqual(left, right string) bool {
	var got, want any
	if json.Unmarshal([]byte(left), &got) != nil || json.Unmarshal([]byte(right), &want) != nil {
		return false
	}
	return reflect.DeepEqual(got, want)
}

func jsonHas(content string, path []string) (bool, error) {
	if strings.TrimSpace(content) == "" {
		return false, nil
	}
	if !json.Valid([]byte(content)) {
		return false, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	start := skipSpace(content, 0)
	if start >= len(content) || content[start] != '{' {
		return false, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	loc, err := memberAt(content, start, path)
	if err != nil {
		return false, err
	}
	return loc.found, nil
}

func nest(path []string, raw string) string {
	value := raw
	for index := len(path) - 1; index >= 0; index-- {
		value = `{"` + path[index] + `":` + value + `}`
	}
	return value
}

func insertMember(content string, loc member, key, raw string) string {
	piece := `"` + key + `":` + raw
	if loc.empty {
		return content[:loc.close] + piece + content[loc.close:]
	}
	return content[:loc.close] + "," + piece + content[loc.close:]
}

func removeMember(content string, loc member) string {
	start, end := loc.keyStart, loc.valueEnd
	if loc.comma >= 0 {
		if loc.first {
			end = loc.comma + 1
		} else {
			start = loc.comma
		}
	}
	return content[:start] + content[end:]
}

func findMember(content string, objStart int, key string) (member, error) {
	if objStart >= len(content) || content[objStart] != '{' {
		return member{}, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	scan := &memberScan{content: content, key: key, loc: member{comma: -1, close: -1}, prevComma: -1, i: objStart + 1}
	for {
		done, loc, err := scan.step()
		if done || err != nil {
			return loc, err
		}
	}
}

type memberScan struct {
	content   string
	key       string
	loc       member
	seen      bool
	prevComma int
	i         int
}

func (s *memberScan) step() (bool, member, error) {
	s.i = skipSpace(s.content, s.i)
	if s.i >= len(s.content) {
		return true, member{}, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	if s.content[s.i] == '}' {
		s.loc.close = s.i
		s.loc.empty = !s.seen
		return true, s.loc, nil
	}
	return s.scanEntry()
}

func parseScanKey(content string, i int) (int, int, string, error) {
	keyStart := i
	keyEnd, err := parseString(content, i)
	if err != nil {
		return 0, 0, "", err
	}
	name, err := unquoteJSON(content[keyStart:keyEnd])
	if err != nil {
		return 0, 0, "", err
	}
	return keyStart, keyEnd, name, nil
}

func (s *memberScan) scanEntry() (bool, member, error) {
	content := s.content
	keyStart, keyEnd, name, err := parseScanKey(content, s.i)
	if err != nil {
		return true, member{}, err
	}
	valueStart, valueEnd, err := s.scanEntryValue(content, keyEnd)
	if err != nil {
		return true, member{}, err
	}
	if name == s.key {
		return true, s.foundEntry(keyStart, valueStart, valueEnd, trailingComma(content, valueEnd)), nil
	}
	return s.skipEntry(content, valueEnd)
}

func (s *memberScan) scanEntryValue(content string, keyEnd int) (int, int, error) {
	s.i = skipSpace(content, keyEnd)
	if s.i >= len(content) || content[s.i] != ':' {
		return 0, 0, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	valueStart := skipSpace(content, s.i+1)
	valueEnd, err := parseValue(content, valueStart)
	if err != nil {
		return 0, 0, err
	}
	return valueStart, valueEnd, nil
}

func trailingComma(content string, valueEnd int) int {
	after := skipSpace(content, valueEnd)
	if after < len(content) && content[after] == ',' {
		return after
	}
	return -1
}

func (s *memberScan) skipEntry(content string, valueEnd int) (bool, member, error) {
	s.seen = true
	after := skipSpace(content, valueEnd)
	if trailing := trailingComma(content, valueEnd); trailing >= 0 {
		s.prevComma = trailing
		s.i = trailing + 1
		return false, member{}, nil
	}
	if after < len(content) && content[after] == '}' {
		s.loc.close = after
		return true, s.loc, nil
	}
	return true, member{}, fmt.Errorf("%w", ErrUnrecognisedFile)
}

func (s *memberScan) foundEntry(keyStart, valueStart, valueEnd, trailing int) member {
	s.loc.found = true
	s.loc.keyStart = keyStart
	s.loc.valueStart = valueStart
	s.loc.valueEnd = valueEnd
	s.loc.first = !s.seen
	s.loc.comma = trailing
	if !s.loc.first {
		s.loc.comma = s.prevComma
	}
	return s.loc
}

func skipSpace(content string, i int) int {
	for i < len(content) {
		switch content[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func parseString(content string, i int) (int, error) {
	if i >= len(content) || content[i] != '"' {
		return 0, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	for i = i + 1; i < len(content); i++ {
		if content[i] == '\\' {
			i++
			continue
		}
		if content[i] == '"' {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("%w", ErrUnrecognisedFile)
}

func parseValue(content string, i int) (int, error) {
	if i >= len(content) {
		return 0, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	switch content[i] {
	case '"':
		return parseString(content, i)
	case '{':
		return parseContainer(content, i, '{', '}')
	case '[':
		return parseContainer(content, i, '[', ']')
	default:
		end := i
		for end < len(content) && !isBoundary(content[end]) {
			end++
		}
		if end == i {
			return 0, fmt.Errorf("%w", ErrUnrecognisedFile)
		}
		return end, nil
	}
}

func parseContainer(content string, i int, open, close byte) (int, error) {
	i++
	for {
		next, done, err := parseContainerEntry(content, i, open, close)
		if done || err != nil {
			return next, err
		}
		i = next
	}
}

func parseContainerEntry(content string, i int, open, close byte) (int, bool, error) {
	i = skipSpace(content, i)
	if i >= len(content) {
		return 0, true, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	if content[i] == close {
		return i + 1, true, nil
	}
	if open == '{' {
		var err error
		if i, err = parseContainerKey(content, i); err != nil {
			return 0, true, err
		}
	}
	end, err := parseValue(content, i)
	if err != nil {
		return 0, true, err
	}
	i = skipSpace(content, end)
	if i < len(content) && content[i] == ',' {
		return i + 1, false, nil
	}
	if i < len(content) && content[i] == close {
		return i + 1, true, nil
	}
	return 0, true, fmt.Errorf("%w", ErrUnrecognisedFile)
}

func parseContainerKey(content string, i int) (int, error) {
	end, err := parseString(content, i)
	if err != nil {
		return 0, err
	}
	i = skipSpace(content, end)
	if i >= len(content) || content[i] != ':' {
		return 0, fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	i++
	return skipSpace(content, i), nil
}

func isBoundary(value byte) bool {
	switch value {
	case ',', '}', ']', ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func unquoteJSON(raw string) (string, error) {
	value, err := strconv.Unquote(raw)
	if err != nil {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	return value, nil
}

func emptyObject(raw string) bool {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return false
	}
	object, ok := value.(map[string]any)
	return ok && len(object) == 0
}

func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return strconv.Quote(value)
	}
	return string(encoded)
}
