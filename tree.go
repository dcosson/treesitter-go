package treesitter

import "strings"

// Tree represents a complete parse tree. It holds the root subtree, the
// language used for parsing, included ranges, and references to all arenas
// that contain subtrees reachable from this tree.
//
// Trees are immutable after creation. For incremental parsing, Tree.Edit()
// creates a new tree with structural sharing.
type Tree struct {
	root           Subtree
	language       *Language
	includedRanges []Range
	arenas         []*SubtreeArena
}

// NewTree creates a new Tree from a root subtree and associated metadata.
func NewTree(root Subtree, language *Language, includedRanges []Range, arenas []*SubtreeArena) *Tree {
	return &Tree{
		root:           root,
		language:       language,
		includedRanges: includedRanges,
		arenas:         arenas,
	}
}

// RootNode returns the root Node of the tree.
func (t *Tree) RootNode() Node {
	if t.root.IsZero() {
		return Node{}
	}
	return t.nodeFromSubtree(t.root, LengthZero, 0)
}

// RootSubtree returns the internal root subtree.
// Used by internal package refactors where parser logic is moved out of package treesitter.
func (t *Tree) RootSubtree() Subtree {
	if t == nil {
		return SubtreeZero
	}
	return t.root
}

// Language returns the Language this tree was parsed with.
func (t *Tree) Language() *Language {
	return t.language
}

// IncludedRanges returns the included ranges used for this parse.
func (t *Tree) IncludedRanges() []Range {
	return t.includedRanges
}

// Arena returns the primary arena (first one) for this tree.
// Used internally for subtree lookups.
func (t *Tree) Arena() *SubtreeArena {
	if len(t.arenas) == 0 {
		return nil
	}
	return t.arenas[0]
}

// Edit propagates an edit through the tree using structural sharing.
// Only nodes on the edit path are cloned; unaffected subtrees are shared.
// Returns a new Tree — the original tree is not modified.
//
// The new tree's arena is a fork of the original arena, so old subtree
// references remain valid. The returned tree can be passed to Parser.Parse
// as the old tree for incremental re-parsing.
func (t *Tree) Edit(edit *InputEdit) *Tree {
	if t.root.IsZero() {
		return t
	}

	forkedArena := t.Arena().Fork()
	newRoot := editSubtree(t.root, edit, forkedArena)

	return &Tree{
		root:           newRoot,
		language:       t.language,
		includedRanges: t.includedRanges,
		arenas:         []*SubtreeArena{forkedArena},
	}
}

// Copy returns a shallow copy of the tree. The new tree shares the same
// arena and subtree data.
func (t *Tree) Copy() *Tree {
	return &Tree{
		root:           t.root,
		language:       t.language,
		includedRanges: t.includedRanges,
		arenas:         t.arenas,
	}
}

// nodeFromSubtree creates a Node value from a subtree, its position offset,
// and an optional alias symbol.
func (t *Tree) nodeFromSubtree(s Subtree, position Length, aliasSymbol Symbol) Node {
	arena := t.Arena()
	padding := GetPadding(s, arena)
	startPos := LengthAdd(position, padding)
	return Node{
		context: [4]uint32{
			startPos.Bytes,
			startPos.Point.Row,
			startPos.Point.Column,
			uint32(aliasSymbol),
		},
		id:      SubtreeIDOf(s),
		tree:    t,
		subtree: s,
	}
}

// --- Node: lightweight value type for tree navigation ---

// Node is a lightweight value type (~56 bytes) representing a node in the parse tree.
// It contains position context so that most accessors require no pointer chasing.
// Nodes are created by Tree and TreeCursor methods.
//
// A zero-value Node is invalid (IsNull() returns true).
type Node struct {
	// context packs start position and alias info:
	//   [0] = startByte
	//   [1] = startRow
	//   [2] = startColumn
	//   [3] = aliasSymbol (0 = no alias)
	context [4]uint32

	// id uniquely identifies this subtree for identity comparison.
	id SubtreeID

	// tree is a reference to the owning Tree.
	tree *Tree

	// subtree is the underlying Subtree value.
	subtree Subtree
}

// IsNull returns true if this is a zero/invalid Node.
func (n Node) IsNull() bool {
	return n.tree == nil
}

// Type returns the grammar type name of this node (e.g., "identifier", "function_definition").
// If the node has an alias, the alias name is returned.
func (n Node) Type() string {
	if n.IsNull() {
		return ""
	}
	sym := n.Symbol()
	return n.tree.language.SymbolName(sym)
}

// Symbol returns the grammar symbol of this node.
// If the node has an alias, the alias symbol is returned.
// The symbol is mapped through PublicSymbolMap to normalize internal variants.
func (n Node) Symbol() Symbol {
	if n.IsNull() {
		return 0
	}
	var sym Symbol
	if n.context[3] != 0 {
		sym = Symbol(n.context[3])
	} else {
		sym = GetSymbol(n.subtree, n.tree.Arena())
	}
	return n.tree.language.PublicSymbol(sym)
}

// StartByte returns the byte offset where this node starts.
func (n Node) StartByte() uint32 {
	return n.context[0]
}

// EndByte returns the byte offset where this node ends.
func (n Node) EndByte() uint32 {
	if n.IsNull() {
		return 0
	}
	arena := n.tree.Arena()
	size := GetSize(n.subtree, arena)
	return n.context[0] + size.Bytes
}

// StartPoint returns the (row, column) position where this node starts.
func (n Node) StartPoint() Point {
	return Point{
		Row:    n.context[1],
		Column: n.context[2],
	}
}

// EndPoint returns the (row, column) position where this node ends.
func (n Node) EndPoint() Point {
	if n.IsNull() {
		return Point{}
	}
	arena := n.tree.Arena()
	size := GetSize(n.subtree, arena)
	startPoint := Point{Row: n.context[1], Column: n.context[2]}
	endLength := LengthAdd(Length{Bytes: 0, Point: startPoint}, size)
	return endLength.Point
}

// ChildCount returns the number of children (including anonymous children).
func (n Node) ChildCount() uint32 {
	if n.IsNull() {
		return 0
	}
	arena := n.tree.Arena()
	return getVisibleChildCountForNode(n.subtree, arena)
}

// NamedChildCount returns the number of named children.
func (n Node) NamedChildCount() uint32 {
	if n.IsNull() {
		return 0
	}
	arena := n.tree.Arena()
	if n.subtree.IsInline() {
		return 0
	}
	return arena.Get(n.subtree).NamedChildCount
}

// Child returns the child at the given index (0-based, including anonymous children).
// Returns a null Node if the index is out of range.
// Handles arbitrary nesting depth of hidden nodes.
func (n Node) Child(index int) Node {
	if n.IsNull() || index < 0 {
		return Node{}
	}
	arena := n.tree.Arena()
	children := GetChildren(n.subtree, arena)
	if children == nil {
		return Node{}
	}

	childPos := Length{Bytes: n.context[0], Point: Point{Row: n.context[1], Column: n.context[2]}}
	visibleIndex := 0
	result, _ := findVisibleChildByIndex(n.tree, children, childPos, n.subtree, arena, index, &visibleIndex)
	return result
}

// findVisibleChildByIndex recursively walks children (descending through hidden
// nodes of arbitrary depth) to find the visible child at the given index.
// parentSubtree is the immediate parent whose children we're iterating (may be hidden).
// structuralIdx tracks the structural child index (non-extra children) within
// the parent, used for alias resolution.
func findVisibleChildByIndex(tree *Tree, children []Subtree, basePos Length, parentSubtree Subtree, arena *SubtreeArena, targetIndex int, currentIndex *int) (Node, Length) {
	pos := basePos
	structuralIdx := 0
	lang := tree.language
	for _, child := range children {
		isExtra := IsExtra(child, arena)
		if IsVisibleInContext(child, arena, parentSubtree, structuralIdx, lang) {
			if *currentIndex == targetIndex {
				return tree.nodeFromChildSubtree(child, pos, parentSubtree, structuralIdx, arena), pos
			}
			*currentIndex++
			if !isExtra {
				structuralIdx++
			}
			pos = advancePosition(pos, child, arena)
		} else {
			// Hidden node: recurse into its children.
			grandchildren := GetChildren(child, arena)
			if len(grandchildren) > 0 {
				hiddenPadding := GetPadding(child, arena)
				gcPos := LengthAdd(pos, hiddenPadding)
				result, _ := findVisibleChildByIndex(tree, grandchildren, gcPos, child, arena, targetIndex, currentIndex)
				if !result.IsNull() {
					return result, pos
				}
			}
			if !isExtra {
				structuralIdx++
			}
			pos = advancePosition(pos, child, arena)
		}
	}
	return Node{}, pos
}

// NamedChild returns the named child at the given index (0-based).
// Returns a null Node if the index is out of range.
// Handles arbitrary nesting depth of hidden nodes.
func (n Node) NamedChild(index int) Node {
	if n.IsNull() || index < 0 {
		return Node{}
	}
	arena := n.tree.Arena()
	children := GetChildren(n.subtree, arena)
	if children == nil {
		return Node{}
	}

	childPos := Length{Bytes: n.context[0], Point: Point{Row: n.context[1], Column: n.context[2]}}
	namedIndex := 0
	result, _ := findNamedChildByIndex(n.tree, children, childPos, n.subtree, arena, index, &namedIndex)
	return result
}

// findNamedChildByIndex recursively walks children (descending through hidden
// nodes of arbitrary depth) to find the named child at the given index.
// parentSubtree and structural indexing work the same as findVisibleChildByIndex.
func findNamedChildByIndex(tree *Tree, children []Subtree, basePos Length, parentSubtree Subtree, arena *SubtreeArena, targetIndex int, currentIndex *int) (Node, Length) {
	pos := basePos
	structuralIdx := 0
	lang := tree.language
	for _, child := range children {
		isExtra := IsExtra(child, arena)
		if IsVisibleInContext(child, arena, parentSubtree, structuralIdx, lang) {
			if IsNamedInContext(child, arena, parentSubtree, structuralIdx, lang) && !isExtra {
				if *currentIndex == targetIndex {
					return tree.nodeFromChildSubtree(child, pos, parentSubtree, structuralIdx, arena), pos
				}
				*currentIndex++
			}
			if !isExtra {
				structuralIdx++
			}
			pos = advancePosition(pos, child, arena)
		} else {
			grandchildren := GetChildren(child, arena)
			if len(grandchildren) > 0 {
				hiddenPadding := GetPadding(child, arena)
				gcPos := LengthAdd(pos, hiddenPadding)
				result, _ := findNamedChildByIndex(tree, grandchildren, gcPos, child, arena, targetIndex, currentIndex)
				if !result.IsNull() {
					return result, pos
				}
			}
			if !isExtra {
				structuralIdx++
			}
			pos = advancePosition(pos, child, arena)
		}
	}
	return Node{}, pos
}

// ChildByFieldName returns the first child associated with the given field name.
// Returns a null Node if no child has that field.
func (n Node) ChildByFieldName(name string) Node {
	if n.IsNull() {
		return Node{}
	}
	lang := n.tree.language
	// Find the field ID for this name.
	fieldID := FieldID(0)
	for i, fn := range lang.FieldNames {
		if fn == name {
			fieldID = FieldID(i)
			break
		}
	}
	if fieldID == 0 {
		return Node{}
	}
	return n.ChildByFieldID(fieldID)
}

// ChildByFieldID returns the first child associated with the given field ID.
// Returns a null Node if no child has that field.
func (n Node) ChildByFieldID(fieldID FieldID) Node {
	if n.IsNull() || fieldID == 0 {
		return Node{}
	}
	arena := n.tree.Arena()
	lang := n.tree.language
	prodID := GetProductionID(n.subtree, arena)
	fieldEntries := lang.FieldMapForProduction(prodID)
	if fieldEntries == nil {
		return Node{}
	}

	// Find which child indices map to this field.
	for _, entry := range fieldEntries {
		if entry.FieldID == fieldID {
			child := n.Child(int(entry.ChildIndex))
			if !child.IsNull() {
				return child
			}
		}
	}
	return Node{}
}

// Parent returns the parent node. This requires a tree walk from the root,
// so it is O(depth). Returns a null Node if this is the root.
func (n Node) Parent() Node {
	if n.IsNull() {
		return Node{}
	}
	// Walk from root to find parent using TreeCursor.
	cursor := NewTreeCursor(n.tree.RootNode())
	return cursor.findParentOf(n)
}

// NextSibling returns the next sibling node (including anonymous nodes).
// Returns a null Node if this is the last child.
func (n Node) NextSibling() Node {
	if n.IsNull() {
		return Node{}
	}
	parent := n.Parent()
	if parent.IsNull() {
		return Node{}
	}
	found := false
	count := int(parent.ChildCount())
	for i := 0; i < count; i++ {
		child := parent.Child(i)
		if found {
			return child
		}
		if child.id.Equal(n.id) {
			found = true
		}
	}
	return Node{}
}

// PrevSibling returns the previous sibling node (including anonymous nodes).
// Returns a null Node if this is the first child.
func (n Node) PrevSibling() Node {
	if n.IsNull() {
		return Node{}
	}
	parent := n.Parent()
	if parent.IsNull() {
		return Node{}
	}
	var prev Node
	count := int(parent.ChildCount())
	for i := 0; i < count; i++ {
		child := parent.Child(i)
		if child.id.Equal(n.id) {
			return prev
		}
		prev = child
	}
	return Node{}
}

// IsNamed returns true if this is a named node.
func (n Node) IsNamed() bool {
	if n.IsNull() {
		return false
	}
	if n.context[3] != 0 {
		// Has alias — check alias symbol metadata.
		return n.tree.language.SymbolIsNamed(Symbol(n.context[3]))
	}
	return IsNamed(n.subtree, n.tree.Arena())
}

// IsExtra returns true if this is an extra node (e.g., comment).
func (n Node) IsExtra() bool {
	if n.IsNull() {
		return false
	}
	return IsExtra(n.subtree, n.tree.Arena())
}

// IsMissing returns true if this node was inserted by error recovery.
func (n Node) IsMissing() bool {
	if n.IsNull() {
		return false
	}
	return IsMissing(n.subtree, n.tree.Arena())
}

// HasChanges returns true if this node has been edited.
func (n Node) HasChanges() bool {
	if n.IsNull() {
		return false
	}
	return HasChanges(n.subtree, n.tree.Arena())
}

// Equal returns true if two nodes refer to the same tree node.
func (n Node) Equal(other Node) bool {
	return n.id.Equal(other.id) && n.tree == other.tree
}

// String returns the S-expression representation of this node.
func (n Node) String() string {
	if n.IsNull() {
		return ""
	}
	var buf strings.Builder
	n.writeSExpr(&buf)
	s := buf.String()
	// The recursive writer prepends a space before each visible named node;
	// trim the leading space from the root.
	if len(s) > 0 && s[0] == ' ' {
		return s[1:]
	}
	return s
}

// writeSExpr writes the S-expression for this node to the builder.
func (n Node) writeSExpr(buf *strings.Builder) {
	writeSExprSubtree(n.subtree, n.tree.Arena(), n.tree.language, buf, 0, "")
}

// writeSExprSubtree writes the S-expression for a subtree, walking raw children
// to correctly track structural child indices for field annotation lookup.
// aliasSymbol overrides the subtree's symbol when non-zero (for alias resolution).
// fieldName is the field annotation from the parent, propagated through hidden nodes.
func writeSExprSubtree(s Subtree, arena *SubtreeArena, lang *Language, buf *strings.Builder, aliasSymbol Symbol, fieldName string) {
	// Determine symbol and named status, considering aliases.
	var sym Symbol
	var isNamed bool
	if aliasSymbol != 0 {
		sym = aliasSymbol
		isNamed = lang.SymbolIsNamed(aliasSymbol)
	} else {
		sym = lang.PublicSymbol(GetSymbol(s, arena))
		isNamed = IsNamed(s, arena)
	}

	isVisible := IsVisible(s, arena) || aliasSymbol != 0
	isMissing := IsMissing(s, arena)

	if isVisible && (isNamed || isMissing) {
		buf.WriteByte(' ')
		if fieldName != "" {
			buf.WriteString(fieldName)
			buf.WriteString(": ")
		}
		if isMissing {
			// Matches C subtree.c: (MISSING symbolname) for named, (MISSING "symbolname") for anonymous.
			buf.WriteString("(MISSING ")
			aliasIsNamed := aliasSymbol != 0 && lang.SymbolIsNamed(aliasSymbol)
			if aliasIsNamed || IsNamed(s, arena) {
				buf.WriteString(lang.SymbolName(sym))
			} else {
				buf.WriteByte('"')
				buf.WriteString(lang.SymbolName(sym))
				buf.WriteByte('"')
			}
		} else {
			buf.WriteByte('(')
			buf.WriteString(lang.SymbolName(sym))
		}
	}

	// Iterate raw children to track structural child indices for field lookup.
	children := GetChildren(s, arena)
	if len(children) > 0 {
		prodID := GetProductionID(s, arena)
		fieldEntries := lang.FieldMapForProduction(prodID)
		structuralIdx := 0

		for _, child := range children {
			childExtra := IsExtra(child, arena)

			if childExtra {
				// Extra nodes (e.g., comments) don't have field annotations
				// and don't increment the structural index.
				if IsVisible(child, arena) && IsNamed(child, arena) {
					writeSExprSubtree(child, arena, lang, buf, 0, "")
				}
				continue
			}

			// Determine the field name for this child:
			// - If current node is visible: start fresh (no propagation)
			// - If current node is hidden: propagate incoming fieldName
			// Then check for non-inherited field map entries to override.
			var childFieldName string
			if !isVisible {
				childFieldName = fieldName
			}
			childFieldName = nonInheritedFieldForChild(fieldEntries, lang, uint16(structuralIdx), childFieldName)

			// Look up alias for this child.
			childAlias := lang.AliasForProduction(prodID, structuralIdx)
			structuralIdx++

			// Determine child visibility and named status considering aliases.
			var childIsVisible, childIsNamed bool
			if childAlias != 0 {
				childIsVisible = lang.SymbolIsVisible(childAlias)
				childIsNamed = lang.SymbolIsNamed(childAlias)
			} else {
				childIsVisible = IsVisible(child, arena)
				childIsNamed = IsNamed(child, arena)
			}

			if childIsVisible {
				if childIsNamed {
					writeSExprSubtree(child, arena, lang, buf, childAlias, childFieldName)
				} else {
					writeSExprSubtree(child, arena, lang, buf, childAlias, "")
				}
			} else {
				// Hidden child: recurse to emit its visible descendants.
				// Propagate the field name through hidden nodes.
				writeSExprSubtree(child, arena, lang, buf, 0, childFieldName)
			}
		}
	}

	if isVisible && (isNamed || isMissing) {
		buf.WriteByte(')')
	}
}

// nonInheritedFieldForChild checks the field map for a non-inherited entry
// at the given structural child index. If found, returns that field name.
// Otherwise returns the fallback (which may be a propagated field from parent).
func nonInheritedFieldForChild(entries []FieldMapEntry, lang *Language, childIndex uint16, fallback string) string {
	for _, entry := range entries {
		if !entry.Inherited && entry.ChildIndex == childIndex {
			return lang.FieldName(entry.FieldID)
		}
	}
	return fallback
}

// --- Internal helpers ---

// getVisibleChildCountForNode returns the count of visible children for
// tree navigation (accounts for hidden nodes whose children bubble up).
func getVisibleChildCountForNode(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		return 0
	}
	data := arena.Get(s)
	if data.ChildCount == 0 {
		return 0
	}
	// If this node is visible, its visible child count was computed by SummarizeChildren.
	return data.VisibleChildCount
}

// advancePosition moves position past a subtree (its padding + size).
func advancePosition(pos Length, s Subtree, arena *SubtreeArena) Length {
	padding := GetPadding(s, arena)
	size := GetSize(s, arena)
	return LengthAdd(LengthAdd(pos, padding), size)
}

// nodeFromChildSubtree creates a Node for a child subtree, resolving aliases.
// The structuralChildIndex is the index of this child among non-extra siblings
// in the parent's children, used to look up alias sequences.
func (t *Tree) nodeFromChildSubtree(child Subtree, position Length, parent Subtree, structuralChildIndex int, arena *SubtreeArena) Node {
	padding := GetPadding(child, arena)
	startPos := LengthAdd(position, padding)

	// Look up alias for this child based on the parent's production.
	// Only heap-allocated parents have a ProductionID; extras are never aliased.
	var aliasSymbol Symbol
	if !parent.IsInline() && !IsExtra(child, arena) {
		parentData := arena.Get(parent)
		if parentData.ProductionID > 0 {
			aliasSymbol = t.language.AliasForProduction(parentData.ProductionID, structuralChildIndex)
		}
	}

	return Node{
		context: [4]uint32{
			startPos.Bytes,
			startPos.Point.Row,
			startPos.Point.Column,
			uint32(aliasSymbol),
		},
		id:      SubtreeIDOf(child),
		tree:    t,
		subtree: child,
	}
}
