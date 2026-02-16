// Package testgrammars provides test grammars for treesitter-go runtime validation.
//
// extscanner_language.go defines a minimal test grammar that uses an external
// scanner. The grammar recognizes a simple "heredoc" language:
//
//   program -> statement*
//   statement -> identifier '=' value
//   value -> number | heredoc
//   heredoc -> '<<' heredoc_body   (the body is scanned externally)
//
// The external scanner handles:
//   - HEREDOC_BODY: reads until a closing marker line ("END" on its own line)
//
// This exercises the full external scanner integration path:
//   - ExternalScanner interface (Scan, Serialize, Deserialize)
//   - External scanner state on subtrees
//   - Token cache invalidation with external tokens
//   - EnabledExternalTokens lookup
//   - Symbol mapping (external token index -> grammar symbol)
package testgrammars

import ts "github.com/treesitter-go/treesitter"

// Simplified external scanner test grammar symbol IDs.
// These match the symbols used in buildExtScannerLanguage().
const (
	ExtSymEnd          ts.Symbol = 0  // end of input
	ExtSymNumber       ts.Symbol = 1  // number (terminal, named)
	ExtSymHeredocMarker ts.Symbol = 2 // "<<" (terminal, anonymous)
	ExtSymHeredocBody  ts.Symbol = 3  // heredoc_body (external token, named)
	ExtSymProgram      ts.Symbol = 4  // program (non-terminal, named)
	ExtSymValue        ts.Symbol = 5  // _value (non-terminal, hidden)
	ExtSymHeredoc      ts.Symbol = 6  // heredoc (non-terminal, named)
)

// External scanner token indices (not grammar symbols — these are the
// indices into the ExternalSymbolMap and the valid_symbols array).
const (
	ExtTokenHeredocBody = 0
)

// ExtScannerLanguage returns a test language with external scanner support.
// The parse tables are hand-built for this minimal grammar.
//
// Parse states:
//   0: error/recovery
//   1: start state — expect identifier or end
//   2: after identifier — expect '='
//   3: after '=' — expect value (number or '<<')
//   4: after number — reduce statement
//   5: after '<<' — expect heredoc_body (external)
//   6: after heredoc_body — reduce heredoc
//   7: after value — reduce statement
//   8: accept state
//
// The grammar is:
//   program -> statement* (repeat via _aux_repeat)
//   statement -> identifier '=' value '\n'
//   value -> number | heredoc
//   heredoc -> '<<' heredoc_body
func ExtScannerLanguage() *ts.Language {
	return buildExtScannerLanguage()
}

// buildExtScannerLanguage builds the external scanner test language
// with a very simple grammar.
//
// Grammar:
//   program -> value
//   value -> number | heredoc_marker heredoc_body
//
// Symbols (6 total):
//   0: end
//   1: number (terminal, named)
//   2: heredoc_marker "<<" (terminal, anonymous)
//   3: heredoc_body (external terminal, named)
//   4: program (non-terminal, named, visible) — the root
//   5: _value (non-terminal, hidden)
//   6: heredoc (non-terminal, named, visible)
//
// Token count: 4 (end, number, heredoc_marker, heredoc_body)
// External tokens: 1 (heredoc_body at ext index 0)
//
// Parse states (all large for simplicity):
//   0: error
//   1: start — shift number to 2, shift << to 4, goto _value to 3, goto heredoc to 3
//   2: after number — reduce _value -> number
//   3: after value — reduce program -> value (accept via reduce+goto)
//   4: after << — shift heredoc_body to 5
//   5: after heredoc_body — reduce heredoc -> << heredoc_body
//
// Using a simple top-level: state 1 has accept on end after pushing program.
func buildExtScannerLanguage() *ts.Language {
	const (
		sEnd           ts.Symbol = 0
		sNumber        ts.Symbol = 1
		sHeredocMarker ts.Symbol = 2
		sHeredocBody   ts.Symbol = 3
		sProgram       ts.Symbol = 4
		sValue         ts.Symbol = 5
		sHeredoc       ts.Symbol = 6
		symbolCount    uint32    = 7
		tokenCount     uint32    = 4
	)

	symbolMetadata := []ts.SymbolMetadata{
		/* 0 end            */ {Visible: false, Named: false},
		/* 1 number         */ {Visible: true, Named: true},
		/* 2 <<             */ {Visible: true, Named: false},
		/* 3 heredoc_body   */ {Visible: true, Named: true},
		/* 4 program        */ {Visible: true, Named: true},
		/* 5 _value         */ {Visible: false, Named: true},
		/* 6 heredoc        */ {Visible: true, Named: true},
	}

	symbolNames := []string{
		"end", "number", "<<", "heredoc_body",
		"program", "_value", "heredoc",
	}

	// Parse actions flat array.
	parseActions := []ts.ParseActionEntry{
		/* 0  */ {Type: ts.ParseActionTypeHeader, Count: 0}, // null action

		// 1: SHIFT to state 2 (after number)
		/* 1  */ {Type: ts.ParseActionTypeHeader, Count: 1},
		/* 2  */ {Type: ts.ParseActionTypeShift, ShiftState: 2},

		// 3: SHIFT to state 4 (after <<)
		/* 3  */ {Type: ts.ParseActionTypeHeader, Count: 1},
		/* 4  */ {Type: ts.ParseActionTypeShift, ShiftState: 4},

		// 5: REDUCE _value -> number (1 child, prod 2)
		/* 5  */ {Type: ts.ParseActionTypeHeader, Count: 1},
		/* 6  */ {Type: ts.ParseActionTypeReduce, ReduceSymbol: sValue, ReduceChildCount: 1, ReduceProdID: 2},

		// 7: REDUCE program -> _value (1 child, prod 1) — this produces the root
		/* 7  */ {Type: ts.ParseActionTypeHeader, Count: 1},
		/* 8  */ {Type: ts.ParseActionTypeReduce, ReduceSymbol: sProgram, ReduceChildCount: 1, ReduceProdID: 1},

		// 9: SHIFT heredoc_body to state 5
		/* 9  */ {Type: ts.ParseActionTypeHeader, Count: 1},
		/* 10 */ {Type: ts.ParseActionTypeShift, ShiftState: 5},

		// 11: REDUCE heredoc -> << heredoc_body (2 children, prod 3)
		/* 11 */ {Type: ts.ParseActionTypeHeader, Count: 1},
		/* 12 */ {Type: ts.ParseActionTypeReduce, ReduceSymbol: sHeredoc, ReduceChildCount: 2, ReduceProdID: 3},

		// 13: REDUCE _value -> heredoc (1 child, prod 2)
		/* 13 */ {Type: ts.ParseActionTypeHeader, Count: 1},
		/* 14 */ {Type: ts.ParseActionTypeReduce, ReduceSymbol: sValue, ReduceChildCount: 1, ReduceProdID: 2},

		// 15: ACCEPT
		/* 15 */ {Type: ts.ParseActionTypeHeader, Count: 1},
		/* 16 */ {Type: ts.ParseActionTypeAccept},
	}

	// State count: 7 (0-6), all large.
	stateCount := uint32(7)
	largeStateCount := stateCount

	// Large parse table: [state * symbolCount + symbol] -> action index.
	// Non-terminals (>= tokenCount) store raw state IDs as goto targets.
	parseTable := make([]uint16, stateCount*symbolCount)
	set := func(state uint32, symbol ts.Symbol, actionIdx uint16) {
		parseTable[state*symbolCount+uint32(symbol)] = actionIdx
	}

	// State 1: start — expect number or <<
	set(1, sNumber, 1)         // shift to state 2
	set(1, sHeredocMarker, 3)  // shift to state 4
	// Non-terminal gotos (raw state IDs):
	set(1, sValue, 3)          // _value -> goto state 3
	set(1, sHeredoc, 0)        // heredoc doesn't go anywhere directly here

	// State 2: after number — reduce _value -> number
	set(2, sEnd, 5)            // reduce _value -> number

	// State 3: after _value — can accept or reduce program
	// On end: reduce program -> _value, then from state 1 goto program
	set(3, sEnd, 7)            // reduce program -> _value

	// State 4: after << — expect heredoc_body (external token)
	set(4, sHeredocBody, 9)    // shift to state 5

	// State 5: after heredoc_body — reduce heredoc -> << heredoc_body
	set(5, sEnd, 11)           // reduce heredoc

	// State 6: after heredoc reduce — reduce _value -> heredoc
	// Actually, after reducing heredoc we go back to state 1 and
	// look up goto for sHeredoc... Let me trace through this.
	//
	// Trace for input "<<END\nfoo\nEND\n":
	//   1. State 1, lex "<<", action shift to state 4
	//   2. State 4, lex heredoc_body "foo\n" (external), action shift to state 5
	//   3. State 5, lex end, action reduce heredoc (pop 2) -> back to state 1
	//   4. State 1, goto for sHeredoc -> need to set this
	//   5. State ??, reduce _value -> heredoc -> back to state 1
	//   6. State 1, goto for sValue -> state 3
	//   7. State 3, end -> reduce program -> _value -> back to state 1
	//   8. State 1, goto for sProgram -> state 6
	//   9. State 6, end -> accept
	//
	// So I need:
	//   - State 1: sHeredoc goto -> state X (where X reduces _value -> heredoc)
	//   - State X: end -> reduce _value -> heredoc (action 13)
	//   - Then back to state 1, sValue goto -> state 3
	//   - State 1: sProgram goto -> state 6
	//   - State 6: end -> accept (action 15)

	// Let me add state 6 for after-heredoc and state 7 for accept.
	// Rework with 8 states: 0-7.
	stateCount = 8
	largeStateCount = stateCount
	parseTable = make([]uint16, stateCount*symbolCount)
	set = func(state uint32, symbol ts.Symbol, actionIdx uint16) {
		parseTable[state*symbolCount+uint32(symbol)] = actionIdx
	}

	// State 1: start
	set(1, sNumber, 1)         // shift to state 2
	set(1, sHeredocMarker, 3)  // shift to state 4
	set(1, sValue, 3)          // _value goto -> state 3 (raw)
	set(1, sHeredoc, 6)        // heredoc goto -> state 6 (raw)
	set(1, sProgram, 7)        // program goto -> state 7 (raw)

	// State 2: after number — reduce to _value
	set(2, sEnd, 5)            // reduce _value -> number (pop 1, back to state 1, goto sValue=3)

	// State 3: after _value — reduce to program
	set(3, sEnd, 7)            // reduce program -> _value (pop 1, back to state 1, goto sProgram=7)

	// State 4: after << — expect heredoc_body
	set(4, sHeredocBody, 9)    // shift to state 5

	// State 5: after heredoc_body — reduce to heredoc
	set(5, sEnd, 11)           // reduce heredoc -> << heredoc_body (pop 2, back to 1, goto sHeredoc=6)

	// State 6: after heredoc — reduce to _value
	set(6, sEnd, 13)           // reduce _value -> heredoc (pop 1, back to state 1, goto sValue=3)

	// State 7: accept state
	set(7, sEnd, 15)           // accept

	// Lex modes: all states use lex state 0, only state 4 uses external lex state 1.
	lexModes := make([]ts.LexMode, stateCount)
	for i := range lexModes {
		lexModes[i] = ts.LexMode{LexState: 0, ExternalLexState: 0}
	}
	// State 4 (after <<) enables external scanner.
	lexModes[4] = ts.LexMode{LexState: 0, ExternalLexState: 1}

	// Primary state IDs: identity mapping.
	primaryStateIDs := make([]ts.StateID, stateCount)
	for i := range primaryStateIDs {
		primaryStateIDs[i] = ts.StateID(i)
	}

	// External scanner states: [externalLexState * externalTokenCount + tokenIdx] -> bool
	// State 0: no external tokens enabled
	// State 1: heredoc_body enabled
	externalScannerStates := []bool{
		false, // state 0, token 0 (heredoc_body) - disabled
		true,  // state 1, token 0 (heredoc_body) - enabled
	}

	// External symbol map: maps external token index -> grammar symbol.
	externalSymbolMap := []ts.Symbol{
		sHeredocBody, // ext token 0 -> sHeredocBody (symbol 3)
	}

	return &ts.Language{
		Version:            15,
		SymbolCount:        symbolCount,
		TokenCount:         tokenCount,
		ExternalTokenCount: 1,
		StateCount:         stateCount,
		LargeStateCount:    largeStateCount,
		ProductionIDCount:  4,
		FieldCount:         0,
		ParseTable:         parseTable,
		SmallParseTable:    nil,
		SmallParseTableMap: nil,
		ParseActions:       parseActions,
		LexModes:           lexModes,
		PrimaryStateIDs:    primaryStateIDs,
		SymbolNames:        symbolNames,
		SymbolMetadata:     symbolMetadata,
		ExternalScannerStates: externalScannerStates,
		ExternalSymbolMap:     externalSymbolMap,
		// LexFn, KeywordLexFn, NewExternalScanner set by caller.
	}
}

// HeredocScanner implements ExternalScanner for the heredoc test grammar.
// It scans for heredoc body content: everything from the current position
// until it finds "END" on its own line. The scanner maintains no state
// (stateless) — it only looks for the closing marker.
type HeredocScanner struct {
	// markerSeen tracks whether we've ever produced a heredoc_body token.
	// This is serialized/deserialized to test state roundtripping.
	markerSeen bool
}

// NewHeredocScanner creates a new HeredocScanner (ExternalScannerFactory).
func NewHeredocScanner() ts.ExternalScanner {
	return &HeredocScanner{}
}

// Scan attempts to recognize a heredoc body token.
// It reads everything until "END" appears at the start of a line.
func (s *HeredocScanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	if len(validSymbols) == 0 || !validSymbols[ExtTokenHeredocBody] {
		return false
	}

	// Skip the initial newline after << if present.
	if !lexer.EOF() && lexer.Lookahead == '\n' {
		lexer.Advance(true) // skip, not part of token
	}

	// Read until we find "END" at the start of a line.
	atLineStart := true
	foundEnd := false
	bodyLen := 0

	for !lexer.EOF() {
		ch := lexer.Lookahead

		if atLineStart && ch == 'E' {
			// Check for "END" followed by newline or EOF.
			lexer.MarkEnd() // Mark before "END" as the body end
			lexer.Advance(false)
			if !lexer.EOF() && lexer.Lookahead == 'N' {
				lexer.Advance(false)
				if !lexer.EOF() && lexer.Lookahead == 'D' {
					lexer.Advance(false)
					if lexer.EOF() || lexer.Lookahead == '\n' {
						// Consume the newline after END if present.
						if !lexer.EOF() {
							lexer.Advance(false)
						}
						lexer.MarkEnd()
						foundEnd = true
						break
					}
				}
			}
			// Not "END" — continue scanning.
			atLineStart = false
			bodyLen++
			continue
		}

		if ch == '\n' {
			atLineStart = true
		} else {
			atLineStart = false
		}

		lexer.Advance(false)
		bodyLen++
	}

	if !foundEnd && bodyLen == 0 {
		return false
	}

	// Even if we didn't find END (unterminated heredoc), accept what we have.
	if !foundEnd {
		lexer.MarkEnd()
	}

	lexer.AcceptToken(ts.Symbol(ExtTokenHeredocBody))
	s.markerSeen = true
	return true
}

// Serialize writes the scanner state to buf.
func (s *HeredocScanner) Serialize(buf []byte) uint32 {
	if len(buf) < 1 {
		return 0
	}
	if s.markerSeen {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	return 1
}

// Deserialize restores scanner state from data.
func (s *HeredocScanner) Deserialize(data []byte) {
	s.markerSeen = false
	if len(data) >= 1 && data[0] == 1 {
		s.markerSeen = true
	}
}

// ExtScannerLexFn is the main lex function for the external scanner test grammar.
// It handles: whitespace skipping, number literals, "<<" heredoc markers.
func ExtScannerLexFn(lexer *ts.Lexer, state ts.StateID) bool {
	// Skip whitespace (but not newlines — those might be significant).
	for !lexer.EOF() && (lexer.Lookahead == ' ' || lexer.Lookahead == '\t') {
		lexer.Skip()
	}

	if lexer.EOF() {
		return false
	}

	ch := lexer.Lookahead

	// Number literal.
	if ch >= '0' && ch <= '9' {
		for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(1)) // sNumber
		return true
	}

	// Heredoc marker "<<"
	if ch == '<' {
		lexer.Advance(false)
		if !lexer.EOF() && lexer.Lookahead == '<' {
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.AcceptToken(ts.Symbol(2)) // sHeredocMarker
			return true
		}
		return false
	}

	// Skip newlines (they're not significant in the non-external-scanner states).
	if ch == '\n' || ch == '\r' {
		lexer.Skip()
		return ExtScannerLexFn(lexer, state)
	}

	return false
}

// ExtScannerLanguageWithLex returns the external scanner test language
// with lex function and external scanner factory configured.
func ExtScannerLanguageWithLex() *ts.Language {
	lang := ExtScannerLanguage()
	lang.LexFn = ExtScannerLexFn
	lang.NewExternalScanner = NewHeredocScanner
	return lang
}
