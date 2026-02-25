package parser

import ts "github.com/treesitter-go/treesitter"

func editSubtree(s Subtree, edit *InputEdit, arena *SubtreeArena) Subtree {
	return ts.EditSubtree(s, edit, arena)
}

func saturatingSub(a, b uint32) uint32 {
	if a >= b {
		return a - b
	}
	return 0
}

func makeSubtreeTestLanguage() *Language {
	return &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false},
			{Visible: true, Named: false},
			{Visible: true, Named: false},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: false},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: false, Named: false},
			{Visible: true, Named: false},
			{Visible: true, Named: true},
		},
		SymbolNames: []string{
			"end", "{", "}", "object", "pair", "string",
			":", "number", "document", "_value", ",", "comment",
		},
		FieldNames: []string{
			"",
			"key",
			"value",
		},
		FieldMapSlices: []ts.FieldMapSlice{
			{},
			{Index: 0, Length: 2},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0},
			{FieldID: 2, ChildIndex: 2},
		},
	}
}

func buildTestTree() (*Tree, *SubtreeArena) {
	arena := NewSubtreeArena(64)
	lang := makeSubtreeTestLanguage()

	lbrace := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)

	strKey := NewLeafSubtree(arena, Symbol(5),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(2), false, false, false, lang)

	colon := NewLeafSubtree(arena, Symbol(6),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(3), false, false, false, lang)

	numVal := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(4), false, false, false, lang)

	rbrace := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(5), false, false, false, lang)

	pair := NewNodeSubtree(arena, Symbol(4), []Subtree{strKey, colon, numVal}, 1, lang)
	SummarizeChildren(pair, arena, lang)

	object := NewNodeSubtree(arena, Symbol(3), []Subtree{lbrace, pair, rbrace}, 0, lang)
	SummarizeChildren(object, arena, lang)

	document := NewNodeSubtree(arena, Symbol(8), []Subtree{object}, 0, lang)
	SummarizeChildren(document, arena, lang)

	tree := NewTree(document, lang, nil, []*SubtreeArena{arena})
	return tree, arena
}
