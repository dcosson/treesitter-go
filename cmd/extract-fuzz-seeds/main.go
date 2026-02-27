// Command extract-fuzz-seeds extracts individual test inputs from tree-sitter
// corpus files and writes them as individual seed files for Go's native fuzzer.
//
// Usage:
//
//	go run ./cmd/extract-fuzz-seeds
//
// This creates testdata/fuzz/corpus/<lang>/<hash>.seed files that Go's fuzzer
// automatically picks up as seed corpus entries.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/treesitter-go/treesitter/internal/corpustest"
)

type grammarEntry struct {
	Name string `json:"name"`
}

func main() {
	manifestPath := "grammars.json"
	if p := os.Getenv("TREESITTER_MANIFEST"); p != "" {
		manifestPath = p
	}
	grammarsDir := "build/grammars"
	if dir := os.Getenv("TREESITTER_GRAMMAR_DIR"); dir != "" {
		grammarsDir = dir
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading manifest %s: %v\n", manifestPath, err)
		os.Exit(1)
	}
	var grammars []grammarEntry
	if err := json.Unmarshal(data, &grammars); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing manifest: %v\n", err)
		os.Exit(1)
	}

	totalSeeds := 0
	for _, g := range grammars {
		repoName := "tree-sitter-" + g.Name
		corpusDir := filepath.Join(grammarsDir, repoName, "test", "corpus")
		if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "skipping %s: corpus not found at %s\n", g.Name, corpusDir)
			continue
		}

		cases, err := corpustest.ParseCorpusDir(corpusDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing %s corpus: %v\n", g.Name, err)
			continue
		}

		seedDir := filepath.Join("testdata", "fuzz", "corpus", g.Name)
		if err := os.MkdirAll(seedDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating %s: %v\n", seedDir, err)
			continue
		}

		count := 0
		for _, tc := range cases {
			if len(tc.Input) == 0 {
				continue
			}

			// Use content hash as filename for deduplication.
			hash := sha256.Sum256(tc.Input)
			// Sanitize test name for use in filename.
			safeName := sanitizeName(tc.Name)
			filename := fmt.Sprintf("%s_%x.seed", safeName, hash[:8])
			seedPath := filepath.Join(seedDir, filename)

			if err := os.WriteFile(seedPath, tc.Input, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing %s: %v\n", seedPath, err)
				continue
			}
			count++
		}
		fmt.Printf("%s: %d seeds extracted to %s\n", g.Name, count, seedDir)
		totalSeeds += count
	}
	fmt.Printf("\ntotal: %d seeds extracted\n", totalSeeds)
}

func sanitizeName(name string) string {
	// Replace spaces and special chars with underscores, keep it short.
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, name)
	// Collapse multiple underscores.
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	name = strings.Trim(name, "_")
	if len(name) > 40 {
		name = name[:40]
	}
	return name
}
