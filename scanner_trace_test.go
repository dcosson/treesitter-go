package treesitter_test

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/corpustest"
	bashgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/bash"
	cppgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cppgrammar"
	cssgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/css"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/html"
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

// traceEntry represents a single recorded external scanner call from the
// C reference implementation. Each entry is independently replayable.
type traceEntry struct {
	Lang     string     `json:"lang"`
	File     string     `json:"file"`
	CallIdx  uint64     `json:"call_index"`
	Input    traceInput `json:"input"`
	Output   traceOutput `json:"output"`
}

type traceInput struct {
	ByteOffset         uint32  `json:"byte_offset"`
	Lookahead          int32   `json:"lookahead"`
	ValidSymbols       []int   `json:"valid_symbols"`
	ScannerStateBefore string  `json:"scanner_state_before"` // base64
}

type traceOutput struct {
	Matched           bool   `json:"matched"`
	ResultSymbol      uint16 `json:"result_symbol"`
	TokenEndByte      uint32 `json:"token_end_byte"`
	Advances          uint32 `json:"advances"`
	ScannerStateAfter string `json:"scanner_state_after"` // base64
}

// scannerLangConfig bundles a language name with its scanner factory and
// the grammar's external token count (for validSymbols sizing).
type scannerLangConfig struct {
	name       string
	repoName   string // e.g. "tree-sitter-python"
	newScanner ts.ExternalScannerFactory
	corpusDirs []string // relative to testdata/grammars/<repoName>/
}

func scannerLanguages() []scannerLangConfig {
	return []scannerLangConfig{
		{name: "bash", repoName: "tree-sitter-bash", newScanner: bashscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "cpp", repoName: "tree-sitter-cpp", newScanner: cppscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "css", repoName: "tree-sitter-css", newScanner: cssscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "html", repoName: "tree-sitter-html", newScanner: htmlscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "javascript", repoName: "tree-sitter-javascript", newScanner: jsscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "lua", repoName: "tree-sitter-lua", newScanner: luascanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "perl", repoName: "tree-sitter-perl", newScanner: perlscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "python", repoName: "tree-sitter-python", newScanner: pyscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "ruby", repoName: "tree-sitter-ruby", newScanner: rubyscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "rust", repoName: "tree-sitter-rust", newScanner: rustscanner.New, corpusDirs: []string{"test/corpus"}},
		{name: "typescript", repoName: "tree-sitter-typescript", newScanner: tsscanner.New, corpusDirs: []string{"typescript/test/corpus", "common/test/corpus"}},
	}
}

// languageForName returns the Language struct for the given scanner language.
// This is needed so we know external_token_count for validSymbols sizing.
func languageForName(name string) *ts.Language {
	switch name {
	case "bash":
		lang := bashgrammar.BashLanguage()
		lang.NewExternalScanner = bashscanner.New
		return lang
	case "cpp":
		lang := cppgrammar.CppLanguage()
		lang.NewExternalScanner = cppscanner.New
		return lang
	case "css":
		lang := cssgrammar.CssLanguage()
		lang.NewExternalScanner = cssscanner.New
		return lang
	case "html":
		lang := htmlgrammar.HtmlLanguage()
		lang.NewExternalScanner = htmlscanner.New
		return lang
	case "javascript":
		lang := jsgrammar.JavascriptLanguage()
		lang.NewExternalScanner = jsscanner.New
		return lang
	case "lua":
		lang := luagrammar.LuaLanguage()
		lang.NewExternalScanner = luascanner.New
		return lang
	case "perl":
		lang := perlgrammar.PerlLanguage()
		lang.NewExternalScanner = perlscanner.New
		return lang
	case "python":
		lang := pygrammar.PythonLanguage()
		lang.NewExternalScanner = pyscanner.New
		return lang
	case "ruby":
		lang := rubygrammar.RubyLanguage()
		lang.NewExternalScanner = rubyscanner.New
		return lang
	case "rust":
		lang := rustgrammar.RustLanguage()
		lang.NewExternalScanner = rustscanner.New
		return lang
	case "typescript":
		lang := tsgrammar.TypescriptLanguage()
		lang.NewExternalScanner = tsscanner.New
		return lang
	default:
		return nil
	}
}

// loadCorpusInputs loads all corpus test inputs for a language, returning
// a map from test file basename (matching trace entry "file" field) to
// the input bytes. The key format matches the trace generator script's naming:
// "{index:04d}_{sanitized_name}" where index restarts at 0 per corpus .txt file.
func loadCorpusInputs(grammarsDir string, cfg scannerLangConfig) (map[string][]byte, error) {
	inputs := make(map[string][]byte)

	for _, corpusRel := range cfg.corpusDirs {
		corpusDir := filepath.Join(grammarsDir, cfg.repoName, corpusRel)
		if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
			continue
		}

		// Read corpus files in the same order as the trace generator (glob *.txt)
		files, err := filepath.Glob(filepath.Join(corpusDir, "*.txt"))
		if err != nil {
			return nil, err
		}

		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", f, err)
			}
			cases, err := corpustest.ParseCorpusFile(data)
			if err != nil {
				return nil, fmt.Errorf("parsing %s: %w", f, err)
			}

			// Number tests per-file starting from 0, matching the trace generator
			for i, tc := range cases {
				safeName := sanitizeTestName(tc.Name)
				key := fmt.Sprintf("%04d_%s", i, safeName)
				inputs[key] = tc.Input
			}
		}
	}

	return inputs, nil
}

// sanitizeTestName matches the Python sanitization in generate-scanner-traces.sh
func sanitizeTestName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name) && len(result) < 80; i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}

func TestScannerTraces(t *testing.T) {
	tracesDir := filepath.Join("testdata", "scanner-traces")
	grammarsDir := filepath.Join("testdata", "grammars")

	if _, err := os.Stat(tracesDir); os.IsNotExist(err) {
		t.Skip("no scanner-traces directory — run 'make generate-scanner-traces' first")
	}

	for _, cfg := range scannerLanguages() {
		cfg := cfg
		traceFile := filepath.Join(tracesDir, cfg.name+".jsonl")

		if _, err := os.Stat(traceFile); os.IsNotExist(err) {
			t.Logf("no trace file for %s, skipping", cfg.name)
			continue
		}

		t.Run(cfg.name, func(t *testing.T) {
			// Load corpus inputs for this language
			corpusInputs, err := loadCorpusInputs(grammarsDir, cfg)
			if err != nil {
				t.Fatalf("failed to load corpus inputs: %v", err)
			}
			if len(corpusInputs) == 0 {
				t.Skipf("no corpus inputs for %s — run 'make fetch-test-grammars'", cfg.name)
			}

			// Open and scan the trace file
			f, err := os.Open(traceFile)
			if err != nil {
				t.Fatalf("failed to open trace file: %v", err)
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB lines

			entryCount := 0
			failCount := 0
			maxFailsPerLang := 20 // don't flood output

			for scanner.Scan() {
				var entry traceEntry
				if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
					t.Errorf("line %d: invalid JSON: %v", entryCount+1, err)
					entryCount++
					continue
				}
				entryCount++

				// Find the corpus input for this trace entry
				inputBytes, ok := corpusInputs[entry.File]
				if !ok {
					// Try without the number prefix in case naming differs
					t.Logf("entry %d: corpus input not found for file %q, skipping", entryCount, entry.File)
					continue
				}

				// Decode pre-scan state
				preState, err := base64.StdEncoding.DecodeString(entry.Input.ScannerStateBefore)
				if err != nil {
					t.Errorf("entry %d: bad base64 pre-state: %v", entryCount, err)
					continue
				}

				// Create a fresh scanner and deserialize to the recorded state
				goScanner := cfg.newScanner()
				goScanner.Deserialize(preState)

				// Build valid_symbols bool slice
				validSymbols := make([]bool, len(entry.Input.ValidSymbols))
				for i, v := range entry.Input.ValidSymbols {
					validSymbols[i] = v != 0
				}

				// Create a real Lexer positioned at the recorded byte offset.
				// The C trace records lookahead as the Unicode code point (int32).
				// In the C implementation, EOF is lookahead == 0.
				// In Go, EOF is Lookahead == -1.
				lexer := ts.NewLexer()
				lexer.SetInput(ts.NewStringInput(inputBytes))
				lexer.Start(ts.Length{Bytes: entry.Input.ByteOffset})

				// Call the Go scanner
				matched := goScanner.Scan(lexer, validSymbols)

				// Compare results
				if matched != entry.Output.Matched {
					failCount++
					if failCount <= maxFailsPerLang {
						t.Errorf("entry %d (file=%s, call=%d, offset=%d): matched=%v, want %v",
							entryCount, entry.File, entry.CallIdx, entry.Input.ByteOffset,
							matched, entry.Output.Matched)
					}
					continue
				}

				if matched {
					// Compare result symbol
					if uint16(lexer.ResultSymbol) != entry.Output.ResultSymbol {
						failCount++
						if failCount <= maxFailsPerLang {
							t.Errorf("entry %d (file=%s, call=%d): result_symbol=%d, want %d",
								entryCount, entry.File, entry.CallIdx,
								lexer.ResultSymbol, entry.Output.ResultSymbol)
						}
						continue
					}
				}

				// Compare post-scan serialized state
				expectedPostState, err := base64.StdEncoding.DecodeString(entry.Output.ScannerStateAfter)
				if err != nil {
					t.Errorf("entry %d: bad base64 post-state: %v", entryCount, err)
					continue
				}

				var postBuf [1024]byte
				postLen := goScanner.Serialize(postBuf[:])
				actualPostState := postBuf[:postLen]

				if !bytesEqual(actualPostState, expectedPostState) {
					failCount++
					if failCount <= maxFailsPerLang {
						t.Errorf("entry %d (file=%s, call=%d): post-state mismatch (got %d bytes, want %d bytes)",
							entryCount, entry.File, entry.CallIdx,
							len(actualPostState), len(expectedPostState))
					}
				}
			}

			if err := scanner.Err(); err != nil {
				t.Fatalf("error reading trace file: %v", err)
			}

			if failCount > maxFailsPerLang {
				t.Errorf("... and %d more failures (showing first %d)", failCount-maxFailsPerLang, maxFailsPerLang)
			}

			t.Logf("%s: %d entries, %d failures", cfg.name, entryCount, failCount)
		})
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
