// Package generate extracts compiled parse tables from tree-sitter's generated
// parser.c and produces equivalent Go source code.
package generate

// Grammar holds the complete extracted data from a tree-sitter parser.c file.
// It is the intermediate representation between parsing and code generation.
type Grammar struct {
	// Name is the grammar name (e.g., "json", "go", "javascript").
	Name string

	// Constants from #define directives.
	SymbolCount            int
	AliasCount             int
	TokenCount             int
	ExternalTokenCount     int
	StateCount             int
	LargeStateCount        int
	ProductionIDCount      int
	FieldCount             int
	MaxAliasSequenceLength int

	// Symbols
	SymbolNames    []string
	SymbolMetadata []SymMeta

	// Parse actions: flat array of action entries.
	ParseActions []ActionEntry

	// Large state parse table: [state * symbolCount + symbol] -> action index.
	ParseTable []uint16

	// Small state parse table (compressed grouped format).
	SmallParseTable    []uint16
	SmallParseTableMap []uint32

	// Lex modes: one per parse state.
	LexModes []LexModeEntry

	// Primary state IDs: maps state -> canonical state.
	PrimaryStateIDs []uint16

	// Alias sequences: flat 2D array [prodID * maxAliasSeqLen + childIdx] -> symbol.
	AliasSequences []uint16

	// Non-terminal alias map (for aliased non-terminals).
	NonTerminalAliasMap []uint16

	// Field maps
	FieldNames      []string
	FieldMapSlices  []FieldSlice
	FieldMapEntries []FieldEntry

	// Lex function DFA states.
	LexStates []LexState

	// Keyword lex function DFA states (may be empty).
	KeywordLexStates []LexState
	// KeywordCaptureToken is the symbol used for keyword capture (0 if none).
	KeywordCaptureToken uint16

	// External scanner state table.
	// [extLexState * extTokenCount + tokenIdx] -> bool
	ExternalScannerStates []bool
	ExternalSymbolMap     []uint16

	// Supertype symbols.
	SupertypeSymbols []uint16

	// Public symbol map: maps internal symbol -> public symbol.
	PublicSymbolMap []uint16

	// Internal: enum maps built during extraction for resolving C identifiers.
	symbolEnum map[string]int // C enum name -> integer value
	fieldEnum  map[string]int // C field enum name -> integer value
}

// SymMeta holds visibility and naming info for a grammar symbol.
type SymMeta struct {
	Visible   bool
	Named     bool
	Supertype bool
}

// ActionEntry is a parsed action from ts_parse_actions[].
type ActionEntry struct {
	// IsHeader indicates this is a header entry (count + reusable).
	IsHeader bool

	// Header fields
	Count    int
	Reusable bool

	// Action type: "shift", "reduce", "accept", "recover"
	ActionType string

	// Shift fields
	ShiftState      uint16
	ShiftExtra      bool
	ShiftRepetition bool

	// Reduce fields
	ReduceSymbol     uint16
	ReduceChildCount int
	ReduceDynPrec    int
	ReduceProdID     uint16
}

// LexModeEntry corresponds to a TSLexMode.
type LexModeEntry struct {
	LexState         uint16
	ExternalLexState uint16
}

// FieldSlice identifies a range in the field entries array.
type FieldSlice struct {
	Index  uint16
	Length uint16
}

// FieldEntry maps a field ID to a child position.
type FieldEntry struct {
	FieldID    uint16
	ChildIndex uint16
	Inherited  bool
}

// LexState represents one DFA state from ts_lex or ts_lex_keywords.
type LexState struct {
	ID int

	// AcceptToken is non-zero if this state accepts a token.
	// Corresponds to ACCEPT_TOKEN(sym).
	AcceptToken uint16

	// Transitions are the character-based transitions from this state.
	Transitions []LexTransition

	// HasEOFCheck is true if this state checks for EOF.
	HasEOFCheck bool
	EOFTarget   int // target state for EOF transition

	// EOFAccept is true if EOF leads to ACCEPT_TOKEN (not state transition).
	EOFAccept      bool
	EOFAcceptToken uint16

	// DefaultAdvance is set when the state has a catch-all "if (lookahead != 0) ADVANCE(n)".
	HasDefault    bool
	DefaultTarget int
	DefaultSkip   bool // true if default uses SKIP instead of ADVANCE
}

// LexTransition is a single character-to-state transition in the DFA.
type LexTransition struct {
	// Single character match
	Char rune
	// Or range match: [Low, High]
	IsRange bool
	Low     rune
	High    rune
	// Negated match: "lookahead != X"
	IsNegated bool

	// Target state (used with ADVANCE/SKIP)
	Target int
	Skip   bool // true for SKIP, false for ADVANCE
}
