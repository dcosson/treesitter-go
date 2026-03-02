package treesitter_test

import (
	"testing"

	ts "github.com/dcosson/treesitter-go"
)

// Sanity tests verifying the public facade re-exports work correctly.
// Full integration and e2e tests live in e2etest/.

func TestPublicTypesExist(t *testing.T) {
	// Verify core type aliases are accessible.
	var _ ts.Symbol
	var _ ts.StateID
	var _ ts.FieldID
	var _ ts.Point
	var _ ts.Length
	var _ ts.Range
	var _ ts.InputEdit
	var _ ts.SymbolMetadata
	var _ ts.ParseActionType
	var _ ts.ParseActionEntry
	var _ ts.TableEntry
	var _ ts.LexMode
	var _ ts.FieldMapSlice
	var _ ts.FieldMapEntry

	// Verify subtree types.
	var _ ts.Subtree
	var _ ts.SubtreeArena
	var _ ts.SubtreeFlags

	// Verify tree types.
	var _ ts.Tree
	var _ ts.Node
	var _ ts.TreeCursor
	var _ ts.ReusableNode

	// Verify query types.
	var _ ts.Query
	var _ ts.QueryCursor
	var _ ts.QueryMatch
	var _ ts.QueryCapture
	var _ ts.QueryError
	var _ ts.PredicateStep

	// Verify language/lexer types.
	var _ ts.Language
	var _ ts.Lexer
	var _ ts.Input
}

func TestConstructors(t *testing.T) {
	// Verify key constructors are callable.
	arena := ts.NewSubtreeArena(16)
	if arena == nil {
		t.Fatal("NewSubtreeArena returned nil")
	}

	lexer := ts.NewLexer()
	if lexer == nil {
		t.Fatal("NewLexer returned nil")
	}

	input := ts.NewStringInput([]byte("hello"))
	if input == nil {
		t.Fatal("NewStringInput returned nil")
	}
}

func TestConstants(t *testing.T) {
	// Verify sentinel symbols are exported.
	if ts.SymbolEnd != 0 {
		t.Errorf("SymbolEnd = %d, want 0", ts.SymbolEnd)
	}
	if ts.SymbolError == 0 {
		t.Error("SymbolError should not be 0")
	}

	// Verify parse action types are distinct.
	actions := []ts.ParseActionType{
		ts.ParseActionTypeHeader,
		ts.ParseActionTypeShift,
		ts.ParseActionTypeReduce,
		ts.ParseActionTypeAccept,
		ts.ParseActionTypeRecover,
	}
	seen := make(map[ts.ParseActionType]bool)
	for _, a := range actions {
		if seen[a] {
			t.Errorf("duplicate ParseActionType: %d", a)
		}
		seen[a] = true
	}
}

func TestLengthArithmetic(t *testing.T) {
	a := ts.Length{Bytes: 5, Point: ts.Point{Row: 0, Column: 5}}
	b := ts.Length{Bytes: 3, Point: ts.Point{Row: 0, Column: 3}}

	sum := ts.LengthAdd(a, b)
	if sum.Bytes != 8 || sum.Point.Column != 8 {
		t.Errorf("LengthAdd = %+v, want bytes=8, col=8", sum)
	}

	diff := ts.LengthSub(sum, b)
	if diff != a {
		t.Errorf("LengthSub = %+v, want %+v", diff, a)
	}
}
