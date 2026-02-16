// fetch-grammars clones or updates tree-sitter grammar repos for corpus testing.
//
// Usage:
//
//	go run ./cmd/fetch-grammars [-config testdata/grammars.json] [-output testdata/grammars/]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
			fetch := exec.Command("git", "fetch", "--depth=1", "origin", "tag", g.Version)
			fetch.Dir = repoDir
			fetch.Stdout = os.Stdout
			fetch.Stderr = os.Stderr
			if err := fetch.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", g.Name, err)
				os.Exit(1)
			}

			checkout := exec.Command("git", "checkout", g.Version)
			checkout.Dir = repoDir
			checkout.Stdout = os.Stdout
			checkout.Stderr = os.Stderr
			if err := checkout.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "error checking out %s: %v\n", g.Name, err)
				os.Exit(1)
			}
		}
	}

	fmt.Println("done.")
}
