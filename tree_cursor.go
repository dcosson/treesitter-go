package treesitter

// TreeCursorEntry represents a position in the cursor's traversal stack.
type TreeCursorEntry struct {
	subtree              Subtree
	position             Length  // byte offset + point where this subtree starts (including padding)
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
func (c *TreeCursor) CurrentNode() Node {
	if len(c.stack) == 0 {
		return Node{}
	}
	entry := &c.stack[len(c.stack)-1]
	return c.tree.nodeFromSubtree(entry.subtree, entry.position, 0)
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

	// Find the first visible child, descending through hidden nodes.
	padding := GetPadding(entry.subtree, arena)
	childBasePos := LengthAdd(entry.position, padding)

	stackBefore := len(c.stack)
	if c.findFirstVisibleChild(children, childBasePos, arena) {
		return true
	}
	// Restore stack if we didn't find anything.
	c.stack = c.stack[:stackBefore]
	return false
}

// findFirstVisibleChild searches through children for the first visible node,
// recursively descending into hidden nodes to handle arbitrary nesting depth.
// Pushes intermediate hidden nodes onto the cursor stack so GotoParent works.
// Returns true if a visible child was found.
func (c *TreeCursor) findFirstVisibleChild(children []Subtree, basePos Length, arena *SubtreeArena) bool {
	pos := basePos
	for i, child := range children {
		if IsVisible(child, arena) {
			c.stack = append(c.stack, TreeCursorEntry{
				subtree:    child,
				position:   pos,
				childIndex: uint32(i),
			})
			return true
		}
		// Hidden node: push it and recurse into its children.
		grandchildren := GetChildren(child, arena)
		if len(grandchildren) > 0 {
			c.stack = append(c.stack, TreeCursorEntry{
				subtree:    child,
				position:   pos,
				childIndex: uint32(i),
			})
			hiddenPadding := GetPadding(child, arena)
			gcBasePos := LengthAdd(pos, hiddenPadding)
			if c.findFirstVisibleChild(grandchildren, gcBasePos, arena) {
				return true
			}
			// Not found in this hidden node's descendants — pop it.
			c.stack = c.stack[:len(c.stack)-1]
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

		// Look for the next visible sibling among remaining children.
		remaining := parentChildren[nextIdx:]
		stackBefore := len(c.stack) - 1 // Will replace top entry
		c.stack = c.stack[:stackBefore]

		if c.findFirstVisibleChild(remaining, pos, arena) {
			// Adjust child indices: findFirstVisibleChild uses 0-based indices
			// within the remaining slice, but we need indices into parentChildren.
			for k := stackBefore; k < len(c.stack); k++ {
				if k == stackBefore {
					// First pushed entry is relative to remaining slice.
					c.stack[k].childIndex += uint32(nextIdx)
				}
			}
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
