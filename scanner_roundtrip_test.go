// Cross-scanner serialization roundtrip tests.
// These tests verify that serialize → deserialize → continued scanning produces
// correct results, validating that scanner state is faithfully preserved across
// incremental parse boundaries.
package treesitter_test

import (
	"testing"

	ts "github.com/treesitter-go/treesitter"

	"github.com/treesitter-go/treesitter/scanners/bash"
	"github.com/treesitter-go/treesitter/scanners/cpp"
	"github.com/treesitter-go/treesitter/scanners/css"
	"github.com/treesitter-go/treesitter/scanners/html"
	"github.com/treesitter-go/treesitter/scanners/javascript"
	"github.com/treesitter-go/treesitter/scanners/lua"
	"github.com/treesitter-go/treesitter/scanners/perl"
	"github.com/treesitter-go/treesitter/scanners/python"
	"github.com/treesitter-go/treesitter/scanners/ruby"
	"github.com/treesitter-go/treesitter/scanners/rust"
	"github.com/treesitter-go/treesitter/scanners/typescript"
)

const serializationBufSize = 1024

// TestBashSerializationRoundtrip verifies that Bash scanner state (heredocs)
// survives serialization roundtrip.
func TestBashSerializationRoundtrip(t *testing.T) {
	s1 := bash.New()
	buf := make([]byte, serializationBufSize)

	// Serialize empty state.
	n := s1.Serialize(buf)
	if n == 0 {
		t.Fatal("empty Bash serialization should produce non-zero bytes")
	}

	// Deserialize into fresh scanner and verify it works.
	s2 := bash.New()
	s2.Deserialize(buf[:n])

	// Both scanners should behave identically on the same input.
	lexer1 := newTestLexer("abc")
	lexer2 := newTestLexer("abc")
	v := bashValidConcat()

	r1 := s1.Scan(lexer1, v)
	r2 := s2.Scan(lexer2, v)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestPythonSerializationRoundtrip verifies that Python scanner state
// (indent stack, delimiters) survives serialization roundtrip.
func TestPythonSerializationRoundtrip(t *testing.T) {
	s1 := python.New()
	buf := make([]byte, serializationBufSize)

	// Parse an indent to build up state.
	lexer := newTestLexer("\n    x")
	v := pythonValidIndent()
	s1.Scan(lexer, v)

	// Serialize with indent state.
	n := s1.Serialize(buf)
	if n < 2 {
		t.Fatalf("expected >2 bytes for Python with indent, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := python.New()
	s2.Deserialize(buf[:n])

	// Both should produce the same dedent on the next scan.
	lexer1 := newTestLexer("\nx")
	lexer2 := newTestLexer("\nx")
	vDedent := pythonValidDedent()

	r1 := s1.Scan(lexer1, vDedent)
	r2 := s2.Scan(lexer2, vDedent)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestRustSerializationRoundtrip verifies Rust scanner state (hash count).
func TestRustSerializationRoundtrip(t *testing.T) {
	s1 := rust.New()
	buf := make([]byte, serializationBufSize)

	// Scan a raw string start to set hash count.
	lexer := newTestLexer(`r##"content"##`)
	v := rustValidRawStart()
	s1.Scan(lexer, v)

	// Serialize.
	n := s1.Serialize(buf)
	if n != 1 {
		t.Fatalf("expected 1 byte for Rust serialization, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := rust.New()
	s2.Deserialize(buf[:n])

	// Both should scan raw string content identically.
	lexer1 := newTestLexer(`some content "## more`)
	lexer2 := newTestLexer(`some content "## more`)
	vContent := rustValidRawContent()

	r1 := s1.Scan(lexer1, vContent)
	r2 := s2.Scan(lexer2, vContent)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
}

// TestCppSerializationRoundtrip verifies C++ scanner state (delimiter).
func TestCppSerializationRoundtrip(t *testing.T) {
	s1 := cpp.New()
	buf := make([]byte, serializationBufSize)

	// Scan opening delimiter.
	lexer := newTestLexer("foo(")
	v := cppValidDelimiter()
	s1.Scan(lexer, v)

	// Serialize.
	n := s1.Serialize(buf)
	if n != 12 { // 3 runes * 4 bytes
		t.Fatalf("expected 12 bytes for C++ with 'foo' delimiter, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := cpp.New()
	s2.Deserialize(buf[:n])

	// Both should scan content identically.
	lexer1 := newTestLexer("content)foo\"")
	lexer2 := newTestLexer("content)foo\"")
	vContent := cppValidContent()

	r1 := s1.Scan(lexer1, vContent)
	r2 := s2.Scan(lexer2, vContent)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
}

// TestTypescriptSerializationRoundtrip verifies TypeScript stateless roundtrip.
func TestTypescriptSerializationRoundtrip(t *testing.T) {
	s1 := typescript.New()
	buf := make([]byte, serializationBufSize)

	// Stateless: serialize should return 0.
	n := s1.Serialize(buf)
	if n != 0 {
		t.Fatalf("TypeScript serialize should return 0, got %d", n)
	}

	s2 := typescript.New()
	s2.Deserialize(buf[:n])

	// Both should produce identical results.
	lexer1 := newTestLexer("hello world`")
	lexer2 := newTestLexer("hello world`")
	v := tsValidTemplateChars()

	r1 := s1.Scan(lexer1, v)
	r2 := s2.Scan(lexer2, v)
	if r1 != r2 {
		t.Errorf("Scan result mismatch: s1=%v s2=%v", r1, r2)
	}
}

// TestHTMLSerializationRoundtrip verifies HTML scanner state (tag stack)
// survives serialization roundtrip.
func TestHTMLSerializationRoundtrip(t *testing.T) {
	s1 := html.New()
	buf := make([]byte, serializationBufSize)

	// Scan a start tag to push it onto the tag stack.
	lexer := newTestLexer("div")
	v := htmlValidStartTagName()
	s1.Scan(lexer, v)

	// Serialize with tag on the stack.
	n := s1.Serialize(buf)
	if n < 4 {
		t.Fatalf("expected >=4 bytes for HTML with tag on stack, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := html.New()
	s2.Deserialize(buf[:n])

	// Re-serialize and verify identical bytes.
	buf2 := make([]byte, serializationBufSize)
	n2 := s2.Serialize(buf2)
	if n != n2 {
		t.Fatalf("serialize size mismatch: first=%d second=%d", n, n2)
	}
	for i := uint32(0); i < n; i++ {
		if buf[i] != buf2[i] {
			t.Fatalf("byte %d mismatch: first=%d second=%d", i, buf[i], buf2[i])
		}
	}

	// Both scanners should produce identical results scanning an end tag.
	lexer1 := newTestLexer("div")
	lexer2 := newTestLexer("div")
	vEnd := htmlValidEndTagName()

	r1 := s1.Scan(lexer1, vEnd)
	r2 := s2.Scan(lexer2, vEnd)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestHTMLSerializationRoundtripCustomTag verifies HTML scanner preserves custom
// (non-builtin) tag names through serialization.
func TestHTMLSerializationRoundtripCustomTag(t *testing.T) {
	s1 := html.New()
	buf := make([]byte, serializationBufSize)

	// Scan a custom tag name (not a built-in HTML tag).
	lexer := newTestLexer("my-component")
	v := htmlValidStartTagName()
	s1.Scan(lexer, v)

	// Serialize.
	n := s1.Serialize(buf)
	if n < 5 {
		t.Fatalf("expected >=5 bytes for HTML with custom tag, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := html.New()
	s2.Deserialize(buf[:n])

	// Re-serialize and verify identical bytes.
	buf2 := make([]byte, serializationBufSize)
	n2 := s2.Serialize(buf2)
	if n != n2 {
		t.Fatalf("serialize size mismatch: first=%d second=%d", n, n2)
	}
	for i := uint32(0); i < n; i++ {
		if buf[i] != buf2[i] {
			t.Fatalf("byte %d mismatch: first=%d second=%d", i, buf[i], buf2[i])
		}
	}
}

// TestHTMLSerializationRoundtripMultipleTags verifies tag stack ordering is preserved.
func TestHTMLSerializationRoundtripMultipleTags(t *testing.T) {
	s1 := html.New()
	buf := make([]byte, serializationBufSize)

	// Push multiple tags: html > body > div
	for _, tag := range []string{"html", "body", "div"} {
		lexer := newTestLexer(tag)
		v := htmlValidStartTagName()
		s1.Scan(lexer, v)
	}

	// Serialize.
	n := s1.Serialize(buf)
	if n < 7 { // 4 header + 3 tag bytes
		t.Fatalf("expected >=7 bytes for HTML with 3 tags, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := html.New()
	s2.Deserialize(buf[:n])

	// Re-serialize and verify identical bytes.
	buf2 := make([]byte, serializationBufSize)
	n2 := s2.Serialize(buf2)
	if n != n2 {
		t.Fatalf("serialize size mismatch: first=%d second=%d", n, n2)
	}
	for i := uint32(0); i < n; i++ {
		if buf[i] != buf2[i] {
			t.Fatalf("byte %d mismatch: first=%d second=%d", i, buf[i], buf2[i])
		}
	}

	// Both should handle closing the innermost tag identically.
	lexer1 := newTestLexer("div")
	lexer2 := newTestLexer("div")
	vEnd := htmlValidEndTagName()

	r1 := s1.Scan(lexer1, vEnd)
	r2 := s2.Scan(lexer2, vEnd)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestLuaSerializationRoundtrip verifies Lua scanner state (endingChar, levelCount)
// survives serialization roundtrip.
func TestLuaSerializationRoundtrip(t *testing.T) {
	s1 := lua.New()
	buf := make([]byte, serializationBufSize)

	// Scan a block string start to set the level count.
	lexer := newTestLexer("[==[")
	v := luaValidBlockStringStart()
	s1.Scan(lexer, v)

	// Serialize.
	n := s1.Serialize(buf)
	if n != 2 {
		t.Fatalf("expected 2 bytes for Lua serialization, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := lua.New()
	s2.Deserialize(buf[:n])

	// Re-serialize and verify identical bytes.
	buf2 := make([]byte, serializationBufSize)
	n2 := s2.Serialize(buf2)
	if n != n2 {
		t.Fatalf("serialize size mismatch: first=%d second=%d", n, n2)
	}
	for i := uint32(0); i < n; i++ {
		if buf[i] != buf2[i] {
			t.Fatalf("byte %d mismatch: first=%d second=%d", i, buf[i], buf2[i])
		}
	}

	// Both should scan block string content identically.
	lexer1 := newTestLexer("some content]==]")
	lexer2 := newTestLexer("some content]==]")
	vContent := luaValidBlockStringContent()

	r1 := s1.Scan(lexer1, vContent)
	r2 := s2.Scan(lexer2, vContent)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestLuaSerializationRoundtripDifferentLevels verifies different level counts
// are preserved through serialization.
func TestLuaSerializationRoundtripDifferentLevels(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		level int
	}{
		{"level-0", "[[", 0},
		{"level-1", "[=[", 1},
		{"level-3", "[===[", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := lua.New()
			lexer := newTestLexer(tc.input)
			v := luaValidBlockStringStart()
			s.Scan(lexer, v)

			buf1 := make([]byte, serializationBufSize)
			n1 := s.Serialize(buf1)

			s2 := lua.New()
			s2.Deserialize(buf1[:n1])

			buf2 := make([]byte, serializationBufSize)
			n2 := s2.Serialize(buf2)

			if n1 != n2 {
				t.Errorf("serialize size mismatch: first=%d second=%d", n1, n2)
			}
			for i := uint32(0); i < n1; i++ {
				if buf1[i] != buf2[i] {
					t.Errorf("byte %d mismatch: first=%d second=%d", i, buf1[i], buf2[i])
				}
			}
		})
	}
}

// TestCSSSerializationRoundtrip verifies CSS stateless roundtrip.
func TestCSSSerializationRoundtrip(t *testing.T) {
	s1 := css.New()
	buf := make([]byte, serializationBufSize)

	// Stateless: serialize should return 0.
	n := s1.Serialize(buf)
	if n != 0 {
		t.Fatalf("CSS serialize should return 0, got %d", n)
	}

	s2 := css.New()
	s2.Deserialize(buf[:n])

	// Both should produce identical results.
	lexer1 := newTestLexer(" div")
	lexer2 := newTestLexer(" div")
	v := cssValidDescendantOp()

	r1 := s1.Scan(lexer1, v)
	r2 := s2.Scan(lexer2, v)
	if r1 != r2 {
		t.Errorf("Scan result mismatch: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestJavaScriptSerializationRoundtrip verifies JavaScript stateless roundtrip.
func TestJavaScriptSerializationRoundtrip(t *testing.T) {
	s1 := javascript.New()
	buf := make([]byte, serializationBufSize)

	// Stateless: serialize should return 0.
	n := s1.Serialize(buf)
	if n != 0 {
		t.Fatalf("JavaScript serialize should return 0, got %d", n)
	}

	s2 := javascript.New()
	s2.Deserialize(buf[:n])

	// Both should produce identical results scanning template chars.
	lexer1 := newTestLexer("hello world`")
	lexer2 := newTestLexer("hello world`")
	v := jsValidTemplateChars()

	r1 := s1.Scan(lexer1, v)
	r2 := s2.Scan(lexer2, v)
	if r1 != r2 {
		t.Errorf("Scan result mismatch: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestPerlSerializationRoundtrip verifies Perl scanner state (quotes, heredoc)
// survives serialization roundtrip.
func TestPerlSerializationRoundtrip(t *testing.T) {
	s1 := perl.New()
	buf := make([]byte, serializationBufSize)

	// Serialize empty state — Perl always writes at least 1 byte (quote count).
	n := s1.Serialize(buf)
	if n == 0 {
		t.Fatal("empty Perl serialization should produce non-zero bytes")
	}

	// Deserialize into fresh scanner.
	s2 := perl.New()
	s2.Deserialize(buf[:n])

	// Re-serialize and verify identical bytes.
	buf2 := make([]byte, serializationBufSize)
	n2 := s2.Serialize(buf2)
	if n != n2 {
		t.Fatalf("serialize size mismatch: first=%d second=%d", n, n2)
	}
	for i := uint32(0); i < n; i++ {
		if buf[i] != buf2[i] {
			t.Fatalf("byte %d mismatch: first=%d second=%d", i, buf[i], buf2[i])
		}
	}
}

// TestPerlSerializationRoundtripWithQuote verifies Perl quote state preservation.
func TestPerlSerializationRoundtripWithQuote(t *testing.T) {
	s1 := perl.New()
	buf := make([]byte, serializationBufSize)

	// Scan an apostrophe to push a quote onto the stack.
	lexer := newTestLexer("'")
	v := perlValidApostrophe()
	s1.Scan(lexer, v)

	// Serialize with quote state.
	n := s1.Serialize(buf)
	if n < 13 { // 1 (quote count) + 12 (one quote: open+close+count @ 4 bytes each)
		t.Fatalf("expected >=13 bytes for Perl with quote, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := perl.New()
	s2.Deserialize(buf[:n])

	// Re-serialize and verify identical bytes.
	buf2 := make([]byte, serializationBufSize)
	n2 := s2.Serialize(buf2)
	if n != n2 {
		t.Fatalf("serialize size mismatch: first=%d second=%d", n, n2)
	}
	for i := uint32(0); i < n; i++ {
		if buf[i] != buf2[i] {
			t.Fatalf("byte %d mismatch: first=%d second=%d", i, buf[i], buf2[i])
		}
	}

	// Both scanners should scan q-string content identically.
	lexer1 := newTestLexer("hello world")
	lexer2 := newTestLexer("hello world")
	vContent := perlValidQStringContent()

	r1 := s1.Scan(lexer1, vContent)
	r2 := s2.Scan(lexer2, vContent)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestRubySerializationRoundtrip verifies Ruby scanner state (literal stack)
// survives serialization roundtrip.
func TestRubySerializationRoundtrip(t *testing.T) {
	s1 := ruby.New()
	buf := make([]byte, serializationBufSize)

	// Serialize empty state — Ruby writes at least 2 bytes (literal count + heredoc count).
	n := s1.Serialize(buf)
	if n < 2 {
		t.Fatalf("expected >=2 bytes for empty Ruby serialization, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := ruby.New()
	s2.Deserialize(buf[:n])

	// Re-serialize and verify identical bytes.
	buf2 := make([]byte, serializationBufSize)
	n2 := s2.Serialize(buf2)
	if n != n2 {
		t.Fatalf("serialize size mismatch: first=%d second=%d", n, n2)
	}
	for i := uint32(0); i < n; i++ {
		if buf[i] != buf2[i] {
			t.Fatalf("byte %d mismatch: first=%d second=%d", i, buf[i], buf2[i])
		}
	}
}

// TestRubySerializationRoundtripWithLiteral verifies Ruby literal stack preservation.
func TestRubySerializationRoundtripWithLiteral(t *testing.T) {
	s1 := ruby.New()
	buf := make([]byte, serializationBufSize)

	// Scan a double quote to push a string literal onto the stack.
	lexer := newTestLexer("\"")
	v := rubyValidStringStart()
	s1.Scan(lexer, v)

	// Serialize with literal state.
	n := s1.Serialize(buf)
	if n < 13 { // 1 (lit count) + 11 (one literal) + 1 (heredoc count)
		t.Fatalf("expected >=13 bytes for Ruby with literal, got %d", n)
	}

	// Deserialize into fresh scanner.
	s2 := ruby.New()
	s2.Deserialize(buf[:n])

	// Re-serialize and verify identical bytes.
	buf2 := make([]byte, serializationBufSize)
	n2 := s2.Serialize(buf2)
	if n != n2 {
		t.Fatalf("serialize size mismatch: first=%d second=%d", n, n2)
	}
	for i := uint32(0); i < n; i++ {
		if buf[i] != buf2[i] {
			t.Fatalf("byte %d mismatch: first=%d second=%d", i, buf[i], buf2[i])
		}
	}

	// Both scanners should scan string content identically.
	lexer1 := newTestLexer("hello world\"")
	lexer2 := newTestLexer("hello world\"")
	vContent := rubyValidStringContent()

	r1 := s1.Scan(lexer1, vContent)
	r2 := s2.Scan(lexer2, vContent)
	if r1 != r2 {
		t.Errorf("Scan result mismatch after roundtrip: s1=%v s2=%v", r1, r2)
	}
	if lexer1.ResultSymbol != lexer2.ResultSymbol {
		t.Errorf("ResultSymbol mismatch: s1=%d s2=%d", lexer1.ResultSymbol, lexer2.ResultSymbol)
	}
}

// TestAllScannersDeserializeEmpty verifies all scanners handle empty data.
func TestAllScannersDeserializeEmpty(t *testing.T) {
	scanners := []struct {
		name string
		s    ts.ExternalScanner
	}{
		{"bash", bash.New()},
		{"python", python.New()},
		{"rust", rust.New()},
		{"cpp", cpp.New()},
		{"typescript", typescript.New()},
		{"css", css.New()},
		{"html", html.New()},
		{"javascript", javascript.New()},
		{"lua", lua.New()},
		{"perl", perl.New()},
		{"ruby", ruby.New()},
	}

	for _, sc := range scanners {
		t.Run(sc.name, func(t *testing.T) {
			// Deserialize nil should not panic.
			sc.s.Deserialize(nil)
			// Deserialize empty should not panic.
			sc.s.Deserialize([]byte{})
			// Serialize after empty deserialize should work.
			buf := make([]byte, serializationBufSize)
			sc.s.Serialize(buf)
		})
	}
}

// TestAllScannersSerializeRoundtrip tests that serialize→deserialize→serialize
// produces identical bytes for all scanners.
func TestAllScannersSerializeRoundtrip(t *testing.T) {
	scanners := []struct {
		name string
		s    ts.ExternalScanner
	}{
		{"bash", bash.New()},
		{"python", python.New()},
		{"rust", rust.New()},
		{"cpp", cpp.New()},
		{"typescript", typescript.New()},
		{"css", css.New()},
		{"html", html.New()},
		{"javascript", javascript.New()},
		{"lua", lua.New()},
		{"perl", perl.New()},
		{"ruby", ruby.New()},
	}

	for _, sc := range scanners {
		t.Run(sc.name, func(t *testing.T) {
			buf1 := make([]byte, serializationBufSize)
			buf2 := make([]byte, serializationBufSize)

			// First roundtrip.
			n1 := sc.s.Serialize(buf1)

			// Deserialize and re-serialize.
			s2 := newScanner(sc.name)
			s2.Deserialize(buf1[:n1])
			n2 := s2.Serialize(buf2)

			if n1 != n2 {
				t.Errorf("serialize size mismatch: first=%d second=%d", n1, n2)
			}
			for i := uint32(0); i < n1; i++ {
				if buf1[i] != buf2[i] {
					t.Errorf("byte %d mismatch: first=%d second=%d", i, buf1[i], buf2[i])
					break
				}
			}
		})
	}
}

// --- Helpers ---

func newTestLexer(s string) *ts.Lexer {
	lexer := ts.NewLexer()
	lexer.SetInput(ts.NewStringInput([]byte(s)))
	lexer.Start(ts.Length{})
	return lexer
}

func newScanner(name string) ts.ExternalScanner {
	switch name {
	case "bash":
		return bash.New()
	case "python":
		return python.New()
	case "rust":
		return rust.New()
	case "cpp":
		return cpp.New()
	case "typescript":
		return typescript.New()
	case "css":
		return css.New()
	case "html":
		return html.New()
	case "javascript":
		return javascript.New()
	case "lua":
		return lua.New()
	case "perl":
		return perl.New()
	case "ruby":
		return ruby.New()
	default:
		panic("unknown scanner: " + name)
	}
}

func bashValidConcat() []bool {
	v := make([]bool, 29) // ErrorRecovery=28
	v[bash.Concat] = true
	return v
}

func pythonValidIndent() []bool {
	v := make([]bool, 12) // Except=11
	v[python.Indent] = true
	v[python.Dedent] = true
	v[python.Newline] = true
	return v
}

func pythonValidDedent() []bool {
	v := make([]bool, 12)
	v[python.Indent] = true
	v[python.Dedent] = true
	v[python.Newline] = true
	return v
}

func rustValidRawStart() []bool {
	v := make([]bool, 10) // ErrorSentinel=9
	v[rust.RawStringLiteralStart] = true
	return v
}

func rustValidRawContent() []bool {
	v := make([]bool, 10)
	v[rust.RawStringLiteralContent] = true
	return v
}

func cppValidDelimiter() []bool {
	v := make([]bool, 2)
	v[cpp.RawStringDelimiter] = true
	return v
}

func cppValidContent() []bool {
	v := make([]bool, 2)
	v[cpp.RawStringContent] = true
	return v
}

func tsValidTemplateChars() []bool {
	v := make([]bool, 10) // ErrorRecovery=9
	v[typescript.TemplateChars] = true
	return v
}

func htmlValidStartTagName() []bool {
	v := make([]bool, html.Comment+1)
	v[html.StartTagName] = true
	return v
}

func htmlValidEndTagName() []bool {
	v := make([]bool, html.Comment+1)
	v[html.EndTagName] = true
	return v
}

func luaValidBlockStringStart() []bool {
	v := make([]bool, lua.BlockStringEnd+1)
	v[lua.BlockStringStart] = true
	return v
}

func luaValidBlockStringContent() []bool {
	v := make([]bool, lua.BlockStringEnd+1)
	v[lua.BlockStringContent] = true
	return v
}

func cssValidDescendantOp() []bool {
	v := make([]bool, css.ErrorRecovery+1)
	v[css.DescendantOp] = true
	return v
}

func jsValidTemplateChars() []bool {
	v := make([]bool, javascript.JSXText+1)
	v[javascript.TemplateChars] = true
	return v
}

func perlValidApostrophe() []bool {
	v := make([]bool, perl.TokenError+1)
	v[perl.TokenApostrophe] = true
	return v
}

func perlValidQStringContent() []bool {
	v := make([]bool, perl.TokenError+1)
	v[perl.TokenQStringContent] = true
	return v
}

func rubyValidStringStart() []bool {
	v := make([]bool, ruby.None+1)
	v[ruby.StringStart] = true
	return v
}

func rubyValidStringContent() []bool {
	v := make([]bool, ruby.None+1)
	v[ruby.StringContent] = true
	v[ruby.StringEnd] = true
	return v
}

// --- Scanner Benchmarks ---

func BenchmarkBashScan(b *testing.B) {
	s := bash.New()
	v := bashValidConcat()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer("abc_def ghi")
		s.Scan(lexer, v)
	}
}

func BenchmarkPythonScan(b *testing.B) {
	s := python.New()
	v := pythonValidIndent()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer("\n    x = 1")
		s.Scan(lexer, v)
	}
}

func BenchmarkRustScan(b *testing.B) {
	s := rust.New()
	v := rustValidRawStart()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer(`r##"content"##`)
		s.Scan(lexer, v)
	}
}

func BenchmarkCppScan(b *testing.B) {
	s := cpp.New()
	v := cppValidDelimiter()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer("delim(")
		s.Scan(lexer, v)
	}
}

func BenchmarkTypescriptScan(b *testing.B) {
	s := typescript.New()
	v := tsValidTemplateChars()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer("hello world`")
		s.Scan(lexer, v)
	}
}

func BenchmarkHTMLScan(b *testing.B) {
	s := html.New()
	v := htmlValidStartTagName()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer("div")
		s.Scan(lexer, v)
	}
}

func BenchmarkLuaScan(b *testing.B) {
	s := lua.New()
	v := luaValidBlockStringStart()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer("[==[")
		s.Scan(lexer, v)
	}
}

func BenchmarkJavaScriptScan(b *testing.B) {
	s := javascript.New()
	v := jsValidTemplateChars()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer("hello world`")
		s.Scan(lexer, v)
	}
}

func BenchmarkCSSScan(b *testing.B) {
	s := css.New()
	v := cssValidDescendantOp()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		lexer := newTestLexer(" div")
		s.Scan(lexer, v)
	}
}

func BenchmarkSerializeDeserialize(b *testing.B) {
	scanners := []struct {
		name string
		s    ts.ExternalScanner
	}{
		{"bash", bash.New()},
		{"python", python.New()},
		{"rust", rust.New()},
		{"cpp", cpp.New()},
		{"typescript", typescript.New()},
		{"css", css.New()},
		{"html", html.New()},
		{"javascript", javascript.New()},
		{"lua", lua.New()},
		{"perl", perl.New()},
		{"ruby", ruby.New()},
	}

	for _, sc := range scanners {
		b.Run(sc.name, func(b *testing.B) {
			buf := make([]byte, serializationBufSize)
			n := sc.s.Serialize(buf)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				s2 := newScanner(sc.name)
				s2.Deserialize(buf[:n])
				s2.Serialize(buf)
			}
		})
	}
}

// --- Scanner Consistency Tests ---

// TestScannerDeterminism verifies that repeated scanning of the same input
// produces identical results, detecting any non-deterministic behavior.
func TestScannerDeterminism(t *testing.T) {
	cases := []struct {
		name        string
		scannerName string
		input       string
		valid       []bool
	}{
		{"bash-concat", "bash", "abc_def", bashValidConcat()},
		{"python-indent", "python", "\n    x", pythonValidIndent()},
		{"rust-rawstring", "rust", `r##"content"##`, rustValidRawStart()},
		{"cpp-delimiter", "cpp", "foo(", cppValidDelimiter()},
		{"typescript-template", "typescript", "hello world`", tsValidTemplateChars()},
		{"css-descendant", "css", " div", cssValidDescendantOp()},
		{"html-starttag", "html", "div", htmlValidStartTagName()},
		{"javascript-template", "javascript", "hello world`", jsValidTemplateChars()},
		{"lua-blockstring", "lua", "[==[", luaValidBlockStringStart()},
		{"perl-apostrophe", "perl", "'", perlValidApostrophe()},
		{"ruby-stringstart", "ruby", "\"", rubyValidStringStart()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Run 100 times and verify all results are identical.
			var firstResult bool
			var firstSymbol ts.Symbol
			for i := 0; i < 100; i++ {
				s := newScanner(tc.scannerName)
				lexer := newTestLexer(tc.input)
				result := s.Scan(lexer, tc.valid)
				if i == 0 {
					firstResult = result
					firstSymbol = lexer.ResultSymbol
				} else {
					if result != firstResult {
						t.Fatalf("non-deterministic result on iteration %d: got %v, want %v", i, result, firstResult)
					}
					if lexer.ResultSymbol != firstSymbol {
						t.Fatalf("non-deterministic symbol on iteration %d: got %d, want %d", i, lexer.ResultSymbol, firstSymbol)
					}
				}
			}
		})
	}
}
