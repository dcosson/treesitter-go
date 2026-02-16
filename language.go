package treesitter

// Language holds the compiled parse tables and metadata for a grammar.
// It is created by the grammar code generator and is safe for concurrent
// read access after creation.
type Language struct {
	// Metadata
	Version         uint32 // ABI version (we target 15)
	SymbolCount     uint32
	AliasCount      uint32
	TokenCount      uint32
	ExternalTokenCount uint32
	StateCount      uint32
	LargeStateCount uint32
	ProductionIDCount uint32
	FieldCount      uint32
	MaxAliasSequenceLength uint16
	PrimaryStateIDs []StateID // maps state -> primary (canonical) state

	// Parse tables — same encoding as C tree-sitter.
	// ParseTable is a dense 2D array for "large" states: [state * symbolCount + symbol] -> action index.
	ParseTable []uint16
	// SmallParseTable is a compressed grouped format for "small" states.
	SmallParseTable []uint16
	// SmallParseTableMap maps (small state index) -> offset into SmallParseTable.
	SmallParseTableMap []uint32

	// ParseActions is a flat array of action entries.
	// Each table lookup yields an index into this array. The entry at that
	// index is a header (Type=Header, Count=N, Reusable=bool), followed by
	// N action entries.
	ParseActions []ParseActionEntry

	// LexModes maps parse state -> lex mode (lex state + external lex state).
	LexModes []LexMode

	// LexFn is the generated main lex function.
	LexFn func(lexer *Lexer, state StateID) bool
	// KeywordLexFn is the generated keyword lex function (may be nil).
	KeywordLexFn func(lexer *Lexer, state StateID) bool
	// KeywordCaptureToken is the symbol for the keyword capture token.
	KeywordCaptureToken Symbol

	// Symbol metadata
	SymbolNames    []string
	SymbolMetadata []SymbolMetadata

	// Field maps: production_id -> slice of field entries
	FieldMapSlices  []FieldMapSlice
	FieldMapEntries []FieldMapEntry
	FieldNames      []string

	// Alias sequences: indexed by production_id * max_alias_sequence_length.
	// Each sequence is max_alias_sequence_length symbols; 0 means "no alias".
	AliasSequences []Symbol

	// Reserved words (ABI v15): flat 2D bool table.
	// [keyword_index * token_count + token_id] -> bool
	ReservedWords []bool
	ReservedWordCount uint32
	ReservedWordSetCount uint32

	// Supertypes (ABI v15)
	SupertypeSymbols []Symbol

	// External scanner
	ExternalScannerStates []bool   // [extLexState * extTokenCount + tokenIdx]
	ExternalSymbolMap     []Symbol // maps external token index -> grammar symbol
	NewExternalScanner    ExternalScannerFactory
}

// ExternalScanner handles context-sensitive lexing that the regular DFA cannot express.
type ExternalScanner interface {
	Scan(lexer *Lexer, validSymbols []bool) bool
	Serialize(buf []byte) uint32
	Deserialize(data []byte)
}

// ExternalScannerFactory creates new ExternalScanner instances.
type ExternalScannerFactory func() ExternalScanner

// ExportLookup is an exported wrapper for lookup, used by tests in other packages.
func (l *Language) ExportLookup(state StateID, symbol Symbol) uint16 {
	return l.lookup(state, symbol)
}

// ExportTableEntry is an exported wrapper for tableEntry, used by tests in other packages.
func (l *Language) ExportTableEntry(state StateID, symbol Symbol) TableEntry {
	return l.tableEntry(state, symbol)
}

// lookup returns the action index for a (state, symbol) pair.
// This mirrors ts_language_lookup() from the C implementation.
func (l *Language) lookup(state StateID, symbol Symbol) uint16 {
	if uint32(state) < l.LargeStateCount {
		idx := uint32(state)*l.SymbolCount + uint32(symbol)
		return l.ParseTable[idx]
	}
	mapIdx := uint32(state) - l.LargeStateCount
	offset := l.SmallParseTableMap[mapIdx]
	data := l.SmallParseTable[offset:]
	groupCount := data[0]
	pos := uint32(1)
	for i := uint16(0); i < groupCount; i++ {
		value := data[pos]
		symCount := data[pos+1]
		pos += 2
		for j := uint16(0); j < symCount; j++ {
			if data[pos] == uint16(symbol) {
				return value
			}
			pos++
		}
	}
	return 0
}

// tableEntry returns the parse actions for a (state, symbol) pair.
func (l *Language) tableEntry(state StateID, symbol Symbol) TableEntry {
	if symbol == SymbolError || symbol == SymbolErrorRepeat {
		return TableEntry{}
	}
	actionIndex := l.lookup(state, symbol)
	if actionIndex == 0 {
		return TableEntry{}
	}
	header := l.ParseActions[actionIndex]
	count := int(header.Count)
	return TableEntry{
		Actions:     l.ParseActions[actionIndex+1 : actionIndex+1+uint16(count)],
		ActionCount: header.Count,
		Reusable:    header.Reusable,
	}
}

// nextState returns the state the parser transitions to after shifting
// or reducing with the given symbol in the given state. This is used for
// goto transitions (non-terminal symbols after a reduce).
func (l *Language) nextState(state StateID, symbol Symbol) StateID {
	if symbol == SymbolError || symbol == SymbolErrorRepeat {
		return 0
	}
	actionIndex := l.lookup(state, symbol)
	if actionIndex == 0 {
		return 0
	}
	entry := l.ParseActions[actionIndex]
	if entry.Type == ParseActionTypeHeader && entry.Count > 0 {
		action := l.ParseActions[actionIndex+1]
		if action.Type == ParseActionTypeShift {
			return action.ShiftState
		}
	}
	return 0
}

// SymbolName returns the name of a symbol.
func (l *Language) SymbolName(symbol Symbol) string {
	if int(symbol) < len(l.SymbolNames) {
		return l.SymbolNames[symbol]
	}
	return ""
}

// SymbolIsNamed returns whether a symbol is a named node.
func (l *Language) SymbolIsNamed(symbol Symbol) bool {
	if int(symbol) < len(l.SymbolMetadata) {
		return l.SymbolMetadata[symbol].Named
	}
	return false
}

// SymbolIsVisible returns whether a symbol is visible in the tree.
func (l *Language) SymbolIsVisible(symbol Symbol) bool {
	if int(symbol) < len(l.SymbolMetadata) {
		return l.SymbolMetadata[symbol].Visible
	}
	return false
}

// FieldName returns the name of a field.
func (l *Language) FieldName(field FieldID) string {
	if int(field) < len(l.FieldNames) {
		return l.FieldNames[field]
	}
	return ""
}

// FieldMapForProduction returns the field map entries for a given production ID.
func (l *Language) FieldMapForProduction(prodID uint16) []FieldMapEntry {
	if int(prodID) >= len(l.FieldMapSlices) {
		return nil
	}
	slice := l.FieldMapSlices[prodID]
	start := int(slice.Index)
	end := start + int(slice.Length)
	if end > len(l.FieldMapEntries) {
		return nil
	}
	return l.FieldMapEntries[start:end]
}

// AliasForProduction returns the alias symbol (if any) for a child at the
// given index in the given production.
func (l *Language) AliasForProduction(prodID uint16, childIndex int) Symbol {
	if l.MaxAliasSequenceLength == 0 || prodID == 0 {
		return 0
	}
	idx := int(prodID) * int(l.MaxAliasSequenceLength)
	if childIndex >= int(l.MaxAliasSequenceLength) {
		return 0
	}
	seqIdx := idx + childIndex
	if seqIdx >= len(l.AliasSequences) {
		return 0
	}
	return l.AliasSequences[seqIdx]
}

// IsReservedWord checks if a given token is a reserved word in the given
// reserved word set (ABI v15).
func (l *Language) IsReservedWord(setIndex uint32, tokenSymbol Symbol) bool {
	if l.ReservedWordCount == 0 || setIndex >= l.ReservedWordSetCount {
		return false
	}
	idx := setIndex*l.ReservedWordCount + uint32(tokenSymbol)
	if idx >= uint32(len(l.ReservedWords)) {
		return false
	}
	return l.ReservedWords[idx]
}
