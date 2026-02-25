package treesitter

import "github.com/treesitter-go/treesitter/internal/core"

// Symbol identifies a grammar symbol (terminal or non-terminal).
// Values 0-1 are reserved: 0 = ts_builtin_sym_end, 1 = ts_builtin_sym_error.
type Symbol = core.Symbol

// Builtin symbol constants matching the C tree-sitter runtime.
const (
	SymbolEnd         Symbol = core.SymbolEnd
	SymbolError       Symbol = core.SymbolError
	SymbolErrorRepeat Symbol = core.SymbolErrorRepeat
)

// StateID identifies a parse state in the grammar's LR automaton.
type StateID = core.StateID

// FieldID identifies a field name in the grammar (e.g., "name", "body").
// 0 means no field.
type FieldID = core.FieldID

// Point represents a position in a source document as row/column.
type Point = core.Point

// Range represents a byte range with associated positions.
type Range = core.Range

// Length represents a size or offset with both byte count and position.
// Used for subtree padding and size.
type Length = core.Length

// LengthZero is the zero value for Length.
var LengthZero = core.LengthZero

// LengthAdd returns the sum of two Lengths.
func LengthAdd(a, b Length) Length {
	return core.LengthAdd(a, b)
}

// LengthSub returns a - b. Caller must ensure a >= b.
func LengthSub(a, b Length) Length {
	return core.LengthSub(a, b)
}

// InputEdit describes an edit to a source document.
type InputEdit = core.InputEdit

// SymbolMetadata holds visibility and naming information for a grammar symbol.
type SymbolMetadata = core.SymbolMetadata

// LexStateNoLookahead is the sentinel value for lex states that should not
// produce a token. In the C tree-sitter runtime, this is represented as
// (TSStateId)(-1) in the lex modes array. When the parser encounters this,
// it returns a null subtree, causing the parser to use SymbolEnd for action
// lookup (triggering reductions from non-terminal extra end states like
// heredoc_body) and then re-lex with the new state.
const LexStateNoLookahead uint16 = core.LexStateNoLookahead

// LexMode describes the lex state for a given parse state.
type LexMode = core.LexMode

// FieldMapSlice identifies a range within the field map entries array.
type FieldMapSlice = core.FieldMapSlice

// FieldMapEntry maps a field ID to a child index within a production.
type FieldMapEntry = core.FieldMapEntry

// ParseActionType distinguishes the kind of parse action.
type ParseActionType = core.ParseActionType

const (
	ParseActionTypeHeader  ParseActionType = core.ParseActionTypeHeader
	ParseActionTypeShift   ParseActionType = core.ParseActionTypeShift
	ParseActionTypeReduce  ParseActionType = core.ParseActionTypeReduce
	ParseActionTypeAccept  ParseActionType = core.ParseActionTypeAccept
	ParseActionTypeRecover ParseActionType = core.ParseActionTypeRecover
)

// ParseActionEntry is a single entry in the parse actions table.
// In C, this is a union of a header entry and an action entry.
// We use a flat struct for clarity; the Type field discriminates usage.
type ParseActionEntry = core.ParseActionEntry

// TableEntry is the result of a parse table lookup for a (state, symbol) pair.
// It contains a pointer into the actions slice plus the header metadata.
type TableEntry = core.TableEntry
