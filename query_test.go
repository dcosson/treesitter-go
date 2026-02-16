package treesitter_test

import (
	"context"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	tg "github.com/treesitter-go/treesitter/internal/testgrammars"
)

func queryTestLanguage() *ts.Language {
	lang := tg.JSONLanguage()
	lang.LexFn = jsonLexFn
	return lang
}

func parseJSON(t *testing.T, input string) *ts.Tree {
	t.Helper()
	p := ts.NewParser()
	p.SetLanguage(queryTestLanguage())
	tree := p.ParseString(context.Background(), []byte(input))
	if tree == nil {
		t.Fatalf("failed to parse: %s", input)
	}
	return tree
}

// --- Query Compiler Tests ---

func TestQueryCompileSimple(t *testing.T) {
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, "(document)")
	if err != nil {
		t.Fatal(err)
	}
	if q.PatternCount() != 1 {
		t.Errorf("expected 1 pattern, got %d", q.PatternCount())
	}
}

func TestQueryCompileMultiplePatterns(t *testing.T) {
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `
		(document)
		(object)
		(array)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if q.PatternCount() != 3 {
		t.Errorf("expected 3 patterns, got %d", q.PatternCount())
	}
}

func TestQueryCompileCaptures(t *testing.T) {
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(pair (string) @key (number) @value)`)
	if err != nil {
		t.Fatal(err)
	}
	if q.CaptureCount() != 2 {
		t.Errorf("expected 2 captures, got %d", q.CaptureCount())
	}
	if q.CaptureNameForID(0) != "key" {
		t.Errorf("capture 0: expected 'key', got %q", q.CaptureNameForID(0))
	}
	if q.CaptureNameForID(1) != "value" {
		t.Errorf("capture 1: expected 'value', got %q", q.CaptureNameForID(1))
	}
}

func TestQueryCompileWildcard(t *testing.T) {
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(_ @node)`)
	if err != nil {
		t.Fatal(err)
	}
	if q.PatternCount() != 1 {
		t.Errorf("expected 1 pattern, got %d", q.PatternCount())
	}
	if q.CaptureCount() != 1 {
		t.Errorf("expected 1 capture, got %d", q.CaptureCount())
	}
}

func TestQueryCompileAlternation(t *testing.T) {
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `[(number) (null)] @literal`)
	if err != nil {
		t.Fatal(err)
	}
	if q.PatternCount() != 1 {
		t.Errorf("expected 1 pattern, got %d", q.PatternCount())
	}
	if q.CaptureCount() != 1 {
		t.Errorf("expected 1 capture, got %d", q.CaptureCount())
	}
}

func TestQueryCompilePredicates(t *testing.T) {
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `
		(string (string_content) @content
			(#eq? @content "hello"))
	`)
	if err != nil {
		t.Fatal(err)
	}
	preds := q.PredicatesForPattern(0)
	if len(preds) != 1 {
		t.Fatalf("expected 1 predicate, got %d", len(preds))
	}
	// Predicate should be: ["eq?" @content "hello"]
	pred := preds[0]
	if len(pred) != 3 {
		t.Fatalf("expected 3 predicate steps, got %d", len(pred))
	}
	if pred[0].Type != ts.PredicateStepString {
		t.Errorf("expected string step for predicate name")
	}
	if pred[1].Type != ts.PredicateStepCapture {
		t.Errorf("expected capture step for @content")
	}
	if pred[2].Type != ts.PredicateStepString {
		t.Errorf("expected string step for 'hello'")
	}
}

func TestQueryCompileComments(t *testing.T) {
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `
		; Match objects
		(object) @obj
		; Match arrays
		(array) @arr
	`)
	if err != nil {
		t.Fatal(err)
	}
	if q.PatternCount() != 2 {
		t.Errorf("expected 2 patterns, got %d", q.PatternCount())
	}
}

func TestQueryCompileErrorUnknownNodeType(t *testing.T) {
	lang := queryTestLanguage()
	_, err := ts.NewQuery(lang, `(nonexistent_type)`)
	if err == nil {
		t.Fatal("expected error for unknown node type")
	}
	qerr, ok := err.(*ts.QueryError)
	if !ok {
		t.Fatalf("expected QueryError, got %T", err)
	}
	if qerr.Type != ts.QueryErrorNodeType {
		t.Errorf("expected NodeType error, got %s", qerr.Type)
	}
}

func TestQueryCompileErrorEmptyQuery(t *testing.T) {
	lang := queryTestLanguage()
	_, err := ts.NewQuery(lang, "")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestQueryCompileErrorSyntax(t *testing.T) {
	lang := queryTestLanguage()
	_, err := ts.NewQuery(lang, "(")
	if err == nil {
		t.Fatal("expected error for incomplete pattern")
	}
}

// --- Query Cursor Tests ---

func TestQueryCursorMatchSimple(t *testing.T) {
	tree := parseJSON(t, `null`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(null) @value`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("expected a match")
	}
	if match.PatternIndex != 0 {
		t.Errorf("expected pattern 0, got %d", match.PatternIndex)
	}
	if len(match.Captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(match.Captures))
	}
	if match.Captures[0].Node.Type() != "null" {
		t.Errorf("expected capture of 'null', got %q", match.Captures[0].Node.Type())
	}
	if q.CaptureNameForID(match.Captures[0].Index) != "value" {
		t.Errorf("expected capture name 'value', got %q", q.CaptureNameForID(match.Captures[0].Index))
	}
}

func TestQueryCursorMatchNumber(t *testing.T) {
	tree := parseJSON(t, `42`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(number) @num`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("expected a match")
	}
	if len(match.Captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(match.Captures))
	}
	if match.Captures[0].Node.Type() != "number" {
		t.Errorf("expected 'number', got %q", match.Captures[0].Node.Type())
	}
}

func TestQueryCursorMatchObject(t *testing.T) {
	tree := parseJSON(t, `{"key": "value"}`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(object) @obj`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("expected a match")
	}
	if len(match.Captures) == 0 {
		t.Fatal("expected captures")
	}
	if match.Captures[0].Node.Type() != "object" {
		t.Errorf("expected 'object', got %q", match.Captures[0].Node.Type())
	}
}

func TestQueryCursorMatchNestedCaptures(t *testing.T) {
	tree := parseJSON(t, `{"a": 1}`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(pair (string) @key (number) @val)`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("expected a match")
	}

	if len(match.Captures) < 2 {
		t.Fatalf("expected at least 2 captures, got %d", len(match.Captures))
	}

	// Verify capture names.
	var keyCapture, valCapture *ts.QueryCapture
	for i := range match.Captures {
		name := q.CaptureNameForID(match.Captures[i].Index)
		switch name {
		case "key":
			keyCapture = &match.Captures[i]
		case "val":
			valCapture = &match.Captures[i]
		}
	}

	if keyCapture == nil {
		t.Fatal("missing @key capture")
	}
	if valCapture == nil {
		t.Fatal("missing @val capture")
	}
	if keyCapture.Node.Type() != "string" {
		t.Errorf("@key: expected 'string', got %q", keyCapture.Node.Type())
	}
	if valCapture.Node.Type() != "number" {
		t.Errorf("@val: expected 'number', got %q", valCapture.Node.Type())
	}
}

func TestQueryCursorMultipleMatches(t *testing.T) {
	tree := parseJSON(t, `{"a": 1, "b": 2, "c": 3}`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(pair) @pair`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	count := 0
	for {
		_, ok := cursor.NextMatch()
		if !ok {
			break
		}
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 matches (3 pairs), got %d", count)
	}
}

func TestQueryCursorMatchStringContent(t *testing.T) {
	tree := parseJSON(t, `"hello"`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(string_content) @content`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("expected a match")
	}
	if len(match.Captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(match.Captures))
	}
	if match.Captures[0].Node.Type() != "string_content" {
		t.Errorf("expected 'string_content', got %q", match.Captures[0].Node.Type())
	}
}

func TestQueryCursorNoMatch(t *testing.T) {
	tree := parseJSON(t, `null`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(object) @obj`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	_, ok := cursor.NextMatch()
	if ok {
		t.Error("expected no matches for (object) on null input")
	}
}

func TestQueryCursorByteRange(t *testing.T) {
	tree := parseJSON(t, `{"a": 1, "b": 2}`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(number) @num`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	// Restrict to first half of the input (should only match first number).
	cursor.SetByteRange(0, 8)
	cursor.Exec(tree.RootNode())

	count := 0
	for {
		_, ok := cursor.NextMatch()
		if !ok {
			break
		}
		count++
	}

	if count != 1 {
		t.Errorf("expected 1 match in range [0,8), got %d", count)
	}
}

func TestQueryCursorMultiplePatterns(t *testing.T) {
	tree := parseJSON(t, `{"a": 1, "b": null}`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `
		(number) @num
		(null) @null_val
	`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	matches := make(map[string]int) // patternIndex -> count
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, cap := range match.Captures {
			name := q.CaptureNameForID(cap.Index)
			matches[name]++
		}
	}

	if matches["num"] != 1 {
		t.Errorf("expected 1 @num match, got %d", matches["num"])
	}
	if matches["null_val"] != 1 {
		t.Errorf("expected 1 @null_val match, got %d", matches["null_val"])
	}
}

func TestQueryCursorDeepNesting(t *testing.T) {
	tree := parseJSON(t, `{"a": {"b": {"c": 42}}}`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(number) @num`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	match, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("expected a match for deeply nested number")
	}
	if len(match.Captures) != 1 || match.Captures[0].Node.Type() != "number" {
		t.Errorf("unexpected capture: %+v", match.Captures)
	}
}

func TestQueryCursorWildcard(t *testing.T) {
	tree := parseJSON(t, `[1, 2, 3]`)
	lang := queryTestLanguage()
	q, err := ts.NewQuery(lang, `(array (_) @element)`)
	if err != nil {
		t.Fatal(err)
	}

	cursor := ts.NewQueryCursor(q)
	cursor.Exec(tree.RootNode())

	count := 0
	for {
		_, ok := cursor.NextMatch()
		if !ok {
			break
		}
		count++
	}

	// Should match each number element in the array.
	if count < 1 {
		t.Errorf("expected at least 1 match for wildcard in array, got %d", count)
	}
}
