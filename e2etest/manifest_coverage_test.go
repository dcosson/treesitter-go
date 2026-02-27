package e2etest_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// grammarEntry matches the structure of grammars.json.
type grammarEntry struct {
	Name string `json:"name"`
}

// loadManifestNames reads grammars.json and returns the set of language names.
func loadManifestNames(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("../grammars.json")
	if err != nil {
		t.Fatalf("failed to read grammars.json: %v", err)
	}
	var entries []grammarEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to parse grammars.json: %v", err)
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name] = true
	}
	return names
}

// TestManifestCorpusCoverage verifies that every language in grammars.json
// has a corresponding entry in corpusLanguages (i.e. a TestCorpus* test).
func TestManifestCorpusCoverage(t *testing.T) {
	manifest := loadManifestNames(t)

	// corpusLanguages maps manifest name -> the repo name used in
	// runCorpusForLanguage. Every language in grammars.json must appear here.
	corpusLanguages := map[string]string{
		"json":       "tree-sitter-json",
		"go":         "tree-sitter-go",
		"javascript": "tree-sitter-javascript",
		"python":     "tree-sitter-python",
		"bash":       "tree-sitter-bash",
		"rust":       "tree-sitter-rust",
		"c":          "tree-sitter-c",
		"cpp":        "tree-sitter-cpp",
		"typescript":  "tree-sitter-typescript",
		"ruby":       "tree-sitter-ruby",
		"java":       "tree-sitter-java",
		"html":       "tree-sitter-html",
		"css":        "tree-sitter-css",
		"lua":        "tree-sitter-lua",
		"perl":       "tree-sitter-perl",
	}

	for name := range manifest {
		if _, ok := corpusLanguages[name]; !ok {
			t.Errorf("language %q is in grammars.json but has no corpus test — add it to corpus_languages_test.go and this map", name)
		}
	}
	for name := range corpusLanguages {
		if !manifest[name] {
			t.Errorf("language %q has a corpus test but is not in grammars.json — remove it or add it to the manifest", name)
		}
	}
}

// TestManifestBenchCoverage verifies that every language in grammars.json
// has a corresponding entry in benchLanguages().
func TestManifestBenchCoverage(t *testing.T) {
	manifest := loadManifestNames(t)

	benchNames := make(map[string]bool)
	for _, bl := range benchLanguages() {
		benchNames[bl.name] = true
	}

	for name := range manifest {
		if !benchNames[name] {
			t.Errorf("language %q is in grammars.json but has no benchmark entry — add it to benchLanguages() in benchmark_test.go", name)
		}
	}
	for name := range benchNames {
		if !manifest[name] {
			t.Errorf("language %q has a benchmark entry but is not in grammars.json — remove it or add it to the manifest", name)
		}
	}
}

// TestManifestRepoNameConsistency verifies that grammars.json repo fields
// follow the expected tree-sitter-{name} convention (with known exceptions).
func TestManifestRepoNameConsistency(t *testing.T) {
	data, err := os.ReadFile("../grammars.json")
	if err != nil {
		t.Fatalf("failed to read grammars.json: %v", err)
	}
	var entries []struct {
		Name string `json:"name"`
		Repo string `json:"repo"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to parse grammars.json: %v", err)
	}

	for _, e := range entries {
		// Extract the repo name (last path segment)
		parts := strings.Split(e.Repo, "/")
		repoName := parts[len(parts)-1]
		expected := "tree-sitter-" + e.Name
		if repoName != expected {
			t.Logf("note: language %q has non-standard repo name %q (expected %q)", e.Name, repoName, expected)
		}
	}
}
