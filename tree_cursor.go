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

	// Find the first visible child, recursively descending into hidden nodes.
	padding := GetPadding(entry.subtree, arena)
	childBasePos := LengthAdd(entry.position, padding)

	return c.findFirstVisibleChild(children, childBasePos, arena)
}

// findFirstVisibleChild searches through children for the first visible node,
// recursively descending into hidden nodes. Pushes all intermediate hidden
// nodes onto the stack.
func (c *TreeCursor) findFirstVisibleChild(children []Subtree, startPos Length, arena *SubtreeArena) bool {
	pos := startPos
	var structuralIdx uint32
	for i, child := range children {
		if IsVisible(child, arena) {
			c.stack = append(c.stack, TreeCursorEntry{
				subtree:              child,
				position:             pos,
				childIndex:           uint32(i),
				structuralChildIndex: structuralIdx,
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
				structuralChildIndex: structuralIdx,
			})
			gcStartPos := pos // hidden node's children start at the same position
			if c.findFirstVisibleChild(grandchildren, gcStartPos, arena) {
				return true
			}
			// No visible child found in this hidden subtree; pop it.
			c.stack = c.stack[:len(c.stack)-1]
		}
		if !IsExtra(child, arena) {
			structuralIdx++
		}
		pos = advancePosition(pos, child, arena)
	}
	return false
}

// GotoNextSibling moves the cursor to the next visible sibling.
// Returns true if a sibling was found, false if this is the last child.
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
		structuralIdx := current.structuralChildIndex
		if !IsExtra(current.subtree, arena) {
			structuralIdx++
		}

		// Look for the next visible sibling, recursively descending into hidden nodes.
		for i := nextIdx; i < len(parentChildren); i++ {
			child := parentChildren[i]
			if IsVisible(child, arena) {
				c.stack[len(c.stack)-1] = TreeCursorEntry{
					subtree:              child,
					position:             pos,
					childIndex:           uint32(i),
					structuralChildIndex: structuralIdx,
				}
				return true
			}
			// Hidden node: push it and recursively search for visible children.
			grandchildren := GetChildren(child, arena)
			if len(grandchildren) > 0 {
				c.stack[len(c.stack)-1] = TreeCursorEntry{
					subtree:              child,
					position:             pos,
					childIndex:           uint32(i),
					structuralChildIndex: structuralIdx,
				}
				if c.findFirstVisibleChild(grandchildren, pos, arena) {
					return true
				}
				// No visible child found; restore stack entry for continued search.
			}
			if !IsExtra(child, arena) {
				structuralIdx++
			}
			pos = advancePosition(pos, child, arena)
		}

		// No more siblings at this level. If the parent is hidden, pop up and continue.
		if len(c.stack) >= 3 && !IsVisible(parentEntry.subtree, arena) {
			c.stack = c.stack[:len(c.stack)-1] // pop current
			continue
		}
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
