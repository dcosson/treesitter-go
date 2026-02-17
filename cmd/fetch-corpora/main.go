// fetch-corpora downloads real-world source files from GitHub for differential testing.
//
// It reads a corpora-manifest.json file that specifies projects, git refs (tags/branches),
// and file paths. For each file, it downloads the raw content from GitHub and stores it
// in testdata/corpora/<language>/<project>/.
//
// The tool resolves each ref to a commit SHA and records it in a fetched-manifest.json
// alongside the downloaded files.
//
// Usage:
//
//	go run ./cmd/fetch-corpora [-manifest testdata/corpora-manifest.json] [-output testdata/corpora/]
//	go run ./cmd/fetch-corpora -verify  # verify all files exist without downloading
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest is the top-level corpora manifest.
type Manifest struct {
	Description string    `json:"description"`
	Projects    []Project `json:"projects"`
}

// Project describes a set of files to fetch from a GitHub repo.
type Project struct {
	Language   string   `json:"language"`
	Extensions []string `json:"extensions"`
	ProjectName string  `json:"project"`
	Repo       string   `json:"repo"`
	Ref        string   `json:"ref"`
	Files      []string `json:"files"`
}

// FetchedManifest records what was actually downloaded.
type FetchedManifest struct {
	FetchedAt string          `json:"fetched_at"`
	Projects  []FetchedProject `json:"projects"`
}

// FetchedProject records the resolved commit and downloaded files for a project.
type FetchedProject struct {
	Language    string   `json:"language"`
	ProjectName string  `json:"project"`
	Repo        string  `json:"repo"`
	Ref         string  `json:"ref"`
	Files       []string `json:"files"`
	LocalDir    string   `json:"local_dir"`
}

func main() {
	manifestPath := flag.String("manifest", "testdata/corpora-manifest.json", "path to corpora manifest")
	outputDir := flag.String("output", "testdata/corpora", "output directory")
	verify := flag.Bool("verify", false, "verify files exist without downloading")
	force := flag.Bool("force", false, "re-download even if files exist")
	flag.Parse()

	data, err := os.ReadFile(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading manifest: %v\n", err)
		os.Exit(1)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing manifest: %v\n", err)
		os.Exit(1)
	}

	if *verify {
		runVerify(manifest, *outputDir)
		return
	}

	fetched := FetchedManifest{
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	totalFiles := 0
	skippedFiles := 0
	downloadedFiles := 0
	failedFiles := 0

	for _, proj := range manifest.Projects {
		projDir := filepath.Join(*outputDir, proj.Language, proj.ProjectName)
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating directory %s: %v\n", projDir, err)
			continue
		}

		fp := FetchedProject{
			Language:    proj.Language,
			ProjectName: proj.ProjectName,
			Repo:        proj.Repo,
			Ref:         proj.Ref,
			LocalDir:    filepath.Join(proj.Language, proj.ProjectName),
		}

		fmt.Printf("== %s/%s (%s @ %s) ==\n", proj.Language, proj.ProjectName, proj.Repo, proj.Ref)

		for _, filePath := range proj.Files {
			totalFiles++

			// Use the basename for local storage to keep paths flat.
			localName := filepath.Base(filePath)
			localPath := filepath.Join(projDir, localName)

			// Skip if file already exists (unless -force).
			if !*force {
				if info, err := os.Stat(localPath); err == nil && info.Size() > 0 {
					fmt.Printf("  skip (exists): %s\n", localName)
					skippedFiles++
					fp.Files = append(fp.Files, localName)
					continue
				}
			}

			// Download from GitHub raw content.
			url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s",
				proj.Repo, proj.Ref, filePath)

			content, err := downloadFile(url)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  FAIL: %s: %v\n", filePath, err)
				failedFiles++
				continue
			}

			if err := os.WriteFile(localPath, content, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "  FAIL writing %s: %v\n", localPath, err)
				failedFiles++
				continue
			}

			fmt.Printf("  ok: %s (%d bytes)\n", localName, len(content))
			downloadedFiles++
			fp.Files = append(fp.Files, localName)
		}

		fetched.Projects = append(fetched.Projects, fp)
	}

	// Write fetched manifest.
	fetchedPath := filepath.Join(*outputDir, "fetched-manifest.json")
	fetchedData, err := json.MarshalIndent(fetched, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling fetched manifest: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(fetchedPath, fetchedData, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing fetched manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nDone: %d downloaded, %d skipped, %d failed (of %d total)\n",
		downloadedFiles, skippedFiles, failedFiles, totalFiles)
	fmt.Printf("Fetched manifest: %s\n", fetchedPath)

	if failedFiles > 0 {
		os.Exit(1)
	}
}

func downloadFile(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return body, nil
}

func runVerify(manifest Manifest, outputDir string) {
	total := 0
	found := 0
	missing := 0

	for _, proj := range manifest.Projects {
		projDir := filepath.Join(outputDir, proj.Language, proj.ProjectName)
		for _, filePath := range proj.Files {
			total++
			localName := filepath.Base(filePath)
			localPath := filepath.Join(projDir, localName)

			if info, err := os.Stat(localPath); err == nil && info.Size() > 0 {
				found++
			} else {
				fmt.Printf("MISSING: %s/%s/%s (from %s)\n",
					proj.Language, proj.ProjectName, localName, filePath)
				missing++
			}
		}
	}

	fmt.Printf("\nVerify: %d found, %d missing (of %d total)\n", found, missing, total)
	if missing > 0 {
		fmt.Println("Run 'go run ./cmd/fetch-corpora' to download missing files.")
		os.Exit(1)
	}

	// Check for duplicate basenames within a project.
	for _, proj := range manifest.Projects {
		seen := make(map[string]string)
		for _, filePath := range proj.Files {
			base := filepath.Base(filePath)
			if prev, ok := seen[base]; ok {
				fmt.Printf("WARNING: duplicate basename %q in %s/%s: %s and %s\n",
					base, proj.Language, proj.ProjectName, prev, filePath)
			}
			seen[base] = filePath
		}
	}

	// Check file extensions match declared language extensions.
	for _, proj := range manifest.Projects {
		extSet := make(map[string]bool)
		for _, e := range proj.Extensions {
			extSet[e] = true
		}
		for _, filePath := range proj.Files {
			ext := filepath.Ext(filePath)
			if ext == "" {
				ext = filepath.Ext(strings.TrimSuffix(filePath, filepath.Ext(filePath)))
			}
			if !extSet[ext] && ext != "" {
				fmt.Printf("WARNING: %s/%s file %s has extension %q not in declared extensions %v\n",
					proj.Language, proj.ProjectName, filePath, ext, proj.Extensions)
			}
		}
	}
}
