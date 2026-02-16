package generate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// resolveSymbolValueStandalone is used by lex extraction when we don't have
// a Grammar pointer in scope. It tries numeric parsing first.
func resolveSymbolValueStandalone(name string, enumMap map[string]int) uint16 {
	if v, err := strconv.Atoi(name); err == nil {
		return uint16(v)
	}
	if enumMap != nil {
		if v, ok := enumMap[name]; ok {
			return uint16(v)
		}
	}
	return 0
}

// extractLexFunction parses a ts_lex or ts_lex_keywords function from parser.c
// and populates the Grammar's LexStates or KeywordLexStates.
func extractLexFunction(g *Grammar, src string, isKeyword bool) error {
	var funcName string
	if isKeyword {
		funcName = "ts_lex_keywords"
	} else {
		funcName = "ts_lex"
	}

	// Find the function body.
	body := extractFunctionBody(src, funcName)
	if body == "" {
		if isKeyword {
			return nil // keyword lex is optional
		}
		return fmt.Errorf("%s function not found", funcName)
	}

	states, err := parseLexDFA(body, g.symbolEnum)
	if err != nil {
		return fmt.Errorf("parsing %s DFA: %w", funcName, err)
	}

	if isKeyword {
		g.KeywordLexStates = states
	} else {
		g.LexStates = states
	}
	return nil
}

// extractFunctionBody finds the body of a named function in C source.
// Returns the content between the outermost braces of the function.
func extractFunctionBody(src, funcName string) string {
	// Find the function signature.
	idx := strings.Index(src, funcName+"(")
	if idx < 0 {
		return ""
	}

	// Find the opening brace of the function body.
	rest := src[idx:]
	braceIdx := strings.Index(rest, "{")
	if braceIdx < 0 {
		return ""
	}

	start := idx + braceIdx
	end := findMatchingBrace(src, start)
	if end < 0 {
		return ""
	}
	return src[start+1 : end]
}

// parseLexDFA parses the DFA states from a lex function body.
func parseLexDFA(body string, symbolEnum map[string]int) ([]LexState, error) {
	// Split into case blocks.
	caseBlocks := splitCaseBlocks(body)

	var states []LexState
	for _, cb := range caseBlocks {
		state, err := parseCaseBlock(cb.id, cb.body, symbolEnum)
		if err != nil {
			return nil, fmt.Errorf("state %d: %w", cb.id, err)
		}
		states = append(states, state)
	}

	return states, nil
}

type caseBlock struct {
	id   int
	body string
}

// splitCaseBlocks splits the switch body into individual case blocks.
func splitCaseBlocks(body string) []caseBlock {
	// Find the switch statement.
	switchIdx := strings.Index(body, "switch (state)")
	if switchIdx < 0 {
		return nil
	}

	// Find the switch body.
	switchBody := body[switchIdx:]
	braceIdx := strings.Index(switchBody, "{")
	if braceIdx < 0 {
		return nil
	}
	switchBody = switchBody[braceIdx+1:]

	// Find all case N: patterns and split.
	caseRe := regexp.MustCompile(`(?m)^\s*case\s+(\d+):`)
	matches := caseRe.FindAllStringSubmatchIndex(switchBody, -1)

	var blocks []caseBlock
	for i, m := range matches {
		id, _ := strconv.Atoi(switchBody[m[2]:m[3]])
		var caseBody string
		if i+1 < len(matches) {
			caseBody = switchBody[m[1]:matches[i+1][0]]
		} else {
			// Last case: find the default or closing brace.
			endIdx := strings.Index(switchBody[m[1]:], "default:")
			if endIdx < 0 {
				// Find closing brace of switch.
				endIdx = len(switchBody) - m[1]
			}
			caseBody = switchBody[m[1] : m[1]+endIdx]
		}
		blocks = append(blocks, caseBlock{id: id, body: caseBody})
	}

	return blocks
}

// parseCaseBlock parses a single DFA state's case block.
func parseCaseBlock(id int, body string, symbolEnum map[string]int) (LexState, error) {
	state := LexState{ID: id}

	lines := strings.Split(body, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines and END_STATE.
		if line == "" || strings.Contains(line, "END_STATE()") {
			continue
		}

		// ACCEPT_TOKEN(sym)
		if m := matchAcceptToken(line, symbolEnum); m != nil {
			state.AcceptToken = m.symbol
			continue
		}

		// ADVANCE_MAP(...)
		if strings.Contains(line, "ADVANCE_MAP(") {
			// Collect all lines until closing ");"
			mapBlock := line
			for !strings.Contains(mapBlock, ");") && i+1 < len(lines) {
				i++
				mapBlock += "\n" + strings.TrimSpace(lines[i])
			}
			transitions := parseAdvanceMap(mapBlock)
			state.Transitions = append(state.Transitions, transitions...)
			continue
		}

		// set_contains() calls — check BEFORE eof to avoid misinterpreting
		// "!eof && set_contains(...)" as a standalone "if (eof)" check.
		if strings.Contains(line, "set_contains(") {
			if t := parseSetContains(line); t != nil {
				state.Transitions = append(state.Transitions, *t)
			}
			continue
		}

		// if (eof) ADVANCE(N) — standalone eof check only (not compound !eof).
		if strings.Contains(line, "eof") && !strings.Contains(line, "!eof") &&
			strings.Contains(line, "ADVANCE(") {
			target := extractInt(line, `ADVANCE\((\d+)\)`)
			state.HasEOFCheck = true
			state.EOFTarget = target
			continue
		}

		// if (eof) ACCEPT_TOKEN(sym) — standalone eof check only.
		if strings.Contains(line, "eof") && !strings.Contains(line, "!eof") &&
			strings.Contains(line, "ACCEPT_TOKEN(") {
			sym := extractInt(line, `ACCEPT_TOKEN\((\w+)\)`)
			state.HasEOFCheck = true
			state.EOFAccept = true
			state.EOFAcceptToken = uint16(sym)
			continue
		}

		// Standard if-chain transitions.
		if strings.HasPrefix(line, "if (") || strings.HasPrefix(line, "if(") {
			transitions := parseIfTransitions(line, lines, &i)
			state.Transitions = append(state.Transitions, transitions...)
			continue
		}
	}

	// Check for default transition (catch-all).
	detectDefault(&state)

	return state, nil
}

type acceptMatch struct {
	symbol uint16
}

// matchAcceptToken checks if a line contains ACCEPT_TOKEN and returns the symbol.
func matchAcceptToken(line string, symbolEnum map[string]int) *acceptMatch {
	re := regexp.MustCompile(`ACCEPT_TOKEN\((\w+)\)`)
	m := re.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	sym := resolveSymbolValueStandalone(m[1], symbolEnum)
	return &acceptMatch{symbol: sym}
}

// parseAdvanceMap parses an ADVANCE_MAP(...) block into transitions.
func parseAdvanceMap(block string) []LexTransition {
	// Extract content between parens.
	start := strings.Index(block, "(")
	end := strings.LastIndex(block, ")")
	if start < 0 || end < 0 {
		return nil
	}
	content := block[start+1 : end]

	// Parse character, target pairs.
	// Format: 'c', N, 'c2', N2, ... or bare_int, N, ...
	// Bare integers (e.g. 0) represent the ASCII value directly (null byte),
	// while quoted chars (e.g. '0') represent the character literal.
	var transitions []LexTransition
	pairs := splitCSV(content)
	for i := 0; i+1 < len(pairs); i += 2 {
		charStr := strings.TrimSpace(pairs[i])
		targetStr := strings.TrimSpace(pairs[i+1])

		var ch rune
		if len(charStr) > 0 && charStr[0] != '\'' {
			// Bare integer literal: treat as ASCII code point.
			v, _ := strconv.ParseInt(charStr, 0, 32)
			ch = rune(v)
		} else {
			ch = parseCChar(charStr)
		}
		target, _ := strconv.Atoi(targetStr)

		transitions = append(transitions, LexTransition{
			Char:   ch,
			Target: target,
		})
	}

	return transitions
}

// parseIfTransitions parses an if-statement into one or more transitions.
// A single if with ||–chained conditions (ranges and chars) produces multiple
// transitions that all share the same target/skip.
func parseIfTransitions(line string, lines []string, idx *int) []LexTransition {
	// Combine multi-line if conditions.
	fullLine := line
	for !strings.Contains(fullLine, "ADVANCE(") && !strings.Contains(fullLine, "SKIP(") &&
		*idx+1 < len(lines) {
		*idx++
		nextLine := strings.TrimSpace(lines[*idx])
		if nextLine == "" || strings.Contains(nextLine, "END_STATE()") {
			break
		}
		fullLine += " " + nextLine
	}

	// Determine target and skip.
	var target int
	var skip bool
	if m := regexp.MustCompile(`ADVANCE\((\d+)\)`).FindStringSubmatch(fullLine); m != nil {
		target, _ = strconv.Atoi(m[1])
	} else if m := regexp.MustCompile(`SKIP\((\d+)\)`).FindStringSubmatch(fullLine); m != nil {
		target, _ = strconv.Atoi(m[1])
		skip = true
	} else {
		return nil
	}

	base := LexTransition{Target: target, Skip: skip}

	// Parse compound conditions by splitting on top-level && (respecting parens),
	// then classifying each sub-condition. This handles all patterns:
	//   Pattern 1: lookahead != X, possibly &&-chained
	//   Pattern 2: lookahead > X && lookahead != Y
	//   Pattern 3: (lookahead < X || Y < lookahead) exclusion ranges
	//   Pattern 4: lookahead > X && exclusion ranges
	//   Pattern 5: != conditions mixed with exclusion ranges
	if strings.Contains(fullLine, "lookahead !=") || strings.Contains(fullLine, "lookahead >") {
		condRe := regexp.MustCompile(`if\s*\(\s*(.*?)\)\s*(?:ADVANCE|SKIP)\(`)
		condMatch := condRe.FindStringSubmatch(fullLine)
		if condMatch != nil {
			condStr := condMatch[1]
			subconds := splitTopLevelAnd(condStr)

			var exclusions []rune
			var lowBound rune
			var excludeRanges []RuneRange

			neqCharRe := regexp.MustCompile(`^\s*lookahead\s*!=\s*'([^']*)'`)
			neqZeroRe := regexp.MustCompile(`^\s*lookahead\s*!=\s*0\s*$`)
			gtCharRe := regexp.MustCompile(`^\s*lookahead\s*>\s*'([^']*)'`)
			exclRangeRe := regexp.MustCompile(`^\s*\(?\s*lookahead\s*<\s*'([^']*)'\s*\|\|\s*'([^']*)'\s*<\s*lookahead\s*\)?\s*$`)

			for _, sc := range subconds {
				sc = strings.TrimSpace(sc)
				if m := neqCharRe.FindStringSubmatch(sc); m != nil {
					exclusions = append(exclusions, parseCChar("'"+m[1]+"'"))
				} else if neqZeroRe.MatchString(sc) {
					exclusions = append(exclusions, 0)
				} else if m := gtCharRe.FindStringSubmatch(sc); m != nil {
					lowBound = parseCChar("'" + m[1] + "'")
				} else if m := exclRangeRe.FindStringSubmatch(sc); m != nil {
					excludeRanges = append(excludeRanges, RuneRange{
						Low:  parseCChar("'" + m[1] + "'"),
						High: parseCChar("'" + m[2] + "'"),
					})
				}
			}

			exclusions = dedupeRunes(exclusions)

			if len(exclusions) > 0 || lowBound != 0 || len(excludeRanges) > 0 {
				t := base
				t.IsNegated = true
				t.LowBound = lowBound
				t.ExcludeRanges = excludeRanges
				if len(exclusions) == 1 {
					t.Char = exclusions[0]
				} else if len(exclusions) > 1 {
					t.CharExclusions = exclusions
				}
				return []LexTransition{t}
			}
		}
	}

	// For || chains (or single conditions), extract all conditions as transitions.
	rangeRe := regexp.MustCompile(`'([^']*)'\s*<=\s*lookahead\s*&&\s*lookahead\s*<=\s*'([^']*)'`)
	hexRangeRe := regexp.MustCompile(`(0x[0-9a-fA-F]+)\s*<=\s*lookahead\s*&&\s*lookahead\s*<=\s*(?:'([^']*)'|(0x[0-9a-fA-F]+))`)
	charRe := regexp.MustCompile(`lookahead\s*==\s*'([^']*)'`)

	var result []LexTransition

	// Find all char-range matches: ('a' <= lookahead && lookahead <= 'z')
	for _, m := range rangeRe.FindAllStringSubmatch(fullLine, -1) {
		t := base
		t.IsRange = true
		t.Low = parseCChar("'" + m[1] + "'")
		t.High = parseCChar("'" + m[2] + "'")
		result = append(result, t)
	}

	// Find all hex-range matches: (0xNN <= lookahead && lookahead <= 0xNN)
	for _, m := range hexRangeRe.FindAllStringSubmatch(fullLine, -1) {
		t := base
		t.IsRange = true
		low, _ := strconv.ParseInt(m[1], 0, 32)
		t.Low = rune(low)
		if m[2] != "" {
			t.High = parseCChar("'" + m[2] + "'")
		} else {
			high, _ := strconv.ParseInt(m[3], 0, 32)
			t.High = rune(high)
		}
		result = append(result, t)
	}

	// Find all single-char matches: lookahead == 'c'
	for _, m := range charRe.FindAllStringSubmatch(fullLine, -1) {
		t := base
		t.Char = parseCChar("'" + m[1] + "'")
		result = append(result, t)
	}

	if len(result) > 0 {
		return result
	}

	// Fallback: return the base transition (might have Char=0 which is valid for eof checks).
	return []LexTransition{base}
}

// detectDefault detects if the last transition is a catch-all default.
// A simple "lookahead != 0" becomes a plain default (advance on anything non-EOF).
// A compound negation like "lookahead != '<' && lookahead != 0" stays as a
// transition with CharExclusions so codegen can emit the proper compound check.
func detectDefault(state *LexState) {
	if len(state.Transitions) == 0 {
		return
	}
	last := &state.Transitions[len(state.Transitions)-1]
	if last.IsNegated && last.Char == 0 && len(last.CharExclusions) == 0 {
		state.HasDefault = true
		state.DefaultTarget = last.Target
		state.DefaultSkip = last.Skip
		// Remove it from transitions.
		state.Transitions = state.Transitions[:len(state.Transitions)-1]
	}
}

// parseCChar parses a C character literal like 'x', '\'', '\\', '\n'.
func parseCChar(s string) rune {
	// Strip only the outermost single quotes (not all of them like strings.Trim).
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		s = s[1 : len(s)-1]
	}
	if s == "" {
		return 0
	}
	switch s {
	case `\n`:
		return '\n'
	case `\t`:
		return '\t'
	case `\r`:
		return '\r'
	case `\0`:
		return 0
	case `\\`:
		return '\\'
	case `\'`:
		return '\''
	case `\"`:
		return '"'
	case `\a`:
		return '\a'
	case `\b`:
		return '\b'
	case `\f`:
		return '\f'
	case `\v`:
		return '\v'
	}
	// Hex escape: \xNN
	if strings.HasPrefix(s, `\x`) {
		v, err := strconv.ParseInt(s[2:], 16, 32)
		if err == nil {
			return rune(v)
		}
	}
	// Unicode escape: \uNNNN
	if strings.HasPrefix(s, `\u`) {
		v, err := strconv.ParseInt(s[2:], 16, 32)
		if err == nil {
			return rune(v)
		}
	}
	// Single character.
	runes := []rune(s)
	if len(runes) == 1 {
		return runes[0]
	}
	return 0
}

// parseSetContains parses a set_contains() call into a LexTransition.
// Handles both simple and compound forms:
//
//	if (set_contains(name, count, lookahead)) ADVANCE(N);
//	if ((!eof && set_contains(name, count, lookahead))) ADVANCE(N);
func parseSetContains(line string) *LexTransition {
	// Use \)+ to handle variable numbers of closing parens (compound !eof forms have extra).
	re := regexp.MustCompile(`set_contains\((\w+),\s*\d+,\s*lookahead\)\)+\s*(?:ADVANCE|SKIP)\((\d+)\)`)
	m := re.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	setName := m[1]
	target, _ := strconv.Atoi(m[2])
	skip := strings.Contains(line, "SKIP(")
	hasEOFGuard := strings.Contains(line, "!eof")

	return &LexTransition{
		CharSetName: setName,
		Target:      target,
		Skip:        skip,
		EOFGuard:    hasEOFGuard,
	}
}

// splitCSV splits a comma-separated string, respecting nested parens and
// C character literals. Properly handles escaped chars like '\'' and '\\'.
func splitCSV(s string) []string {
	var parts []string
	var current strings.Builder
	depth := 0

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			// C char literal: consume everything through closing quote.
			// Handles: 'x', '\'', '\\', '\n', '\xNN'
			current.WriteByte(c) // opening '
			i++
			for i < len(s) {
				cc := s[i]
				current.WriteByte(cc)
				if cc == '\\' {
					// Escape: consume next char unconditionally.
					i++
					if i < len(s) {
						current.WriteByte(s[i])
					}
				} else if cc == '\'' {
					// Closing quote.
					break
				}
				i++
			}
		} else if c == '(' {
			depth++
			current.WriteByte(c)
		} else if c == ')' {
			depth--
			current.WriteByte(c)
		} else if depth == 0 && c == ',' {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// splitTopLevelAnd splits a C condition string on top-level "&&" operators,
// respecting parenthesized sub-expressions.
func splitTopLevelAnd(s string) []string {
	var parts []string
	var current strings.Builder
	depth := 0

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			current.WriteByte(c)
			i++
			for i < len(s) {
				cc := s[i]
				current.WriteByte(cc)
				if cc == '\\' {
					i++
					if i < len(s) {
						current.WriteByte(s[i])
					}
				} else if cc == '\'' {
					break
				}
				i++
			}
		} else if c == '(' {
			depth++
			current.WriteByte(c)
		} else if c == ')' {
			depth--
			current.WriteByte(c)
		} else if depth == 0 && c == '&' && i+1 < len(s) && s[i+1] == '&' {
			parts = append(parts, current.String())
			current.Reset()
			i++ // skip second '&'
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// dedupeRunes removes duplicate runes from a slice while preserving order.
func dedupeRunes(rs []rune) []rune {
	if len(rs) <= 1 {
		return rs
	}
	seen := make(map[rune]bool, len(rs))
	var result []rune
	for _, r := range rs {
		if !seen[r] {
			seen[r] = true
			result = append(result, r)
		}
	}
	return result
}
