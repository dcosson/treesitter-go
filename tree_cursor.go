package treesitter

// TreeCursorEntry represents a position in the cursor's traversal stack.
type TreeCursorEntry struct {
	subtree              Subtree
	position             Length  // pre-padding position: content start = position + padding
	childIndex           uint32  // index in parent's children slice
	structuralChildIndex uint32  // index counting only structural (non-extra) children
}

// TreeCursor provides efficient stack-based traversal of a parse tree.
// Unlike Node navigation methods (which walk from root), TreeCursor maintains
// a stack of ancestors for O(1) parent, first-child, and next-sibling operations.
type TreeCursor struct {
	tree  *Tree
	stack []TreeCursorEntry
}

// NewTreeCursor creates a new TreeCursor starting at the given node.
func NewTreeCursor(node Node) TreeCursor {
	stack := make([]TreeCursorEntry, 1, 32)
	stack[0] = TreeCursorEntry{
		subtree:  node.subtree,
		position: Length{Bytes: node.context[0] - GetPadding(node.subtree, node.tree.Arena()).Bytes},
	}
	// Recompute position to be the start of the subtree including padding.
	// The node's context[0] is the start byte (after padding), so we go back.
	padding := GetPadding(node.subtree, node.tree.Arena())
	startPos := LengthSub(
		Length{
			Bytes: node.context[0],
			Point: Point{Row: node.context[1], Column: node.context[2]},
		},
		padding,
	)
	stack[0].position = startPos

	return TreeCursor{
		tree:  node.tree,
		stack: stack,
	}
}

// CurrentNode returns the Node at the cursor's current position.
// Resolves aliases by looking up the parent's production and the current
// node's structural child index.
func (c *TreeCursor) CurrentNode() Node {
	if len(c.stack) == 0 {
		return Node{}
	}
	entry := &c.stack[len(c.stack)-1]

	// Resolve alias: if we have a parent and the current node is not extra,
	// check if the parent's production aliases this child.
	var aliasSymbol Symbol
	if len(c.stack) > 1 {
		arena := c.tree.Arena()
		if !IsExtra(entry.subtree, arena) {
			parentEntry := &c.stack[len(c.stack)-2]
			if !parentEntry.subtree.IsInline() {
				parentData := arena.Get(parentEntry.subtree)
				if parentData.ProductionID > 0 {
					aliasSymbol = c.tree.language.AliasForProduction(
						parentData.ProductionID,
						int(entry.structuralChildIndex),
					)
				}
			}
		}
	}

	return c.tree.nodeFromSubtree(entry.subtree, entry.position, aliasSymbol)
}

// GotoFirstChild moves the cursor to the first visible child of the current node.
// Returns true if a child was found, false if the current node has no children.
// Handles arbitrary nesting depth of hidden nodes — descends through multiple
// layers of hidden wrapping until a visible node is found.
func (c *TreeCursor) GotoFirstChild() bool {
	if len(c.stack) == 0 {
		return false
	}
	entry := &c.stack[len(c.stack)-1]
	arena := c.tree.Arena()

	children := GetChildren(entry.subtree, arena)
	if len(children) == 0 {
		return false
	}

	// Start child iteration at the parent's position. entry.position is the
	// pre-padding position of the parent. Since SummarizeChildren sets
	// parent.padding = first_child.padding, the parent's pre-padding position
	// equals the first child's pre-padding position. advancePosition advances
	// by child.padding + child.size, so pos always tracks the pre-padding
	// position of each successive child. This matches C tree-sitter's
	// ts_tree_cursor_iterate_children which starts at entry->position directly.
	stackBefore := len(c.stack)
	if c.findFirstVisibleChild(children, entry.position, arena, 0) {
		return true
	}
	// Restore stack if we didn't find anything.
	c.stack = c.stack[:stackBefore]
	return false
}

// findFirstVisibleChild searches through children for the first visible node,
// recursively descending into hidden nodes to handle arbitrary nesting depth.
// Pushes intermediate hidden nodes onto the cursor stack so GotoParent works.
// structuralOffset is the structural child index accumulated from prior siblings
// at this level (used when called from GotoNextSibling to continue counting).
// Returns true if a visible child was found.
func (c *TreeCursor) findFirstVisibleChild(children []Subtree, basePos Length, arena *SubtreeArena, structuralOffset int) bool {
	pos := basePos
	structuralIdx := structuralOffset
	for i, child := range children {
		isExtra := IsExtra(child, arena)
		if IsVisible(child, arena) {
			c.stack = append(c.stack, TreeCursorEntry{
				subtree:              child,
				position:             pos,
				childIndex:           uint32(i),
				structuralChildIndex: uint32(structuralIdx),
			})
			return true
		}
		// Hidden node: push it and recurse into its children.
		grandchildren := GetChildren(child, arena)
		if len(grandchildren) > 0 {
			c.stack = append(c.stack, TreeCursorEntry{
				subtree:              child,
				position:             pos,
				childIndex:           uint32(i),
				structuralChildIndex: uint32(structuralIdx),
			})
			// Pass pos directly — hidden child's pre-padding position equals
			// its first grandchild's pre-padding position (same invariant as
			// GotoFirstChild: parent.padding = first_child.padding).
			if c.findFirstVisibleChild(grandchildren, pos, arena, 0) {
				return true
			}
			// Not found in this hidden node's descendants — pop it.
			c.stack = c.stack[:len(c.stack)-1]
		}
		if !isExtra {
			structuralIdx++
		}
		pos = advancePosition(pos, child, arena)
	}
	return false
}

// GotoNextSibling moves the cursor to the next visible sibling.
// Returns true if a sibling was found, false if this is the last child.
// Handles arbitrary nesting depth of hidden nodes.
func (c *TreeCursor) GotoNextSibling() bool {
	if len(c.stack) < 2 {
		return false
	}
	arena := c.tree.Arena()

	for {
		current := c.stack[len(c.stack)-1]
		parentEntry := &c.stack[len(c.stack)-2]
		parentChildren := GetChildren(parentEntry.subtree, arena)

		// Advance position past the current child.
		pos := advancePosition(current.position, current.subtree, arena)
		nextIdx := int(current.childIndex) + 1

		// Compute the structural offset for the remaining children.
		// The current child occupies a structural slot (unless it's extra),
		// so the next structural index continues from there.
		nextStructuralIdx := int(current.structuralChildIndex)
		if !IsExtra(current.subtree, arena) {
			nextStructuralIdx++
		}

		// Look for the next visible sibling among remaining children.
		remaining := parentChildren[nextIdx:]
		stackBefore := len(c.stack) - 1 // Will replace top entry
		c.stack = c.stack[:stackBefore]

		if c.findFirstVisibleChild(remaining, pos, arena, nextStructuralIdx) {
			// Adjust the first pushed entry's childIndex: findFirstVisibleChild
			// uses 0-based indices within the remaining slice, but we need
			// indices into parentChildren. Deeper entries (hidden node descendants)
			// are relative to their own parent and don't need adjustment.
			c.stack[stackBefore].childIndex += uint32(nextIdx)
			return true
		}

		// No more siblings at this level. If the parent is hidden, pop up
		// past the hidden node and continue looking in the grandparent.
		if len(c.stack) >= 2 && !IsVisible(parentEntry.subtree, arena) {
			continue
		}

		// No sibling found — restore the current entry so the cursor
		// remains positioned at the same node it was before the call.
		c.stack = append(c.stack[:stackBefore], current)
		return false
	}
}

// GotoParent moves the cursor to the parent of the current node.
// Returns true if successful, false if the cursor is at the root.
func (c *TreeCursor) GotoParent() bool {
	if len(c.stack) <= 1 {
		return false
	}
	// Pop entries until we find a visible ancestor (skip hidden nodes).
	for len(c.stack) > 1 {
		c.stack = c.stack[:len(c.stack)-1]
		entry := &c.stack[len(c.stack)-1]
		arena := c.tree.Arena()
		if IsVisible(entry.subtree, arena) || len(c.stack) == 1 {
			return true
		}
	}
	return true
}

// Reset resets the cursor to start from the given node.
func (c *TreeCursor) Reset(node Node) {
	padding := GetPadding(node.subtree, node.tree.Arena())
	startPos := LengthSub(
		Length{
			Bytes: node.context[0],
			Point: Point{Row: node.context[1], Column: node.context[2]},
		},
		padding,
	)
	c.tree = node.tree
	c.stack = c.stack[:1]
	c.stack[0] = TreeCursorEntry{
		subtree:  node.subtree,
		position: startPos,
	}
}

// findParentOf walks from root to find the parent of the target node.
// Returns a null Node if target is the root.
func (c *TreeCursor) findParentOf(target Node) Node {
	c.Reset(c.tree.RootNode())
	return c.findParentRecursive(target)
}

func (c *TreeCursor) findParentRecursive(target Node) Node {
	current := c.CurrentNode()
	if current.id.Equal(target.id) {
		return Node{}
	}

	if c.GotoFirstChild() {
		for {
			child := c.CurrentNode()
			if child.id.Equal(target.id) {
				c.GotoParent()
				return c.CurrentNode()
			}
			// Check if target could be a descendant of this child.
			if child.StartByte() <= target.StartByte() && child.EndByte() >= target.EndByte() {
				result := c.findParentRecursive(target)
				if !result.IsNull() {
					return result
				}
			}
			if !c.GotoNextSibling() {
				break
			}
		}
		c.GotoParent()
	}
	return Node{}
}
