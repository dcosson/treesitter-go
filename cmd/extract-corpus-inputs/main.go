// Command extract-corpus-inputs extracts individual test inputs from tree-sitter
// corpus files, writing each test's source code to a separate file.
//
// This replaces the inline Python extractor that was previously embedded in
// generate-scanner-traces.sh, ensuring exact byte-level consistency with our
// Go corpus parser (internal/corpustest).
//
// Usage:
//
//	go run ./cmd/extract-corpus-inputs --lang bash --output-dir /tmp/inputs/bash --ext sh testdata/grammars/tree-sitter-bash/test/corpus
//
// Each test input is written as {index:04d}_{sanitized_name}.{ext}
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/treesitter-go/treesitter/internal/corpustest"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func main() {
	lang := flag.String("lang", "", "Language name (for logging)")
	fileExt := flag.String("ext", "txt", "File extension for output files")
	outputDir := flag.String("output-dir", "", "Directory to write extracted inputs")
	flag.Parse()

	corpusDirs := flag.Args()
	if len(corpusDirs) == 0 || *outputDir == "" {
		fmt.Fprintf(os.Stderr, "Usage: extract-corpus-inputs --output-dir DIR [--lang NAME] [--ext EXT] CORPUS_DIR [CORPUS_DIR...]\n")
		os.Exit(1)
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
		os.Exit(1)
	}

	testNum := 0
	for _, corpusDir := range corpusDirs {
		entries, err := os.ReadDir(corpusDir)
		if err != nil {
			// Skip missing directories silently (some languages have optional corpus dirs)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			entryExt := filepath.Ext(entry.Name())
			// Accept .txt files and extensionless files (some grammars like Perl
			// use extensionless corpus files in the standard format).
			if entryExt != ".txt" && entryExt != "" {
				continue
			}

			data, err := os.ReadFile(filepath.Join(corpusDir, entry.Name()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", entry.Name(), err)
				continue
			}

			cases, err := corpustest.ParseCorpusFile(data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", entry.Name(), err)
				continue
			}

			for _, tc := range cases {
				safeName := sanitizeName(tc.Name)
				filename := fmt.Sprintf("%04d_%s.%s", testNum, safeName, *fileExt)
				outPath := filepath.Join(*outputDir, filename)
				if err := os.WriteFile(outPath, tc.Input, 0o644); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", filename, err)
				}
				testNum++
			}
		}
	}

	langLabel := *lang
	if langLabel == "" {
		langLabel = "unknown"
	}
	fmt.Printf("Extracted %d test inputs for %s\n", testNum, langLabel)
}

func sanitizeName(name string) string {
	s := sanitizeRe.ReplaceAllString(name, "_")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
