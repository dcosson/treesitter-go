package treesitter_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	tg "github.com/treesitter-go/treesitter/internal/testgrammars"
	bashgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/bash"
	cgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cgrammar"
	cppgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cppgrammar"
	cssgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/css"
	golanggrammar "github.com/treesitter-go/treesitter/internal/testgrammars/golang"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/html"
	javagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/java"
	jsgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/javascript"
	luagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/lua"
	perlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/perl"
	pygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/python"
	rubygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/ruby"
	rustgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/rustgrammar"
	tsgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/typescript"
	bashscanner "github.com/treesitter-go/treesitter/scanners/bash"
	cppscanner "github.com/treesitter-go/treesitter/scanners/cpp"
	cssscanner "github.com/treesitter-go/treesitter/scanners/css"
	htmlscanner "github.com/treesitter-go/treesitter/scanners/html"
	jsscanner "github.com/treesitter-go/treesitter/scanners/javascript"
	luascanner "github.com/treesitter-go/treesitter/scanners/lua"
	perlscanner "github.com/treesitter-go/treesitter/scanners/perl"
	pyscanner "github.com/treesitter-go/treesitter/scanners/python"
	rubyscanner "github.com/treesitter-go/treesitter/scanners/ruby"
	rustscanner "github.com/treesitter-go/treesitter/scanners/rust"
	tsscanner "github.com/treesitter-go/treesitter/scanners/typescript"
)

// tsCLI is the path to the tree-sitter CLI binary.
// Set via -ts-cli flag or TS_CLI_PATH environment variable.
var tsCLI = flag.String("ts-cli", os.Getenv("TS_CLI_PATH"), "path to tree-sitter CLI binary")

// benchDylibDir is the directory containing prebuilt grammar dylibs for CLI benchmarks.
// Build with: make bench-grammars
const benchDylibDir = "build/benchmark-dylibs"

// benchLang describes a language available for benchmarking.
type benchLang struct {
	name     string
	ext      string // file extension for temp file naming
	libName  string // tree-sitter language name for --lang-name flag
	language func() *ts.Language
	generate func(targetBytes int) []byte
}

// benchLanguages returns all 15 supported languages with their input generators.
func benchLanguages() []benchLang {
	return []benchLang{
		{"json", ".json", "json", func() *ts.Language { return tg.JSONLanguage() }, generateJSON},
		{"go", ".go", "go", func() *ts.Language { return golanggrammar.GoLanguage() }, generateGo},
		{"python", ".py", "python", func() *ts.Language {
			l := pygrammar.PythonLanguage()
			l.NewExternalScanner = pyscanner.New
			return l
		}, generatePython},
		{"javascript", ".js", "javascript", func() *ts.Language {
			l := jsgrammar.JavascriptLanguage()
			l.NewExternalScanner = jsscanner.New
			return l
		}, generateJavaScript},
		{"typescript", ".ts", "typescript", func() *ts.Language {
			l := tsgrammar.TypescriptLanguage()
			l.NewExternalScanner = tsscanner.New
			return l
		}, generateTypeScript},
		{"c", ".c", "c", func() *ts.Language { return cgrammar.CLanguage() }, generateC},
		{"cpp", ".cpp", "cpp", func() *ts.Language {
			l := cppgrammar.CppLanguage()
			l.NewExternalScanner = cppscanner.New
			return l
		}, generateCpp},
		{"rust", ".rs", "rust", func() *ts.Language {
			l := rustgrammar.RustLanguage()
			l.NewExternalScanner = rustscanner.New
			return l
		}, generateRust},
		{"java", ".java", "java", func() *ts.Language { return javagrammar.JavaLanguage() }, generateJava},
		{"ruby", ".rb", "ruby", func() *ts.Language {
			l := rubygrammar.RubyLanguage()
			l.NewExternalScanner = rubyscanner.New
			return l
		}, generateRuby},
		{"bash", ".sh", "bash", func() *ts.Language {
			l := bashgrammar.BashLanguage()
			l.NewExternalScanner = bashscanner.New
			return l
		}, generateBash},
		{"css", ".css", "css", func() *ts.Language {
			l := cssgrammar.CssLanguage()
			l.NewExternalScanner = cssscanner.New
			return l
		}, generateCSS},
		{"html", ".html", "html", func() *ts.Language {
			l := htmlgrammar.HtmlLanguage()
			l.NewExternalScanner = htmlscanner.New
			return l
		}, generateHTML},
		{"perl", ".pl", "perl", func() *ts.Language {
			l := perlgrammar.PerlLanguage()
			l.NewExternalScanner = perlscanner.New
			return l
		}, generatePerl},
		{"lua", ".lua", "lua", func() *ts.Language {
			l := luagrammar.LuaLanguage()
			l.NewExternalScanner = luascanner.New
			return l
		}, generateLua},
	}
}

// hasCLI returns true if the tree-sitter CLI is configured and available,
// and at least one prebuilt grammar dylib exists in benchDylibDir.
func hasCLI() bool {
	if *tsCLI == "" {
		return false
	}
	_, err := exec.LookPath(*tsCLI)
	if err != nil {
		return false
	}
	entries, err := os.ReadDir(benchDylibDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".dylib" {
			return true
		}
	}
	return false
}

// cliParseBytes parses input bytes using the tree-sitter CLI with a prebuilt grammar dylib.
// Uses --lib-path to point at the precompiled dylib and --lang-name to select the language.
func cliParseBytes(input []byte, ext, libName string) error {
	tmpFile, err := os.CreateTemp("", "bench-*"+ext)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(input); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	libPath := filepath.Join(benchDylibDir, libName+".dylib")
	cmd := exec.Command(*tsCLI, "parse", "--lib-path", libPath, "--lang-name", libName, tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil // Parse tree has errors but CLI still produced valid output.
		}
		return fmt.Errorf("tree-sitter parse failed: %v\noutput: %s", err, output)
	}
	return nil
}

// --- Unified Parse Benchmark (Go + optional CLI comparison) ---

func BenchmarkParse(b *testing.B) {
	sizes := []struct {
		name  string
		bytes int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	for _, lang := range benchLanguages() {
		for _, size := range sizes {
			input := lang.generate(size.bytes)
			l := lang.language()

			// Go benchmark.
			b.Run(fmt.Sprintf("go/%s/%s", lang.name, size.name), func(b *testing.B) {
				parser := ts.NewParser()
				parser.SetLanguage(l)

				// Warm up and verify parse succeeds.
				tree := parser.ParseString(context.Background(), input)
				if tree == nil {
					b.Skipf("%s parse returned nil for %s input", lang.name, size.name)
				}

				b.SetBytes(int64(len(input)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					parser.ParseString(context.Background(), input)
				}
			})

			// CLI comparison benchmark (only when CLI is available).
			if hasCLI() {
				ext := lang.ext
				libName := lang.libName
				inputCopy := append([]byte(nil), input...)
				b.Run(fmt.Sprintf("cli/%s/%s", lang.name, size.name), func(b *testing.B) {
					// Verify CLI can parse this language with the prebuilt dylib.
					if err := cliParseBytes(inputCopy[:min(len(inputCopy), 100)], ext, libName); err != nil {
						b.Skipf("CLI cannot parse %s: %v", lang.name, err)
					}

					b.SetBytes(int64(len(inputCopy)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						cliParseBytes(inputCopy, ext, libName)
					}
				})
			}
		}
	}
}

// --- Latency Distribution Benchmark (p50/p95/p99) ---

func BenchmarkLatencyDistribution(b *testing.B) {
	langs := benchLanguages()
	// Use a representative subset for latency: JSON, Go, Python, JavaScript, C++.
	latencyLangs := []benchLang{}
	latencyNames := map[string]bool{"json": true, "go": true, "python": true, "javascript": true, "cpp": true}
	for _, l := range langs {
		if latencyNames[l.name] {
			latencyLangs = append(latencyLangs, l)
		}
	}

	for _, lang := range latencyLangs {
		input := lang.generate(10 * 1024) // 10KB — typical editor buffer size
		l := lang.language()

		b.Run(lang.name, func(b *testing.B) {
			parser := ts.NewParser()
			parser.SetLanguage(l)

			tree := parser.ParseString(context.Background(), input)
			if tree == nil {
				b.Skipf("%s parse returned nil", lang.name)
			}

			// Collect individual parse times.
			const iterations = 1000
			durations := make([]time.Duration, iterations)

			b.ResetTimer()
			for i := 0; i < iterations; i++ {
				start := time.Now()
				parser.ParseString(context.Background(), input)
				durations[i] = time.Since(start)
			}
			b.StopTimer()

			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

			p50 := durations[iterations*50/100]
			p95 := durations[iterations*95/100]
			p99 := durations[iterations*99/100]

			b.ReportMetric(float64(p50.Microseconds()), "p50-us")
			b.ReportMetric(float64(p95.Microseconds()), "p95-us")
			b.ReportMetric(float64(p99.Microseconds()), "p99-us")
			b.ReportMetric(float64(len(input))/p50.Seconds()/1e6, "MB/s-p50")
		})
	}
}

// --- Nested JSON Benchmarks (stress-test deep parse stacks) ---

func BenchmarkParseNestedJSON_500(b *testing.B) {
	input := generateNestedJSON(2000) // ~500 depth
	lang := tg.JSONLanguage()

	parser := ts.NewParser()
	parser.SetLanguage(lang)

	tree := parser.ParseString(context.Background(), input)
	if tree == nil {
		b.Fatal("parse failed for nested JSON")
	}

	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser.ParseString(context.Background(), input)
	}
}

// --- Tree Traversal Benchmarks ---

func BenchmarkTreeTraversal(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
	} {
		b.Run(size.name, func(b *testing.B) {
			input := generateJSON(size.bytes)
			lang := tg.JSONLanguage()

			parser := ts.NewParser()
			parser.SetLanguage(lang)
			tree := parser.ParseString(context.Background(), input)
			if tree == nil {
				b.Fatal("parse failed")
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				countNodes(tree.RootNode())
			}
		})
	}
}

func countNodes(n ts.Node) int {
	count := 1
	for i := 0; i < int(n.ChildCount()); i++ {
		count += countNodes(n.Child(i))
	}
	return count
}

// --- S-Expression Benchmarks ---

func BenchmarkSExpression_1KB(b *testing.B) {
	input := generateJSON(1024)
	lang := tg.JSONLanguage()

	parser := ts.NewParser()
	parser.SetLanguage(lang)
	tree := parser.ParseString(context.Background(), input)
	if tree == nil {
		b.Fatal("parse failed")
	}
	root := tree.RootNode()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = root.String()
	}
}

// --- Memory Profiling ---

func TestAllocationsPerParse(t *testing.T) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	lang := tg.JSONLanguage()

	for _, s := range sizes {
		t.Run(s.name, func(t *testing.T) {
			input := generateJSON(s.size)
			parser := ts.NewParser()
			parser.SetLanguage(lang)

			// Warm up
			for i := 0; i < 3; i++ {
				parser.ParseString(context.Background(), input)
			}

			// Flush pending finalizers for cleaner measurements.
			runtime.GC()

			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			allocsBefore := stats.Mallocs
			bytesBefore := stats.TotalAlloc

			iterations := 100
			for i := 0; i < iterations; i++ {
				parser.ParseString(context.Background(), input)
			}

			runtime.ReadMemStats(&stats)
			allocsPerParse := (stats.Mallocs - allocsBefore) / uint64(iterations)
			bytesPerParse := (stats.TotalAlloc - bytesBefore) / uint64(iterations)

			t.Logf("input size: %d bytes", len(input))
			t.Logf("allocations per parse: %d", allocsPerParse)
			t.Logf("bytes allocated per parse: %d", bytesPerParse)
			t.Logf("bytes per input byte: %.2f", float64(bytesPerParse)/float64(len(input)))
		})
	}
}

// --- Parser Reuse Benchmark ---

func BenchmarkParserReuse(b *testing.B) {
	input := generateJSON(10 * 1024)
	lang := tg.JSONLanguage()

	b.Run("reuse", func(b *testing.B) {
		parser := ts.NewParser()
		parser.SetLanguage(lang)
		b.SetBytes(int64(len(input)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser.ParseString(context.Background(), input)
		}
	})

	b.Run("new-each-time", func(b *testing.B) {
		b.SetBytes(int64(len(input)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser := ts.NewParser()
			parser.SetLanguage(lang)
			parser.ParseString(context.Background(), input)
		}
	})
}

// --- Parallel Parse Benchmark ---

func BenchmarkParallelParse(b *testing.B) {
	input := generateJSON(10 * 1024)
	lang := tg.JSONLanguage()

	for _, goroutines := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("goroutines-%d", goroutines), func(b *testing.B) {
			b.SetBytes(int64(len(input)) * int64(goroutines))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				wg.Add(goroutines)
				for g := 0; g < goroutines; g++ {
					go func() {
						defer wg.Done()
						parser := ts.NewParser()
						parser.SetLanguage(lang)
						parser.ParseString(context.Background(), input)
					}()
				}
				wg.Wait()
			}
		})
	}
}

// --- Chunked Input Parse Benchmark ---

func BenchmarkParseChunkedInput(b *testing.B) {
	input := generateJSON(10 * 1024)
	lang := tg.JSONLanguage()

	for _, chunkSize := range []int{64, 256, 1024, 4096} {
		b.Run(fmt.Sprintf("chunk-%d", chunkSize), func(b *testing.B) {
			parser := ts.NewParser()
			parser.SetLanguage(lang)

			chunked := &chunkedInput{data: input, chunkSize: chunkSize}

			tree := parser.Parse(context.Background(), chunked, nil)
			if tree == nil {
				b.Fatal("chunked parse failed")
			}

			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				parser.Parse(context.Background(), chunked, nil)
			}
		})
	}
}

type chunkedInput struct {
	data      []byte
	chunkSize int
}

func (c *chunkedInput) Read(byteOffset uint32, _ ts.Point) []byte {
	if int(byteOffset) >= len(c.data) {
		return nil
	}
	end := int(byteOffset) + c.chunkSize
	if end > len(c.data) {
		end = len(c.data)
	}
	return c.data[byteOffset:end]
}

// --- GC Impact Measurement ---

func TestGCImpact(t *testing.T) {
	input := generateJSON(100 * 1024)
	lang := tg.JSONLanguage()

	parser := ts.NewParser()
	parser.SetLanguage(lang)

	for i := 0; i < 5; i++ {
		parser.ParseString(context.Background(), input)
	}

	runtime.GC()
	prev := debug.SetGCPercent(100)
	t.Cleanup(func() { debug.SetGCPercent(prev) })

	iterations := 50
	var totalPause time.Duration
	var maxPause time.Duration

	var statsBefore, statsAfter debug.GCStats
	debug.ReadGCStats(&statsBefore)

	for i := 0; i < iterations; i++ {
		parser.ParseString(context.Background(), input)
	}

	debug.ReadGCStats(&statsAfter)

	gcCount := int(statsAfter.NumGC - statsBefore.NumGC)
	if gcCount > len(statsAfter.Pause) {
		gcCount = len(statsAfter.Pause)
	}
	newPauses := statsAfter.Pause[:gcCount]
	for _, p := range newPauses {
		totalPause += p
		if p > maxPause {
			maxPause = p
		}
	}

	t.Logf("GC cycles during %d parses: %d", iterations, gcCount)
	if gcCount > 0 {
		t.Logf("total GC pause: %v", totalPause)
		t.Logf("avg GC pause: %v", totalPause/time.Duration(gcCount))
		t.Logf("max GC pause: %v", maxPause)
	}
}

// --- Scaling Benchmark (parse time vs input size) ---

func BenchmarkParseScaling(b *testing.B) {
	lang := tg.JSONLanguage()

	for _, size := range []int{256, 512, 1024, 2048, 4096, 8192, 16384} {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			input := generateJSON(size)
			parser := ts.NewParser()
			parser.SetLanguage(lang)

			tree := parser.ParseString(context.Background(), input)
			if tree == nil {
				b.Fatal("parse failed")
			}

			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				parser.ParseString(context.Background(), input)
			}
		})
	}
}

// ============================================================================
// Input generators for all 15 languages
// ============================================================================

func generateJSON(targetBytes int) []byte {
	pairSize := 30
	numPairs := targetBytes / pairSize
	if numPairs < 1 {
		numPairs = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	b.WriteString("{\n")
	for i := 0; i < numPairs; i++ {
		if i > 0 {
			b.WriteString(",\n")
		}
		fmt.Fprintf(&b, `  "key%05d": "value%05d"`, i, i)
	}
	b.WriteString("\n}")
	return []byte(b.String())
}

func generateNestedJSON(targetBytes int) []byte {
	depth := targetBytes / 4
	if depth > 500 {
		depth = 500
	}
	var b strings.Builder
	b.Grow(depth*2 + 20)
	for i := 0; i < depth; i++ {
		b.WriteByte('[')
	}
	b.WriteString("1")
	for i := 0; i < depth; i++ {
		b.WriteByte(']')
	}
	return []byte(b.String())
}

func generateGo(targetBytes int) []byte {
	// ~55 bytes per function
	unit := "func f%d(x int) int {\n\treturn x + %d\n}\n\n"
	unitSize := 45
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	b.WriteString("package bench\n\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generatePython(targetBytes int) []byte {
	unit := "def func_%d(x):\n    return x + %d\n\n"
	unitSize := 35
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateJavaScript(targetBytes int) []byte {
	unit := "function func_%d(x) {\n  return x + %d;\n}\n\n"
	unitSize := 42
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateTypeScript(targetBytes int) []byte {
	unit := "function func_%d(x: number): number {\n  return x + %d;\n}\n\n"
	unitSize := 55
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateC(targetBytes int) []byte {
	unit := "int func_%d(int x) {\n    return x + %d;\n}\n\n"
	unitSize := 42
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateCpp(targetBytes int) []byte {
	unit := "int func_%d(int x) {\n    return x + %d;\n}\n\n"
	unitSize := 42
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	b.WriteString("namespace bench {\n\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	b.WriteString("} // namespace bench\n")
	return []byte(b.String())
}

func generateRust(targetBytes int) []byte {
	unit := "fn func_%d(x: i32) -> i32 {\n    x + %d\n}\n\n"
	unitSize := 42
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateJava(targetBytes int) []byte {
	unit := "    public static int func_%d(int x) {\n        return x + %d;\n    }\n\n"
	unitSize := 65
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	b.WriteString("public class Bench {\n\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func generateRuby(targetBytes int) []byte {
	unit := "def func_%d(x)\n  x + %d\nend\n\n"
	unitSize := 30
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateBash(targetBytes int) []byte {
	unit := "func_%d() {\n  echo %d\n}\n\n"
	unitSize := 28
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	b.WriteString("#!/bin/bash\n\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateCSS(targetBytes int) []byte {
	unit := ".class_%d {\n  color: red;\n  margin: %dpx;\n}\n\n"
	unitSize := 45
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateHTML(targetBytes int) []byte {
	unit := "<div id=\"d%d\"><span>text %d</span></div>\n"
	unitSize := 42
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 200)
	b.WriteString("<html><body>\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	b.WriteString("</body></html>\n")
	return []byte(b.String())
}

func generatePerl(targetBytes int) []byte {
	unit := "sub func_%d {\n    my $x = shift;\n    return $x + %d;\n}\n\n"
	unitSize := 55
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	b.WriteString("use strict;\nuse warnings;\n\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

func generateLua(targetBytes int) []byte {
	unit := "function func_%d(x)\n  return x + %d\nend\n\n"
	unitSize := 40
	count := targetBytes / unitSize
	if count < 1 {
		count = 1
	}
	var b strings.Builder
	b.Grow(targetBytes + 100)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, unit, i, i)
	}
	return []byte(b.String())
}

// writeBenchFixtureFiles generates and writes benchmark fixture files to testdata/bench/.
// This is a helper for manual use — called via TestGenerateBenchFixtures.
func TestGenerateBenchFixtures(t *testing.T) {
	if os.Getenv("GENERATE_BENCH_FIXTURES") == "" {
		t.Skip("set GENERATE_BENCH_FIXTURES=1 to generate fixture files")
	}

	dir := filepath.Join("testdata", "bench")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	sizes := []struct {
		prefix string
		bytes  int
	}{
		{"small", 1024},
		{"medium", 10 * 1024},
		{"large", 100 * 1024},
	}

	for _, lang := range benchLanguages() {
		for _, size := range sizes {
			input := lang.generate(size.bytes)
			name := fmt.Sprintf("%s%s", size.prefix, lang.ext)
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, input, 0o644); err != nil {
				t.Fatalf("writing %s: %v", path, err)
			}
			t.Logf("wrote %s (%d bytes)", path, len(input))
		}
	}
}
