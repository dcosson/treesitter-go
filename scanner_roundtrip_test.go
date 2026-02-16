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
	"github.com/treesitter-go/treesitter/scanners/python"
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
