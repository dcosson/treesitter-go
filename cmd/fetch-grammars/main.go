// fetch-grammars clones or updates tree-sitter grammar repos for corpus testing.
// After cloning, it ensures parser.c exists for each grammar by running
// "tree-sitter generate src/grammar.json" when parser.c is missing.
//
// Usage:
//
//	go run ./cmd/fetch-grammars [-config testdata/grammars.json] [-output testdata/grammars/]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type grammar struct {
	Name    string `json:"name"`
	Repo    string `json:"repo"`
	Version string `json:"version"`
}

func main() {
	configPath := flag.String("config", "testdata/grammars.json", "path to grammars config file")
	outputDir := flag.String("output", "testdata/grammars", "output directory for cloned grammars")
	flag.Parse()

	data, err := os.ReadFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading config: %v\n", err)
		os.Exit(1)
	}

	var grammars []grammar
	if err := json.Unmarshal(data, &grammars); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing config: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	tsCLI, err := exec.LookPath("tree-sitter")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: tree-sitter CLI not found, will skip parser generation for grammars missing parser.c\n")
	}

	for _, g := range grammars {
		repoURL := fmt.Sprintf("https://github.com/%s.git", g.Repo)
		dirName := fmt.Sprintf("tree-sitter-%s", g.Name)
		repoDir := filepath.Join(*outputDir, dirName)

		if _, err := os.Stat(repoDir); os.IsNotExist(err) {
			fmt.Printf("cloning %s @ %s ...\n", g.Repo, g.Version)
			cmd := exec.Command("git", "clone", "--depth=1", "--branch", g.Version, repoURL, repoDir)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "error cloning %s: %v\n", g.Name, err)
				os.Exit(1)
			}
		} else {
			// Fetch the desired version and checkout.
			fmt.Printf("updating %s to %s ...\n", g.Name, g.Version)
			if err := fetchVersion(repoDir, g.Version); err != nil {
				fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", g.Name, err)
				os.Exit(1)
			}
		}

		// Ensure parser.c exists for each grammar.json in the repo.
		// Some grammars (e.g. perl) don't commit parser.c; others (e.g.
		// typescript) have multiple sub-grammars in subdirectories.
		if tsCLI != "" {
			if err := generateMissingParsers(tsCLI, repoDir, g.Name); err != nil {
				fmt.Fprintf(os.Stderr, "error generating parser for %s: %v\n", g.Name, err)
				os.Exit(1)
			}
		}
	}

	fmt.Println("done.")
}

// fetchVersion fetches the given version (tag or branch) and checks it out.
// It tries as a tag first, then falls back to a branch.
func fetchVersion(repoDir, version string) error {
	// Try fetching as a tag first.
	fetchTag := exec.Command("git", "fetch", "--depth=1", "origin", "tag", version)
	fetchTag.Dir = repoDir
	if err := fetchTag.Run(); err == nil {
		checkout := exec.Command("git", "checkout", version)
		checkout.Dir = repoDir
		checkout.Stdout = os.Stdout
		checkout.Stderr = os.Stderr
		return checkout.Run()
	}

	// Fall back to fetching as a branch.
	fetchBranch := exec.Command("git", "fetch", "--depth=1", "origin", version)
	fetchBranch.Dir = repoDir
	fetchBranch.Stdout = os.Stdout
	fetchBranch.Stderr = os.Stderr
	if err := fetchBranch.Run(); err != nil {
		return fmt.Errorf("fetch tag and branch both failed for %q: %w", version, err)
	}

	checkout := exec.Command("git", "checkout", "FETCH_HEAD")
	checkout.Dir = repoDir
	checkout.Stdout = os.Stdout
	checkout.Stderr = os.Stderr
	return checkout.Run()
}

// generateMissingParsers walks the repo looking for src/grammar.json files
// that don't have a sibling parser.c, and runs "tree-sitter generate" to
// produce it. This handles both standard layouts (src/grammar.json) and
// multi-grammar repos like typescript (typescript/src/grammar.json, tsx/src/grammar.json).
func generateMissingParsers(tsCLI, repoDir, name string) error {
	return filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip .git directories.
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() || d.Name() != "grammar.json" {
			return nil
		}
		// Only consider grammar.json files inside a src/ directory.
		dir := filepath.Dir(path)
		if filepath.Base(dir) != "src" {
			return nil
		}
		parserC := filepath.Join(dir, "parser.c")
		if _, err := os.Stat(parserC); err == nil {
			return nil // parser.c already exists
		}

		// Determine a label for logging (e.g. "perl" or "typescript/tsx").
		rel, _ := filepath.Rel(repoDir, dir)
		subGrammar := strings.TrimSuffix(rel, "/src")
		if subGrammar == "src" {
			subGrammar = name
		} else {
			subGrammar = name + "/" + subGrammar
		}

		fmt.Printf("generating parser.c for %s ...\n", subGrammar)
		grammarRoot := filepath.Dir(dir) // parent of src/
		cmd := exec.Command(tsCLI, "generate", "src/grammar.json")
		cmd.Dir = grammarRoot
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})
}
