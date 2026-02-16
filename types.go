package treesitter

// Symbol identifies a grammar symbol (terminal or non-terminal).
// Values 0-1 are reserved: 0 = ts_builtin_sym_end, 1 = ts_builtin_sym_error.
type Symbol uint16

// Builtin symbol constants matching the C tree-sitter runtime.
const (
	SymbolEnd         Symbol = 0
	SymbolError       Symbol = 65535 // uint16 max
	SymbolErrorRepeat Symbol = 65534
)

// StateID identifies a parse state in the grammar's LR automaton.
type StateID uint16

// FieldID identifies a field name in the grammar (e.g., "name", "body").
// 0 means no field.
type FieldID uint16

// Point represents a position in a source document as row/column.
type Point struct {
	Row    uint32
	Column uint32
}

// Range represents a byte range with associated positions.
type Range struct {
	StartPoint Point
	EndPoint   Point
	StartByte  uint32
	EndByte    uint32
}

// Length represents a size or offset with both byte count and position.
// Used for subtree padding and size.
type Length struct {
	Bytes  uint32
	Point  Point
}

// LengthZero is the zero value for Length.
var LengthZero = Length{}

// LengthAdd returns the sum of two Lengths.
func LengthAdd(a, b Length) Length {
	result := Length{Bytes: a.Bytes + b.Bytes}
	if b.Point.Row > 0 {
		result.Point.Row = a.Point.Row + b.Point.Row
		result.Point.Column = b.Point.Column
	} else {
		result.Point.Row = a.Point.Row
		result.Point.Column = a.Point.Column + b.Point.Column
	}
	return result
}

// LengthSub returns a - b. Caller must ensure a >= b.
func LengthSub(a, b Length) Length {
	result := Length{Bytes: a.Bytes - b.Bytes}
	if a.Point.Row == b.Point.Row {
		result.Point.Column = a.Point.Column - b.Point.Column
	} else {
		result.Point.Row = a.Point.Row - b.Point.Row
		result.Point.Column = a.Point.Column
	}
	return result
}

// InputEdit describes an edit to a source document.
type InputEdit struct {
	StartByte    uint32
	OldEndByte   uint32
	NewEndByte   uint32
	StartPoint   Point
	OldEndPoint  Point
	NewEndPoint  Point
}

// SymbolMetadata holds visibility and naming information for a grammar symbol.
type SymbolMetadata struct {
	Visible bool
	Named   bool
	// Supertype is true if this symbol is a supertype node (ABI v15).
	Supertype bool
}

// LexMode describes the lex state for a given parse state.
type LexMode struct {
	LexState         uint16
	ExternalLexState uint16
}

// FieldMapSlice identifies a range within the field map entries array.
type FieldMapSlice struct {
	Index  uint16
	Length uint16
}

// FieldMapEntry maps a field ID to a child index within a production.
type FieldMapEntry struct {
	FieldID    FieldID
	ChildIndex uint16
	Inherited  bool
}

// ParseActionType distinguishes the kind of parse action.
type ParseActionType uint8

const (
	ParseActionTypeHeader  ParseActionType = 0
	ParseActionTypeShift   ParseActionType = 1
	ParseActionTypeReduce  ParseActionType = 2
	ParseActionTypeAccept  ParseActionType = 3
	ParseActionTypeRecover ParseActionType = 4
)

// ParseActionEntry is a single entry in the parse actions table.
// In C, this is a union of a header entry and an action entry.
// We use a flat struct for clarity; the Type field discriminates usage.
type ParseActionEntry struct {
	// Type discriminates how this entry is used.
	Type ParseActionType

	// Header fields (Type == ParseActionTypeHeader)
	Count    uint8
	Reusable bool

	// Shift fields (Type == ParseActionTypeShift)
	ShiftState      StateID
	ShiftExtra      bool
	ShiftRepetition bool

	// Reduce fields (Type == ParseActionTypeReduce)
	ReduceSymbol     Symbol
	ReduceChildCount uint8
	ReduceDynPrec    int16
	ReduceProdID     uint16
}

// TableEntry is the result of a parse table lookup for a (state, symbol) pair.
// It contains a pointer into the actions slice plus the header metadata.
type TableEntry struct {
	Actions    []ParseActionEntry
	ActionCount uint8
	Reusable   bool
}
