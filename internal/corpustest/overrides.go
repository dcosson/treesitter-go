package corpustest

import (
	"encoding/json"
	"fmt"
	"os"
)

// CorpusOverride describes an optional per-test override loaded from JSON.
type CorpusOverride struct {
	Skip     bool   `json:"skip"`
	Expected string `json:"expected"`
}

// CorpusOverrides is keyed by repo name, then test name.
type CorpusOverrides map[string]map[string]CorpusOverride

// ParseOverridesFile parses an overrides JSON file.
func ParseOverridesFile(path string) (CorpusOverrides, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var overrides CorpusOverrides
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("parse corpus overrides %s: %w", path, err)
	}
	return overrides, nil
}

// ApplyOverrides mutates and returns cases with repo-specific overrides applied.
func ApplyOverrides(cases []TestCase, repoName string, overrides CorpusOverrides) ([]TestCase, error) {
	repoOverrides, ok := overrides[repoName]
	if !ok || len(repoOverrides) == 0 {
		return cases, nil
	}

	for i := range cases {
		ov, ok := repoOverrides[cases[i].Name]
		if !ok {
			continue
		}
		if ov.Skip {
			cases[i].Attributes.Skip = true
		}
		if ov.Expected != "" {
			normalized, hasFields := normalizeSExpression(ov.Expected)
			cases[i].Expected = normalized
			cases[i].HasFields = hasFields
		}
	}

	return cases, nil
}
