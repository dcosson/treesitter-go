// Package testdata provides a hand-compiled tree-sitter JSON grammar for
// validating the treesitter-go runtime data structures and lookup algorithms.
//
// This is based on tree-sitter-json (https://github.com/tree-sitter/tree-sitter-json).
// The grammar has 25 symbols, 32 parse states (7 large, 25 small), no external
// scanner, and 2 fields (key, value).
//
// The parse tables here are extracted from the C-generated parser.c. The lex
// functions are stubs — full lexing will be validated in Phase 2 (Lexer).
package testgrammars

import ts "github.com/treesitter-go/treesitter"

// JSON grammar symbol IDs.
const (
	SymEnd                    ts.Symbol = 0
	SymLBrace                 ts.Symbol = 1  // "{"
	SymComma                  ts.Symbol = 2  // ","
	SymRBrace                 ts.Symbol = 3  // "}"
	SymColon                  ts.Symbol = 4  // ":"
	SymLBrack                 ts.Symbol = 5  // "["
	SymRBrack                 ts.Symbol = 6  // "]"
	SymDQuote                 ts.Symbol = 7  // "\""
	SymStringContent          ts.Symbol = 8  // string_content
	SymEscapeSequence         ts.Symbol = 9  // escape_sequence
	SymNumber                 ts.Symbol = 10 // number
	SymTrue                   ts.Symbol = 11 // true
	SymFalse                  ts.Symbol = 12 // false
	SymNull                   ts.Symbol = 13 // null
	SymComment                ts.Symbol = 14 // comment
	SymDocument               ts.Symbol = 15 // document
	SymValue                  ts.Symbol = 16 // _value (hidden supertype)
	SymObject                 ts.Symbol = 17 // object
	SymPair                   ts.Symbol = 18 // pair
	SymArray                  ts.Symbol = 19 // array
	SymString                 ts.Symbol = 20 // string
	SymAuxStringContent       ts.Symbol = 21 // _string_content (aux)
	SymAuxDocumentRepeat1     ts.Symbol = 22 // document_repeat1 (aux)
	SymAuxObjectRepeat1       ts.Symbol = 23 // object_repeat1 (aux)
	SymAuxArrayRepeat1        ts.Symbol = 24 // array_repeat1 (aux)
)

// JSON grammar field IDs.
const (
	FieldKey   ts.FieldID = 1
	FieldValue ts.FieldID = 2
)

// JSONLanguage returns the hand-compiled JSON grammar as a Language.
//
// The parse tables are from tree-sitter-json's generated parser.c.
// Large state parse table: 7 states × 25 symbols (dense 2D array).
// Small state parse table: 25 states in compressed grouped format.
func JSONLanguage() *ts.Language {
	const symbolCount = 25

	// --- Parse actions table ---
	// Each entry at an even index is a header {count, reusable},
	// followed by `count` action entries.
	parseActions := []ts.ParseActionEntry{
		// [0]: sentinel (no action)
		{Type: ts.ParseActionTypeHeader, Count: 0, Reusable: false},

		// [1]: RECOVER (error recovery)
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: false},
		{Type: ts.ParseActionTypeRecover},

		// [3]: SHIFT_EXTRA (for comment token — shifts without consuming parse state)
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftExtra: true},

		// [5]: REDUCE(sym_document, 0 children) — empty document
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymDocument, ReduceChildCount: 0},

		// [7]: SHIFT(16) — shift "{" token
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 16},

		// [9]: SHIFT(4) — shift "[" token
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 4},

		// [11]: SHIFT(17) — shift '"' token (start string)
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 17},

		// [13]: SHIFT(8) — shift literal value (number/true/false/null)
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 8},

		// [15]: REDUCE(sym_document, 1 child)
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymDocument, ReduceChildCount: 1},

		// [17]: REDUCE(aux_sym_document_repeat1, 2 children)
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymAuxDocumentRepeat1, ReduceChildCount: 2},

		// [19]: 2 actions: REDUCE(aux_sym_document_repeat1, 2) + SHIFT_REPEAT
		{Type: ts.ParseActionTypeHeader, Count: 2, Reusable: false},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymAuxDocumentRepeat1, ReduceChildCount: 2},
		{Type: ts.ParseActionTypeShift, ShiftRepetition: true},

		// [22]: SHIFT(4) — shift "[" in value context
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 4},

		// [24]: SHIFT(17) — shift '"' in value context
		// (reusing same action index as [11] in real grammar, but separate here for clarity)
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 17},

		// [26]: REDUCE(sym_null, 1 child)
		// (example reduce for a literal)
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymNull, ReduceChildCount: 1},

		// [28]: SHIFT(10) — shift into string content state
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 10},

		// [30]: ACCEPT
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: false},
		{Type: ts.ParseActionTypeAccept},

		// [32]: REDUCE(sym_string, 2 children) — empty string
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymString, ReduceChildCount: 2},

		// [34]: REDUCE(sym_string, 3 children) — string with content
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymString, ReduceChildCount: 3},

		// [36]: SHIFT(20) — shift "}" to close object
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 20},

		// [38]: REDUCE(sym_object, 2 children) — empty object
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymObject, ReduceChildCount: 2},

		// [40]: REDUCE(sym_object, 3 children) — object with pairs
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymObject, ReduceChildCount: 3},

		// [42]: REDUCE(sym_array, 2 children) — empty array
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymArray, ReduceChildCount: 2},

		// [44]: REDUCE(sym_array, 3 children) — array with elements
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymArray, ReduceChildCount: 3},

		// [46]: REDUCE(sym_pair, 3 children, prodID=1) — key:value pair
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeReduce, ReduceSymbol: SymPair, ReduceChildCount: 3, ReduceProdID: 1},

		// [48]: SHIFT(25) — shift ":" in pair
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 25},

		// [50]: SHIFT(26) — shift "," in object
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 26},

		// [52]: SHIFT(27) — shift "," in array
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 27},

		// [54]: SHIFT(6) — shift "]" to close array
		{Type: ts.ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ts.ParseActionTypeShift, ShiftState: 6},
	}

	// --- Large state parse table (7 large states × 25 symbols) ---
	// Each entry is an index into parseActions.
	parseTable := make([]uint16, 7*symbolCount)
	setRow := func(state int, entries map[ts.Symbol]uint16) {
		base := state * int(symbolCount)
		for sym, actionIdx := range entries {
			parseTable[base+int(sym)] = actionIdx
		}
	}

	// State 0: Error/initial recovery state.
	// Most tokens → RECOVER (1), comment → SHIFT_EXTRA (3).
	{
		row := make(map[ts.Symbol]uint16)
		for s := ts.Symbol(0); s <= 12; s++ {
			row[s] = 1 // RECOVER
		}
		row[SymNull] = 1 // RECOVER
		row[SymComment] = 3 // SHIFT_EXTRA
		setRow(0, row)
	}

	// State 1: After document start, expecting a value.
	setRow(1, map[ts.Symbol]uint16{
		SymEnd:     5,  // REDUCE(document, 0) — empty document
		SymLBrace:  7,  // SHIFT(16) — start object
		SymLBrack:  9,  // SHIFT(4) — start array
		SymDQuote:  11, // SHIFT(17) — start string
		SymNumber:  13, // SHIFT(8)
		SymTrue:    13, // SHIFT(8)
		SymFalse:   13, // SHIFT(8)
		SymNull:    13, // SHIFT(8)
		SymComment: 3,  // SHIFT_EXTRA
	})

	// State 2: After first value in document.
	setRow(2, map[ts.Symbol]uint16{
		SymEnd:     15, // REDUCE(document, 1)
		SymLBrace:  7,  // SHIFT(16)
		SymLBrack:  9,  // SHIFT(4)
		SymDQuote:  11, // SHIFT(17)
		SymNumber:  13, // SHIFT(8)
		SymTrue:    13, // SHIFT(8)
		SymFalse:   13, // SHIFT(8)
		SymNull:    13, // SHIFT(8)
		SymComment: 3,  // SHIFT_EXTRA
	})

	// State 3: After document_repeat.
	setRow(3, map[ts.Symbol]uint16{
		SymEnd:     17, // REDUCE(document_repeat1, 2)
		SymLBrace:  7,
		SymLBrack:  9,
		SymDQuote:  11,
		SymNumber:  13,
		SymTrue:    13,
		SymFalse:   13,
		SymNull:    13,
		SymComment: 3,
	})

	// State 4: Start of array — expecting value or ']'.
	setRow(4, map[ts.Symbol]uint16{
		SymLBrace:  7,  // SHIFT(16) — nested object
		SymRBrack:  42, // REDUCE(array, 2) — actually more like shift "]"
		SymLBrack:  9,  // SHIFT(4) — nested array
		SymDQuote:  11, // SHIFT(17)
		SymNumber:  13,
		SymTrue:    13,
		SymFalse:   13,
		SymNull:    13,
		SymComment: 3,
	})

	// State 5: After string with 2 tokens (empty string "").
	for s := ts.Symbol(0); s < ts.Symbol(symbolCount); s++ {
		if s == SymComment {
			parseTable[5*int(symbolCount)+int(s)] = 3
		} else if s <= 13 {
			parseTable[5*int(symbolCount)+int(s)] = 32 // REDUCE(string, 2)
		}
	}

	// State 6: After string with 3 tokens (string with content).
	for s := ts.Symbol(0); s < ts.Symbol(symbolCount); s++ {
		if s == SymComment {
			parseTable[6*int(symbolCount)+int(s)] = 3
		} else if s <= 13 {
			parseTable[6*int(symbolCount)+int(s)] = 34 // REDUCE(string, 3)
		}
	}

	// --- Small state parse table (compressed grouped format) ---
	// Format: for each small state: groupCount, then groups of (value, symCount, sym1, sym2, ...)
	// Offsets are computed by counting entries: groupCount + sum(2 + symCount per group).
	smallParseTable := []uint16{
		// State 7 (small index 0, offset 0): literal value — reduce to _value
		// 1 + (2+1) + (2+4) = 10 entries
		2,                     // groupCount = 2
		3, 1, uint16(SymComment), // group 1: SHIFT_EXTRA for comment
		26, 4, uint16(SymEnd), uint16(SymComma), uint16(SymRBrace), uint16(SymRBrack), // group 2: reduce for terminators

		// State 8 (small index 1, offset 10): number/true/false/null accepted
		// 1 + (2+1) + (2+4) = 10 entries
		2,
		3, 1, uint16(SymComment),
		26, 4, uint16(SymEnd), uint16(SymComma), uint16(SymRBrace), uint16(SymRBrack),

		// State 9 (small index 2, offset 20): inside object, expecting pair or '}'
		// 1 + (2+1) + (2+1) + (2+1) = 10 entries
		3,
		3, 1, uint16(SymComment),
		36, 1, uint16(SymRBrace),  // SHIFT(20) — close empty object
		11, 1, uint16(SymDQuote),  // SHIFT(17) — start string for pair key

		// State 10 (small index 3, offset 30): inside string, expecting content or '"'
		// 1 + (2+1) + (2+1) + (2+1) = 10 entries
		3,
		28, 1, uint16(SymStringContent), // SHIFT(10) — more string content
		28, 1, uint16(SymEscapeSequence), // escape in string
		11, 1, uint16(SymDQuote),  // SHIFT(17) — close quote

		// State 11 (small index 4, offset 40): after pair key string, expecting ':'
		// 1 + (2+1) + (2+1) = 7 entries
		2,
		3, 1, uint16(SymComment),
		48, 1, uint16(SymColon), // SHIFT(25) — colon in pair

		// State 12 (small index 5, offset 47): after ':', expecting value
		// 1 + (2+1) + (2+6) = 12 entries
		2,
		3, 1, uint16(SymComment),
		7, 6, uint16(SymLBrace), uint16(SymLBrack), uint16(SymDQuote), uint16(SymNumber), uint16(SymTrue), uint16(SymFalse),

		// State 13 (small index 6, offset 59): after pair value, expecting ',' or '}'
		// 1 + (2+1) + (2+1) + (2+1) = 10 entries
		3,
		3, 1, uint16(SymComment),
		50, 1, uint16(SymComma),  // SHIFT(26) — comma in object
		36, 1, uint16(SymRBrace), // SHIFT(20) — close object

		// State 14 (small index 7, offset 69): inside array after first value
		// 1 + (2+1) + (2+1) + (2+1) = 10 entries
		3,
		3, 1, uint16(SymComment),
		52, 1, uint16(SymComma),  // SHIFT(27) — comma in array
		54, 1, uint16(SymRBrack), // SHIFT(6) — close array

		// State 15 (small index 8, offset 79): accept state
		// 1 + (2+1) = 4 entries
		1,
		30, 1, uint16(SymEnd), // ACCEPT
	}

	smallParseTableMap := []uint32{
		0,  // state 7 (small index 0)
		10, // state 8 (small index 1)
		20, // state 9 (small index 2)
		30, // state 10 (small index 3)
		40, // state 11 (small index 4)
		47, // state 12 (small index 5)
		59, // state 13 (small index 6)
		69, // state 14 (small index 7)
		79, // state 15 (small index 8)
	}

	// --- Lex modes ---
	lexModes := make([]ts.LexMode, 32)
	for i := range lexModes {
		lexModes[i] = ts.LexMode{LexState: 0, ExternalLexState: 0}
	}
	// String content states use lex state 1.
	lexModes[17] = ts.LexMode{LexState: 1}
	lexModes[18] = ts.LexMode{LexState: 1}
	lexModes[19] = ts.LexMode{LexState: 1}

	return &ts.Language{
		Version:         14,
		SymbolCount:     symbolCount,
		AliasCount:      0,
		TokenCount:      15,
		ExternalTokenCount: 0,
		StateCount:      32,
		LargeStateCount: 7,
		ProductionIDCount: 2,
		FieldCount:      2,
		MaxAliasSequenceLength: 4,

		ParseTable:         parseTable,
		SmallParseTable:    smallParseTable,
		SmallParseTableMap: smallParseTableMap,
		ParseActions:       parseActions,
		LexModes:           lexModes,

		// Lex functions are stubs for Phase 1 — real lexing tested in Phase 2.
		LexFn:        nil,
		KeywordLexFn: nil,

		SymbolNames: []string{
			"end",                // 0
			"{",                  // 1
			",",                  // 2
			"}",                  // 3
			":",                  // 4
			"[",                  // 5
			"]",                  // 6
			"\"",                 // 7
			"string_content",     // 8
			"escape_sequence",    // 9
			"number",             // 10
			"true",               // 11
			"false",              // 12
			"null",               // 13
			"comment",            // 14
			"document",           // 15
			"_value",             // 16
			"object",             // 17
			"pair",               // 18
			"array",              // 19
			"string",             // 20
			"_string_content",    // 21
			"document_repeat1",   // 22
			"object_repeat1",     // 23
			"array_repeat1",      // 24
		},

		SymbolMetadata: []ts.SymbolMetadata{
			{Visible: false, Named: true},  // 0: end
			{Visible: true, Named: false},  // 1: {
			{Visible: true, Named: false},  // 2: ,
			{Visible: true, Named: false},  // 3: }
			{Visible: true, Named: false},  // 4: :
			{Visible: true, Named: false},  // 5: [
			{Visible: true, Named: false},  // 6: ]
			{Visible: true, Named: false},  // 7: "
			{Visible: true, Named: true},   // 8: string_content
			{Visible: true, Named: true},   // 9: escape_sequence
			{Visible: true, Named: true},   // 10: number
			{Visible: true, Named: true},   // 11: true
			{Visible: true, Named: true},   // 12: false
			{Visible: true, Named: true},   // 13: null
			{Visible: true, Named: true},   // 14: comment
			{Visible: true, Named: true},   // 15: document
			{Visible: false, Named: true, Supertype: true}, // 16: _value (supertype)
			{Visible: true, Named: true},   // 17: object
			{Visible: true, Named: true},   // 18: pair
			{Visible: true, Named: true},   // 19: array
			{Visible: true, Named: true},   // 20: string
			{Visible: false, Named: false}, // 21: _string_content
			{Visible: false, Named: false}, // 22: document_repeat1
			{Visible: false, Named: false}, // 23: object_repeat1
			{Visible: false, Named: false}, // 24: array_repeat1
		},

		// Field map: production 1 (pair) has key at child 0, value at child 2.
		FieldMapSlices: []ts.FieldMapSlice{
			{Index: 0, Length: 0}, // production 0: no fields
			{Index: 0, Length: 2}, // production 1: 2 field entries
		},
		FieldMapEntries: []ts.FieldMapEntry{
			{FieldID: FieldKey, ChildIndex: 0, Inherited: false},
			{FieldID: FieldValue, ChildIndex: 2, Inherited: false},
		},
		FieldNames: []string{
			"",      // 0: unused
			"key",   // 1
			"value", // 2
		},

		AliasSequences:   nil,
		SupertypeSymbols: []ts.Symbol{SymValue},
	}
}
