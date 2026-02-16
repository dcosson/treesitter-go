// Package corpustest parses and runs tree-sitter grammar corpus test files.
//
// Every tree-sitter grammar has a test/corpus/ directory with .txt files
// in a specific format: header delimiters (3+ equals signs), test name
// with optional attributes, source code, divider (longest line of 3+
// hyphens), and expected S-expression output.
package corpustest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// TestCase represents a single corpus test.
type TestCase struct {
	Name       string
	Input      []byte
	Expected   string // Normalized S-expression
	HasFields  bool   // Whether the expected output includes field annotations
	Attributes TestAttributes
}

// TestAttributes holds optional markers on a corpus test.
type TestAttributes struct {
	Skip      bool
	Error     bool     // Expect ERROR/MISSING nodes in the parse tree
	FailFast  bool     // Stop on first failure
	CST       bool     // Compare full concrete syntax tree (preserve formatting)
	Platform  bool     // true if this test applies to the current platform
	Languages []string // Language name(s) for this test (empty string = default)
}

var (
	// headerRe matches a test header block:
	//   ===+ (3 or more equals)
	//   test name (one or more lines)
	//   optional attributes (:skip, :error, etc.)
	//   ===+ (3 or more equals)
	// The closing delimiter line must be followed by a newline.
	headerRe = regexp.MustCompile(`(?m)^(={3,})([^=\r\n][^\r\n]*)?\r?\n((?:(?:[^=\r\n]|\s+:)[^\r\n]*\r?\n)+)={3,}([^=\r\n][^\r\n]*)?\r?\n`)

	// dividerRe matches a divider line: 3+ hyphens at the start of a line.
	dividerRe = regexp.MustCompile(`(?m)^(-{3,})([^-\r\n][^\r\n]*)?\r?\n`)

	// commentRe matches S-expression comments (lines starting with optional whitespace then ;).
	commentRe = regexp.MustCompile(`(?m)^\s*;.*$`)

	// whitespaceRe matches one or more whitespace characters.
	whitespaceRe = regexp.MustCompile(`\s+`)

	// fieldRe detects field annotations like " name: (" in S-expressions.
	fieldRe = regexp.MustCompile(` \w+: \(`)

	// pointRe matches point annotations like [0, 5] - [1, 0] in S-expressions,
	// including the optional " - " separator between point pairs.
	pointRe = regexp.MustCompile(`\s*\[\s*\d+\s*,\s*\d+\s*\](?:\s*-\s*\[\s*\d+\s*,\s*\d+\s*\])?\s*`)
)

// ParseCorpusFile parses a corpus .txt file into test cases.
func ParseCorpusFile(data []byte) ([]TestCase, error) {
	return parseContent(data)
}

// ParseCorpusDir parses corpus test files in a directory into test cases.
// It accepts .txt files and extensionless files (some grammars like Perl
// use extensionless corpus files in the standard format).
func ParseCorpusDir(dir string) ([]TestCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading corpus directory %s: %w", dir, err)
	}

	var allCases []TestCase
	for _, entry := range entries {
		if entry.IsDir() {
			// Recurse into subdirectories (tree-sitter supports nested corpus dirs).
			subCases, err := ParseCorpusDir(filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, err
			}
			allCases = append(allCases, subCases...)
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".txt" && ext != "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		cases, err := ParseCorpusFile(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		allCases = append(allCases, cases...)
	}
	return allCases, nil
}

// parseContent parses the content of a single corpus file.
func parseContent(data []byte) ([]TestCase, error) {
	// Find all header matches and determine the first suffix (if any).
	// Headers with a different suffix than the first one are ignored.
	allHeaders := headerRe.FindAllSubmatchIndex(data, -1)
	if len(allHeaders) == 0 {
		return nil, nil
	}

	// Extract the suffix from the first header's opening delimiter.
	firstSuffix := extractSubmatch(data, allHeaders[0], 2)

	// Filter headers to only those matching the first suffix.
	type headerMatch struct {
		fullStart, fullEnd int
		bodyStart          int // byte offset where the content after the header starts
		name               string
		attributesStr      string
		attrs              TestAttributes
	}

	var headers []headerMatch
	for _, loc := range allHeaders {
		// loc indices: [0:1] = full match, [2:3] = opening delimiter,
		// [4:5] = suffix1, [6:7] = test name+markers, [8:9] = suffix2
		suffix1 := extractSubmatch(data, loc, 2)
		suffix2 := extractSubmatch(data, loc, 4)
		if suffix1 != firstSuffix || suffix2 != firstSuffix {
			continue
		}

		nameAndMarkers := string(data[loc[6]:loc[7]])
		name, attrsStr, attrs := parseNameAndAttributes(nameAndMarkers)

		headers = append(headers, headerMatch{
			fullStart:     loc[0],
			fullEnd:       loc[1],
			bodyStart:     loc[1],
			name:          name,
			attributesStr: attrsStr,
			attrs:         attrs,
		})
	}

	// For each consecutive pair of headers, find the divider between them
	// and extract the input/output.
	var cases []TestCase
	for i, hdr := range headers {
		var regionEnd int
		if i+1 < len(headers) {
			regionEnd = headers[i+1].fullStart
		} else {
			regionEnd = len(data)
		}

		region := data[hdr.bodyStart:regionEnd]

		// Find all dividers in this region, filter by matching suffix,
		// and pick the longest one (the "longest divider" rule).
		dividerLocs := dividerRe.FindAllSubmatchIndex(region, -1)
		bestLen := 0
		bestStart := -1
		bestEnd := -1
		for _, dloc := range dividerLocs {
			divSuffix := ""
			if dloc[4] >= 0 {
				divSuffix = string(region[dloc[4]:dloc[5]])
			}
			if divSuffix != firstSuffix {
				continue
			}
			hyphenLen := dloc[3] - dloc[2]
			if hyphenLen > bestLen {
				bestLen = hyphenLen
				bestStart = dloc[0]
				bestEnd = dloc[1]
			}
		}

		if bestStart < 0 {
			// No divider found — skip this test (malformed).
			continue
		}

		// Input is everything from header end to divider start, minus trailing newline.
		input := bytes.TrimRight(region[:bestStart], "\r\n")
		// Also strip a single leading newline (the blank line after the header).
		if len(input) > 0 && input[0] == '\n' {
			input = input[1:]
		} else if len(input) > 1 && input[0] == '\r' && input[1] == '\n' {
			input = input[2:]
		}

		// Output is everything from divider end to the next header (or EOF).
		output := region[bestEnd:]
		// Trim trailing whitespace that precedes the next header.
		output = bytes.TrimRight(output, "\r\n")

		var expected string
		var hasFields bool
		if hdr.attrs.CST {
			expected = strings.TrimSpace(string(output))
		} else {
			expected, hasFields = normalizeSExpression(string(output))
		}

		// Copy input to avoid aliasing the original data slice.
		inputCopy := make([]byte, len(input))
		copy(inputCopy, input)

		cases = append(cases, TestCase{
			Name:       hdr.name,
			Input:      inputCopy,
			Expected:   expected,
			HasFields:  hasFields,
			Attributes: hdr.attrs,
		})
	}

	return cases, nil
}

// extractSubmatch extracts submatch group n from a loc array.
// Returns empty string if the group didn't match.
func extractSubmatch(data []byte, loc []int, group int) string {
	start := loc[group*2]
	end := loc[group*2+1]
	if start < 0 {
		return ""
	}
	return string(data[start:end])
}

// parseNameAndAttributes splits the test name and markers block into
// the test name, attributes string, and parsed TestAttributes.
func parseNameAndAttributes(block string) (name, attrsStr string, attrs TestAttributes) {
	attrs.Platform = true // default: applies to all platforms
	var nameLines []string
	var attrLines []string
	seenMarker := false

	for _, line := range strings.Split(block, "\n") {
		if line == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		marker := trimmed
		if idx := strings.IndexByte(marker, '('); idx >= 0 {
			marker = marker[:idx]
		}

		switch marker {
		case ":skip":
			seenMarker = true
			attrs.Skip = true
		case ":error":
			seenMarker = true
			attrs.Error = true
		case ":fail-fast":
			seenMarker = true
			attrs.FailFast = true
		case ":cst":
			seenMarker = true
			attrs.CST = true
		case ":platform":
			seenMarker = true
			if p := extractParenArg(trimmed); p != "" {
				attrs.Platform = (strings.TrimSpace(p) == runtime.GOOS)
			}
		case ":language":
			seenMarker = true
			if lang := extractParenArg(trimmed); lang != "" {
				attrs.Languages = append(attrs.Languages, strings.TrimSpace(lang))
			}
		default:
			if !seenMarker {
				nameLines = append(nameLines, line)
			}
		}
		if seenMarker {
			attrLines = append(attrLines, line)
		}
	}

	// prefer skip over error — both shouldn't be set
	if attrs.Skip {
		attrs.Error = false
	}

	// default language if none specified
	if len(attrs.Languages) == 0 {
		attrs.Languages = []string{""}
	}

	name = strings.TrimSpace(strings.Join(nameLines, "\n"))
	attrsStr = strings.TrimSpace(strings.Join(attrLines, "\n"))
	return
}

// extractParenArg extracts the argument from ":attr(arg)" syntax.
func extractParenArg(s string) string {
	start := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if start < 0 || end <= start {
		return ""
	}
	return s[start+1 : end]
}

// NormalizeSExpression normalizes an S-expression string for comparison:
// - Strips ; comments
// - Collapses whitespace to single spaces
// - Removes space before )
// - Strips point annotations like [0, 5]
// Returns the normalized string and whether it contains field annotations.
func NormalizeSExpression(s string) (string, bool) {
	return normalizeSExpression(s)
}

func normalizeSExpression(s string) (string, bool) {
	// Strip comments.
	s = commentRe.ReplaceAllString(s, "")

	// Strip point annotations.
	s = pointRe.ReplaceAllString(s, " ")

	// Normalize whitespace.
	s = strings.TrimSpace(s)
	s = whitespaceRe.ReplaceAllString(s, " ")

	// Remove space before closing paren.
	s = strings.ReplaceAll(s, " )", ")")

	// Detect field annotations.
	hasFields := fieldRe.MatchString(s)

	return s, hasFields
}

// StripFields removes field annotations from an S-expression.
// " name: (" becomes " (" — used when the expected output has no fields.
func StripFields(s string) string {
	return fieldRe.ReplaceAllString(s, " (")
}
