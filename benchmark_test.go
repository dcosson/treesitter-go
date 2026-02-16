package treesitter_test

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/testgrammars"
)

// generateJSON generates a JSON object with the specified approximate byte size.
func generateJSON(targetBytes int) []byte {
	// Each pair is approximately: `"keyXXXXX": "valueXXXXX",\n` = ~30 bytes
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

// generateNestedJSON generates deeply nested JSON arrays.
func generateNestedJSON(targetBytes int) []byte {
	// Create nested arrays: [[[[...]]]]
	depth := targetBytes / 4 // each level ~= "[" + "]" = 2 chars, plus nesting
	if depth > 500 {
		depth = 500 // limit depth to avoid stack issues
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

// --- Parse Benchmarks ---

func BenchmarkParseJSON_1KB(b *testing.B) {
	benchmarkParseJSON(b, 1024)
}

func BenchmarkParseJSON_10KB(b *testing.B) {
	benchmarkParseJSON(b, 10*1024)
}

func BenchmarkParseJSON_100KB(b *testing.B) {
	benchmarkParseJSON(b, 100*1024)
}

func BenchmarkParseJSON_1MB(b *testing.B) {
	benchmarkParseJSON(b, 1024*1024)
}

func benchmarkParseJSON(b *testing.B, size int) {
	input := generateJSON(size)
	lang := testgrammars.JSONLanguage()

	parser := ts.NewParser()
	parser.SetLanguage(lang)

	// Warm up: verify parsing works.
	tree := parser.ParseString(context.Background(), input)
	if tree == nil {
		b.Fatalf("parse failed for %d byte input", size)
	}

	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser.ParseString(context.Background(), input)
	}
}

// --- Nested JSON Benchmarks (stress-test deep parse stacks) ---

func BenchmarkParseNestedJSON_500(b *testing.B) {
	input := generateNestedJSON(2000) // ~500 depth
	lang := testgrammars.JSONLanguage()

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

func BenchmarkTreeTraversal_1KB(b *testing.B) {
	benchmarkTreeTraversal(b, 1024)
}

func BenchmarkTreeTraversal_10KB(b *testing.B) {
	benchmarkTreeTraversal(b, 10*1024)
}

func benchmarkTreeTraversal(b *testing.B, size int) {
	input := generateJSON(size)
	lang := testgrammars.JSONLanguage()

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
	lang := testgrammars.JSONLanguage()

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

	lang := testgrammars.JSONLanguage()

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
	// Test that reusing a parser is faster than creating a new one each time
	input := generateJSON(10 * 1024)
	lang := testgrammars.JSONLanguage()

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
	lang := testgrammars.JSONLanguage()

	for _, goroutines := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("goroutines-%d", goroutines), func(b *testing.B) {
			// Total throughput across all goroutines (aggregate, not per-goroutine).
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
	lang := testgrammars.JSONLanguage()

	for _, chunkSize := range []int{64, 256, 1024, 4096} {
		b.Run(fmt.Sprintf("chunk-%d", chunkSize), func(b *testing.B) {
			parser := ts.NewParser()
			parser.SetLanguage(lang)

			chunked := &chunkedInput{data: input, chunkSize: chunkSize}

			// Verify parsing works.
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

// chunkedInput implements ts.Input, returning data in fixed-size chunks.
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
	lang := testgrammars.JSONLanguage()

	parser := ts.NewParser()
	parser.SetLanguage(lang)

	// Warm up.
	for i := 0; i < 5; i++ {
		parser.ParseString(context.Background(), input)
	}

	// Force GC to start clean.
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
	// Pause is a circular buffer of recent pauses, most-recent-first.
	// Clamp to available length to avoid OOB if many GC cycles occurred.
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
	lang := testgrammars.JSONLanguage()

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
