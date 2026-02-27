// Package difftest provides differential testing between the Go tree-sitter
// implementation and the reference C tree-sitter CLI. It parses the same
// input with both implementations and compares the normalized S-expression
// output, reporting any divergences.
package difftest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/treesitter-go/treesitter/internal/corpustest"
)

// TreeSitterCLI is the path to the tree-sitter CLI binary.
// Set via -ts-cli flag, TS_CLI_PATH environment variable, or directly.
// Defaults to "tree-sitter" (auto-discovered via PATH).
var TreeSitterCLI = "tree-sitter"

// GrammarBaseDir is the directory containing fetched grammar repos (tree-sitter-<lang>/).
// The tree-sitter CLI uses -p (--grammar-path) to compile and load grammars from here.
// Defaults to "build/grammars" relative to the repo root.
var GrammarBaseDir = "build/grammars"

// Scope maps file extensions to tree-sitter --scope arguments.
// Kept for backward compatibility; grammar path resolution is preferred.
var Scope = map[string]string{
	".json": "source.json",
	".js":   "source.js",
	".ts":   "source.ts",
	".go":   "source.go",
	".py":   "source.python",
	".c":    "source.c",
	".cpp":  "source.cpp",
	".rs":   "source.rust",
	".rb":   "source.ruby",
	".java": "source.java",
	".css":  "source.css",
	".html": "text.html.basic",
	".sh":   "source.bash",
	".lua":  "source.luau",
	".pl":   "source.perl",
	".pm":   "source.perl",
	".h":    "source.c",
	".cc":   "source.cpp",
}

// grammarManifestEntry matches the fields we need from grammars.json.
type grammarManifestEntry struct {
	Name    string `json:"name"`
	Ext     string `json:"ext"`
	Subpath string `json:"subpath,omitempty"`
}

var (
	grammarPathMap     map[string]string // ext → grammar source dir
	grammarPathMapOnce sync.Once
)

// loadGrammarPathMap reads grammars.json and builds ext → grammar source dir mapping.
// Paths are resolved relative to grammars.json location so tests work from any directory.
func loadGrammarPathMap() map[string]string {
	grammarPathMapOnce.Do(func() {
		grammarPathMap = make(map[string]string)

		// Try grammars.json in repo root (may be running from e2etest/ or internal/difftest/).
		candidates := []string{
			"grammars.json",
			"../grammars.json",
			"../../grammars.json",
		}
		var data []byte
		var repoRoot string
		for _, c := range candidates {
			var err error
			data, err = os.ReadFile(c)
			if err == nil {
				repoRoot = filepath.Dir(c)
				if repoRoot == "." {
					repoRoot = ""
				}
				break
			}
		}
		if data == nil {
			return
		}

		var entries []grammarManifestEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return
		}

		baseDir := GrammarBaseDir
		if repoRoot != "" {
			baseDir = filepath.Join(repoRoot, baseDir)
		}

		for _, e := range entries {
			grammarDir := filepath.Join(baseDir, "tree-sitter-"+e.Name)
			if e.Subpath != "" {
				grammarDir = filepath.Join(grammarDir, e.Subpath)
			}
			grammarPathMap[e.Ext] = grammarDir
		}

		// Map alternate extensions to the same grammar directory.
		// grammars.json only stores a single ext per grammar, but some
		// languages use multiple file extensions.
		altExts := map[string]string{
			".h":  ".c",   // C headers (also used by C++, but C is the common case)
			".cc": ".cpp", // C++ alternate extension
			".pm": ".pl",  // Perl modules
		}
		for alt, primary := range altExts {
			if _, exists := grammarPathMap[alt]; !exists {
				if gp, ok := grammarPathMap[primary]; ok {
					grammarPathMap[alt] = gp
				}
			}
		}
	})
	return grammarPathMap
}

// grammarPathForExt returns the grammar source directory for a file extension,
// or empty string if not found.
func grammarPathForExt(ext string) string {
	m := loadGrammarPathMap()
	return m[ext]
}

// ParseWithCLI parses a source file using the tree-sitter CLI and returns
// the S-expression output. It uses -p (--grammar-path) to point the CLI at
// the grammar source directory for compilation. Falls back to --scope if
// no grammar path is available.
func ParseWithCLI(filePath, scope string) (string, error) {
	args := []string{"parse"}

	ext := filepath.Ext(filePath)
	if gp := grammarPathForExt(ext); gp != "" {
		args = append(args, "-p", gp)
	} else if scope != "" {
		args = append(args, "--scope", scope)
	}
	args = append(args, filePath)

	cmd := exec.Command(TreeSitterCLI, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// tree-sitter parse returns exit code 1 when the parse tree contains errors,
		// but still produces valid output. Only fail on other errors.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 && stdout.Len() > 0 {
				// Parse succeeded but tree has errors - still usable.
				return stdout.String(), nil
			}
		}
		return "", fmt.Errorf("tree-sitter parse failed: %v\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// ParseBytesWithCLI writes input bytes to a temp file and parses with CLI.
func ParseBytesWithCLI(input []byte, scope string) (string, error) {
	// Determine extension from scope for temp file naming.
	ext := ".txt"
	for e, s := range Scope {
		if s == scope {
			ext = e
			break
		}
	}

	tmpFile, err := os.CreateTemp("", "difftest-*"+ext)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(input); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("writing temp file: %w", err)
	}
	tmpFile.Close()

	return ParseWithCLI(tmpFile.Name(), scope)
}

// NormalizeCLIOutput normalizes the tree-sitter CLI output for comparison
// with the Go parser. Strips point ranges, field annotations, trailing
// filename paths (appended by the CLI), collapses whitespace, and trims.
func NormalizeCLIOutput(s string) string {
	normalized, _ := corpustest.NormalizeSExpression(s)
	stripped := corpustest.StripFields(normalized)
	// The tree-sitter CLI appends the filename after the S-expression.
	// After normalization this looks like "...) /path/to/file.ext".
	// Find the position where the root S-expression closes and truncate.
	stripped = stripTrailingAfterSExpr(stripped)
	return stripped
}

// stripTrailingAfterSExpr removes any text after the root S-expression's
// final closing paren. The tree-sitter CLI appends the filename path.
func stripTrailingAfterSExpr(s string) string {
	depth := 0
	for i, ch := range s {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return s
}

// CompareResult holds the result of comparing two parse trees.
type CompareResult struct {
	Input      []byte
	GoSExpr    string
	CSExpr     string
	GoNorm     string
	CNorm      string
	Match      bool
	Divergence string // Human-readable description of the first difference.
}

// Compare parses input with both implementations and compares.
func Compare(input []byte, scope string, goParseFunc corpustest.ParseFunc) (*CompareResult, error) {
	// Parse with Go.
	goSExpr, err := goParseFunc(input)
	if err != nil {
		return nil, fmt.Errorf("Go parser: %w", err)
	}

	// Parse with C tree-sitter CLI.
	cSExpr, err := ParseBytesWithCLI(input, scope)
	if err != nil {
		return nil, fmt.Errorf("C tree-sitter: %w", err)
	}

	// Normalize both for comparison.
	goNorm := NormalizeCLIOutput(goSExpr)
	cNorm := NormalizeCLIOutput(cSExpr)

	result := &CompareResult{
		Input:   input,
		GoSExpr: goSExpr,
		CSExpr:  cSExpr,
		GoNorm:  goNorm,
		CNorm:   cNorm,
		Match:   goNorm == cNorm,
	}

	if !result.Match {
		result.Divergence = findFirstDivergence(goNorm, cNorm)
	}

	return result, nil
}

// RunDifferentialCorpus runs corpus tests with differential comparison.
// For each corpus test case, it parses with both Go and C and reports
// any divergences.
func RunDifferentialCorpus(t *testing.T, cases []corpustest.TestCase, scope string, goParseFunc corpustest.ParseFunc) {
	t.Helper()

	if _, err := exec.LookPath(TreeSitterCLI); err != nil {
		t.Skipf("tree-sitter CLI not found in PATH: %v", err)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Helper()
			if tc.Attributes.Skip {
				t.Skip("corpus test marked :skip")
			}
			if !tc.Attributes.Platform {
				t.Skip("corpus test not for this platform")
			}

			result, err := Compare(tc.Input, scope, goParseFunc)
			if err != nil {
				t.Fatalf("differential compare: %v", err)
			}

			if !result.Match {
				t.Errorf("differential divergence\ninput:\n  %s\nGo:\n  %s\nC:\n  %s\nfirst diff:\n  %s",
					abbreviate(string(tc.Input), 200),
					result.GoNorm,
					result.CNorm,
					result.Divergence)
			}
		})
	}
}

// RunDifferentialFile parses a single file with both implementations and compares.
func RunDifferentialFile(t *testing.T, filePath string, goParseFunc corpustest.ParseFunc) {
	t.Helper()

	if _, err := exec.LookPath(TreeSitterCLI); err != nil {
		t.Skipf("tree-sitter CLI not found in PATH: %v", err)
	}

	input, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading %s: %v", filePath, err)
	}

	ext := filepath.Ext(filePath)
	scope := Scope[ext]

	result, err := Compare(input, scope, goParseFunc)
	if err != nil {
		t.Fatalf("differential compare: %v", err)
	}

	if !result.Match {
		t.Errorf("differential divergence for %s\nGo:\n  %s\nC:\n  %s\nfirst diff:\n  %s",
			filePath,
			abbreviate(result.GoNorm, 500),
			abbreviate(result.CNorm, 500),
			result.Divergence)
	}
}

// RunDifferentialDir runs differential testing on all files in a directory
// matching the given extensions.
func RunDifferentialDir(t *testing.T, dir string, extensions []string, goParseFunc corpustest.ParseFunc) {
	t.Helper()

	if _, err := exec.LookPath(TreeSitterCLI); err != nil {
		t.Skipf("tree-sitter CLI not found in PATH: %v", err)
	}

	extSet := make(map[string]bool)
	for _, e := range extensions {
		extSet[e] = true
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !extSet[filepath.Ext(path)] {
			return nil
		}

		relPath, _ := filepath.Rel(dir, path)
		t.Run(relPath, func(t *testing.T) {
			RunDifferentialFile(t, path, goParseFunc)
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking directory %s: %v", dir, err)
	}
}

// findFirstDivergence finds the first point where two normalized S-expressions differ.
func findFirstDivergence(a, b string) string {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	pos := 0
	for pos < minLen && a[pos] == b[pos] {
		pos++
	}

	// Show context around the divergence.
	contextStart := pos - 30
	if contextStart < 0 {
		contextStart = 0
	}
	contextEnd := pos + 30
	aEnd := contextEnd
	if aEnd > len(a) {
		aEnd = len(a)
	}
	bEnd := contextEnd
	if bEnd > len(b) {
		bEnd = len(b)
	}

	prefix := ""
	if contextStart > 0 {
		prefix = "..."
	}

	return fmt.Sprintf("at position %d:\n    Go: %s%q\n    C:  %s%q",
		pos,
		prefix, a[contextStart:aEnd],
		prefix, b[contextStart:bEnd])
}

// abbreviate truncates a string to maxLen, appending "..." if truncated.
func abbreviate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// WriteRegressionFile writes a regression test input+expected pair.
func WriteRegressionFile(dir, name string, input []byte, expectedSExpr string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	inputPath := filepath.Join(dir, name+".input")
	expectedPath := filepath.Join(dir, name+".expected")

	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		return err
	}
	return os.WriteFile(expectedPath, []byte(expectedSExpr), 0o644)
}

// RunRegressionTests loads .input/.expected pairs from a directory and
// verifies the Go parser matches the expected output.
func RunRegressionTests(t *testing.T, dir string, goParseFunc corpustest.ParseFunc) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("regression directory not found")
		}
		t.Fatalf("reading %s: %v", dir, err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".input") {
			continue
		}

		baseName := strings.TrimSuffix(entry.Name(), ".input")
		inputPath := filepath.Join(dir, entry.Name())
		expectedPath := filepath.Join(dir, baseName+".expected")

		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			continue // No expected file, skip.
		}

		t.Run(baseName, func(t *testing.T) {
			t.Helper()

			input, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("reading input: %v", err)
			}
			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("reading expected: %v", err)
			}

			actual, err := goParseFunc(input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			expectedNorm := NormalizeCLIOutput(string(expected))
			actualNorm := NormalizeCLIOutput(actual)

			if actualNorm != expectedNorm {
				t.Errorf("regression mismatch\nexpected:\n  %s\nactual:\n  %s",
					expectedNorm, actualNorm)
			}
		})
	}
}
