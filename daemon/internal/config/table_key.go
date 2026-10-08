// TOML table keys: upserting one value in place.
package config

import (
	"regexp"
	"strings"
)

func upsertTableKey(source, table, key, value string) string {
	headerPattern := regexp.MustCompile(`^\s*\[` + regexp.QuoteMeta(table) + `\]\s*(?:#.*)?$`)
	keyPattern := regexp.MustCompile(
		`^(\s*)` + regexp.QuoteMeta(key) + `(\s*=\s*)[^#\r\n]*(\s*(?:#.*)?)(\r?\n?)$`)
	return rewriteTableKey(source, headerPattern, keyPattern, table, key, value)
}

type tableWrite struct {
	lines         []string
	header        *regexp.Regexp
	keyPattern    *regexp.Regexp
	inside, found bool
	wrote         bool
}

func rewriteTableKey(source string, header, keyPattern *regexp.Regexp, table, key, value string) string {
	lines := strings.SplitAfter(source, "\n")
	tracked := &tableWrite{lines: lines, header: header, keyPattern: keyPattern}
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r\n")
		if header.MatchString(trimmed) {
			tracked.enterTable()
			continue
		}
		if tracked.closeTable(i, line, key, value) {
			continue
		}
		tracked.replaceKey(line, i, key, value)
	}
	return tracked.finish(source, table, key, value)
}

func (t *tableWrite) enterTable() {
	t.inside, t.found = true, true
}

func (t *tableWrite) closeTable(i int, line, key, value string) bool {
	trimmed := strings.TrimRight(line, "\r\n")
	if !t.inside || !tableHeader.MatchString(trimmed) {
		return false
	}
	if !t.wrote {
		t.lines[i] = key + " = " + value + "\n" + line
		t.wrote = true
	}
	t.inside = false
	return true
}

func (t *tableWrite) replaceKey(line string, i int, key, value string) {
	if !t.inside || !t.keyPattern.MatchString(line) {
		return
	}
	ending := lineEnding(line)
	if ending == "" {
		ending = "\n"
	}
	t.lines[i] = key + " = " + value + ending
	t.wrote = true
}

func (t *tableWrite) finish(source, table, key, value string) string {
	body := strings.Join(t.lines, "")
	if t.found && t.wrote {
		return body
	}
	if t.inside && !t.wrote {
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		return body + key + " = " + value + "\n"
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + "\n[" + table + "]\n" + key + " = " + value + "\n"
}
