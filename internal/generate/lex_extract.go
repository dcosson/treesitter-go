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

		// if (eof) ADVANCE(N)
		if strings.Contains(line, "eof") && strings.Contains(line, "ADVANCE(") {
			target := extractInt(line, `ADVANCE\((\d+)\)`)
			state.HasEOFCheck = true
			state.EOFTarget = target
			continue
		}

		// if (eof) ACCEPT_TOKEN(sym)
		if strings.Contains(line, "eof") && strings.Contains(line, "ACCEPT_TOKEN(") {
			sym := extractInt(line, `ACCEPT_TOKEN\((\w+)\)`)
			state.HasEOFCheck = true
			state.EOFAccept = true
			state.EOFAcceptToken = uint16(sym)
			continue
		}

		// Standard if-chain transitions.
		if strings.HasPrefix(line, "if (") || strings.HasPrefix(line, "if(") {
			transition := parseIfTransition(line, lines, &i)
			if transition != nil {
				state.Transitions = append(state.Transitions, *transition)
			}
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
	// Format: 'c', N, 'c2', N2, ...
	var transitions []LexTransition
	pairs := splitCSV(content)
	for i := 0; i+1 < len(pairs); i += 2 {
		charStr := strings.TrimSpace(pairs[i])
		targetStr := strings.TrimSpace(pairs[i+1])

		ch := parseCChar(charStr)
		target, _ := strconv.Atoi(targetStr)

		transitions = append(transitions, LexTransition{
			Char:   ch,
			Target: target,
		})
	}

	return transitions
}

// parseIfTransition parses an if-statement transition.
func parseIfTransition(line string, lines []string, idx *int) *LexTransition {
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

	// Parse the condition.
	t := LexTransition{
		Target: target,
		Skip:   skip,
	}

	// Single char: if (lookahead == 'c')
	if m := regexp.MustCompile(`lookahead\s*==\s*'([^']*)'`).FindStringSubmatch(fullLine); m != nil {
		t.Char = parseCChar("'" + m[1] + "'")
		return &t
	}

	// Negated: if (lookahead != 0)
	if strings.Contains(fullLine, "lookahead != 0") {
		t.IsNegated = true
		t.Char = 0
		return &t
	}

	// Complex negation with multiple conditions.
	if strings.Contains(fullLine, "lookahead !=") {
		re := regexp.MustCompile(`lookahead\s*!=\s*'([^']*)'`)
		m := re.FindStringSubmatch(fullLine)
		if m != nil {
			t.IsNegated = true
			t.Char = parseCChar("'" + m[1] + "'")
			return &t
		}
	}

	// Range: ('a' <= lookahead && lookahead <= 'z')
	rangeRe := regexp.MustCompile(`'([^']*)'\s*<=\s*lookahead\s*&&\s*lookahead\s*<=\s*'([^']*)'`)
	if m := rangeRe.FindStringSubmatch(fullLine); m != nil {
		t.IsRange = true
		t.Low = parseCChar("'" + m[1] + "'")
		t.High = parseCChar("'" + m[2] + "'")
		return &t
	}

	// Range with hex: (0xNN <= lookahead && lookahead <= 0xNN)
	hexRangeRe := regexp.MustCompile(`(0x[0-9a-fA-F]+)\s*<=\s*lookahead\s*&&\s*lookahead\s*<=\s*(?:'([^']*)'|(0x[0-9a-fA-F]+))`)
	if m := hexRangeRe.FindStringSubmatch(fullLine); m != nil {
		t.IsRange = true
		low, _ := strconv.ParseInt(m[1], 0, 32)
		t.Low = rune(low)
		if m[2] != "" {
			t.High = parseCChar("'" + m[2] + "'")
		} else {
			high, _ := strconv.ParseInt(m[3], 0, 32)
			t.High = rune(high)
		}
		return &t
	}

	// Parenthesized char tests: e.g. (lookahead == '\t' || lookahead == ' ')
	if strings.Contains(fullLine, "||") {
		// Extract all character comparisons.
		charRe := regexp.MustCompile(`lookahead\s*==\s*'([^']*)'`)
		chars := charRe.FindAllStringSubmatch(fullLine, -1)
		if len(chars) > 0 {
			// If this is a set of chars, we use the first one and the rest will
			// be separate entries. For simplicity, generate individual transitions.
			// But we should check if there's also a range embedded.
			if rangeRe.MatchString(fullLine) {
				// Has range + char tests. Return range, skip chars for now.
				if m := rangeRe.FindStringSubmatch(fullLine); m != nil {
					t.IsRange = true
					t.Low = parseCChar("'" + m[1] + "'")
					t.High = parseCChar("'" + m[2] + "'")
					return &t
				}
			}
			// Use first char match.
			t.Char = parseCChar("'" + chars[0][1] + "'")
			return &t
		}
	}

	return &t
}

// detectDefault detects if the last transition is a catch-all default.
func detectDefault(state *LexState) {
	if len(state.Transitions) == 0 {
		return
	}
	last := &state.Transitions[len(state.Transitions)-1]
	if last.IsNegated && last.Char == 0 {
		state.HasDefault = true
		state.DefaultTarget = last.Target
		state.DefaultSkip = last.Skip
		// Remove it from transitions.
		state.Transitions = state.Transitions[:len(state.Transitions)-1]
	}
}

// parseCChar parses a C character literal.
func parseCChar(s string) rune {
	s = strings.Trim(s, "'")
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

// splitCSV splits a comma-separated string, respecting nested parens/quotes.
func splitCSV(s string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	inQuote := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && (i == 0 || s[i-1] != '\\') {
			inQuote = !inQuote
			current.WriteByte(c)
		} else if !inQuote && c == '(' {
			depth++
			current.WriteByte(c)
		} else if !inQuote && c == ')' {
			depth--
			current.WriteByte(c)
		} else if !inQuote && depth == 0 && c == ',' {
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
