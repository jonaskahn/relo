package codingclients

import (
	"strings"
	"testing"
)

func TestSpliceKeyBuildsNestedObjectFromEmpty(t *testing.T) {
	got, err := spliceKey("", []string{"a", "b"}, `1`)
	if err != nil {
		t.Fatalf("spliceKey() error = %v", err)
	}
	if !jsonEqual(got, `{"a":{"b":1}}`) {
		t.Fatalf("spliceKey() = %s", got)
	}
}

func TestSpliceKeyRejectsUnusableInput(t *testing.T) {
	cases := map[string]string{
		"invalid JSON":   `{oops`,
		"array root":     `[1,2]`,
		"empty path":     `{}`,
		"nested in leaf": `{"a":1}`,
	}
	paths := map[string][]string{
		"invalid JSON":   {"a"},
		"array root":     {"a"},
		"empty path":     {},
		"nested in leaf": {"a", "b"},
	}
	raws := map[string]string{
		"invalid JSON":   `1`,
		"array root":     `1`,
		"empty path":     `1`,
		"nested in leaf": `1`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := spliceKey(content, paths[name], raws[name]); err == nil {
				t.Fatalf("spliceKey(%q) succeeded, want error", content)
			}
		})
	}
}

func TestSpliceKeyReplacesOrInsertsMembers(t *testing.T) {
	cases := []struct {
		name    string
		content string
		path    []string
		raw     string
		want    string
	}{
		{"replace first", `{"a":1,"b":2}`, []string{"a"}, `9`, `{"a":9,"b":2}`},
		{"replace middle", `{"a":1,"b":2,"c":3}`, []string{"b"}, `9`, `{"a":1,"b":9,"c":3}`},
		{"replace last", `{"a":1,"b":2}`, []string{"b"}, `9`, `{"a":1,"b":9}`},
		{"insert into empty", `{}`, []string{"a"}, `1`, `{"a":1}`},
		{"insert new key", `{"a":1}`, []string{"b"}, `2`, `{"a":1,"b":2}`},
		{"insert nested", `{"a":{}}`, []string{"a", "b"}, `2`, `{"a":{"b":2}}`},
		{"create nested path", `{}`, []string{"a", "b"}, `2`, `{"a":{"b":2}}`},
		{"replace nested", `{"a":{"b":1,"c":2}}`, []string{"a", "b"}, `9`, `{"a":{"b":9,"c":2}}`},
		{"escaped key", `{"a\"b":1}`, []string{`a"b`}, `2`, `{"a\"b":2}`},
		{"whitespace", "{\n  \"a\" : 1\n}", []string{"b"}, `2`, "{\n  \"a\" : 1\n,\"b\":2}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := spliceKey(tc.content, tc.path, tc.raw)
			if err != nil {
				t.Fatalf("spliceKey() error = %v", err)
			}
			if !jsonEqual(got, tc.want) {
				t.Fatalf("spliceKey() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDeleteKeyRemovesMembersAndPrunesEmpties(t *testing.T) {
	cases := []struct {
		name    string
		content string
		path    []string
		want    string
	}{
		{"missing key", `{"a":1}`, []string{"b"}, `{"a":1}`},
		{"only member", `{"a":1}`, []string{"a"}, `{}`},
		{"first of two", `{"a":1,"b":2}`, []string{"a"}, `{"b":2}`},
		{"last of two", `{"a":1,"b":2}`, []string{"b"}, `{"a":1}`},
		{"middle of three", `{"a":1,"b":2,"c":3}`, []string{"b"}, `{"a":1,"c":3}`},
		{"nested keeps parent", `{"a":{"b":1,"c":2}}`, []string{"a", "b"}, `{"a":{"c":2}}`},
		{"nested prunes empty", `{"a":{"b":1},"c":2}`, []string{"a", "b"}, `{"c":2}`},
		{"missing nested", `{"a":{"b":1}}`, []string{"a", "z"}, `{"a":{"b":1}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deleteKey(tc.content, tc.path)
			if err != nil {
				t.Fatalf("deleteKey() error = %v", err)
			}
			if !jsonEqual(got, tc.want) {
				t.Fatalf("deleteKey() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDeleteKeyHandlesEdgeInput(t *testing.T) {
	if got, err := deleteKey("", []string{"a"}); err != nil || got != "" {
		t.Fatalf("deleteKey(empty) = %q, %v", got, err)
	}
	for name, content := range map[string]string{"invalid": `{oops`, "array": `[1]`, "nested leaf": `{"a":1}`} {
		path := []string{"a"}
		if name == "nested leaf" {
			path = []string{"a", "b"}
		}
		if _, err := deleteKey(content, path); err == nil {
			t.Fatalf("deleteKey(%s) succeeded, want error", name)
		}
	}
	if _, err := deleteKey(`{"a":1}`, []string{}); err != nil {
		t.Fatalf("deleteKey(empty path) error = %v", err)
	}
}

func TestJSONReadersRoundTripSplicedContent(t *testing.T) {
	content, err := spliceKey(`{"a":{"b":1}}`, []string{"a", "c"}, `[1,{"d":null}, "x"]`)
	if err != nil {
		t.Fatalf("spliceKey() error = %v", err)
	}
	raw, ok, err := jsonRaw(content, []string{"a", "c"})
	if err != nil || !ok {
		t.Fatalf("jsonRaw() = %q, %v, %v", raw, ok, err)
	}
	if !jsonEqual(raw, `[1,{"d":null}, "x"]`) {
		t.Fatalf("jsonRaw() = %s", raw)
	}
	for _, path := range [][]string{{"a", "c"}, {"a"}} {
		has, err := jsonHas(content, path)
		if err != nil || !has {
			t.Fatalf("jsonHas(%v) = %v, %v", path, has, err)
		}
	}
	if has, _ := jsonHas(content, []string{"missing"}); has {
		t.Fatal("jsonHas(missing) = true")
	}
	if _, ok, _ := jsonRaw(content, []string{"missing"}); ok {
		t.Fatal("jsonRaw(missing) found")
	}
}

func TestJSONReadersRejectUnusableInput(t *testing.T) {
	if _, _, err := jsonRaw(`{oops`, []string{"a"}); err == nil {
		t.Fatal("jsonRaw(invalid) succeeded")
	}
	if _, _, err := jsonRaw(`[1]`, []string{"a"}); err == nil {
		t.Fatal("jsonRaw(array) succeeded")
	}
	if _, err := jsonHas(`{oops`, []string{"a"}); err == nil {
		t.Fatal("jsonHas(invalid) succeeded")
	}
	if _, err := jsonHas(`[1]`, []string{"a"}); err == nil {
		t.Fatal("jsonHas(array) succeeded")
	}
	if jsonEqual(`{oops`, `{"a":1}`) || jsonEqual(`{"a":1}`, `{oops`) {
		t.Fatal("jsonEqual(invalid) = true")
	}
	if !jsonEqual(`{"a":1,"b":[2]}`, `{"b":[2],"a":1}`) {
		t.Fatal("jsonEqual(key order) = false")
	}
}

func TestJSONHelpersCoverSmallBranches(t *testing.T) {
	if nest([]string{"a"}, `1`) != `{"a":1}` {
		t.Fatal("nest() mismatch")
	}
	if got := insertMember(`{}`, member{close: 1, empty: true}, "b", `2`); !jsonEqual(got, `{"b":2}`) {
		t.Fatalf("insertMember() = %s", got)
	}
	if !emptyObject(`{}`) || emptyObject(`{"a":1}`) || emptyObject(`oops`) {
		t.Fatal("emptyObject() mismatch")
	}
	if jsonString("a\"b") != `"a\"b"` {
		t.Fatalf("jsonString() = %s", jsonString("a\"b"))
	}
	if !isBoundary(',') || !isBoundary('}') || !isBoundary(' ') || isBoundary('x') {
		t.Fatal("isBoundary() mismatch")
	}
	if _, err := unquoteJSON(`"abc`); err == nil {
		t.Fatal("unquoteJSON(unterminated) succeeded")
	}
	if _, err := parseString(`{"a":1}`, 0); err == nil {
		t.Fatal("parseString(non-string) succeeded")
	}
	if _, err := parseString(`"abc`, 0); err == nil {
		t.Fatal("parseString(unterminated) succeeded")
	}
	if _, err := parseValue("", 0); err == nil {
		t.Fatal("parseValue(empty) succeeded")
	}
	if _, err := parseValue(`{`, 0); err == nil {
		t.Fatal("parseValue(lone brace) succeeded")
	}
	if end, err := parseValue(`123,`, 0); err != nil || end != 3 {
		t.Fatalf("parseValue(number) = %d, %v", end, err)
	}
	if _, _, err := parseContainerEntry(`{"a" 1}`, 1, '{', '}'); err == nil {
		t.Fatal("parseContainerEntry(missing colon) succeeded")
	}
	if _, _, err := parseContainerEntry(`[1 2]`, 1, '[', ']'); err == nil {
		t.Fatal("parseContainerEntry(missing comma) succeeded")
	}
	if skipSpace("   ", 0) != 3 || skipSpace("a", 5) != 5 {
		t.Fatal("skipSpace() mismatch")
	}
	if _, err := findMember(`{"a":1}`, 1, "a"); err == nil {
		t.Fatal("findMember(non-object start) succeeded")
	}
	if _, err := findMember(`{"a":`, 0, "a"); err == nil {
		t.Fatal("findMember(truncated) succeeded")
	}
	if !strings.Contains(jsonString("\xff"), "\\") {
		t.Fatal("jsonString() did not escape")
	}
}
