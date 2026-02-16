package treesitter

// reusable_node.go implements the ReusableNode iterator for incremental parsing.
// It walks the old tree in document order (DFS), providing candidate subtrees
// that the parser can reuse instead of re-lexing. This is the Go equivalent
// of C reusable_node.h and parser.c:ts_parser__reuse_node.

// reusableNodeEntry is a stack entry for DFS traversal of the old tree.
type reusableNodeEntry struct {
	tree       Subtree
	childIndex uint32
	byteOffset uint32
}

// ReusableNode walks an old tree in document order, providing subtree
// candidates for reuse during incremental parsing.
type ReusableNode struct {
	stack             []reusableNodeEntry
	lastExternalToken Subtree
	arena             *SubtreeArena
}

// NewReusableNode creates a ReusableNode rooted at the given subtree.
func NewReusableNode(root Subtree, arena *SubtreeArena) *ReusableNode {
	rn := &ReusableNode{arena: arena}
	if !root.IsZero() {
		rn.stack = append(rn.stack, reusableNodeEntry{
			tree:       root,
			childIndex: 0,
			byteOffset: 0,
		})
	}
	return rn
}

// Reset re-initializes the ReusableNode with a new root.
func (rn *ReusableNode) Reset(root Subtree) {
	rn.stack = rn.stack[:0]
	rn.lastExternalToken = SubtreeZero
	if !root.IsZero() {
		rn.stack = append(rn.stack, reusableNodeEntry{
			tree:       root,
			childIndex: 0,
			byteOffset: 0,
		})
	}
}

// Tree returns the current subtree at the top of the stack, or SubtreeZero
// if the iterator is exhausted.
func (rn *ReusableNode) Tree() Subtree {
	if len(rn.stack) == 0 {
		return SubtreeZero
	}
	return rn.stack[len(rn.stack)-1].tree
}

// ByteOffset returns the byte offset of the current subtree.
func (rn *ReusableNode) ByteOffset() uint32 {
	if len(rn.stack) == 0 {
		return 0
	}
	return rn.stack[len(rn.stack)-1].byteOffset
}

// Done returns true if the iterator has no more nodes to visit.
func (rn *ReusableNode) Done() bool {
	return len(rn.stack) == 0
}

// Advance moves past the current subtree (skip it without descending).
// Used when the current subtree is reused or is past the parse position.
func (rn *ReusableNode) Advance() {
	if len(rn.stack) == 0 {
		return
	}

	top := rn.stack[len(rn.stack)-1]
	tree := top.tree
	byteOffset := top.byteOffset

	// Track external tokens.
	if HasExternalTokens(tree, rn.arena) {
		rn.lastExternalToken = tree
	}

	// Advance past this subtree's span.
	size := GetPadding(tree, rn.arena)
	totalBytes := size.Bytes + GetSize(tree, rn.arena).Bytes

	rn.stack = rn.stack[:len(rn.stack)-1]

	// Move to next sibling or ascend.
	rn.advancePastByte(byteOffset + totalBytes)
}

// AdvanceToByteOffset advances the iterator until ByteOffset() >= target.
// Descends into children of nodes that start before target.
func (rn *ReusableNode) AdvanceToByteOffset(target uint32) {
	for len(rn.stack) > 0 {
		top := rn.stack[len(rn.stack)-1]
		tree := top.tree
		byteOffset := top.byteOffset
		totalBytes := GetPadding(tree, rn.arena).Bytes + GetSize(tree, rn.arena).Bytes
		endOffset := byteOffset + totalBytes

		if byteOffset >= target {
			// Current node starts at or after target. Stop.
			return
		}

		if endOffset <= target {
			// Current node ends before target. Advance past it.
			if HasExternalTokens(tree, rn.arena) {
				rn.lastExternalToken = tree
			}
			rn.stack = rn.stack[:len(rn.stack)-1]
			rn.advancePastByte(endOffset)
			continue
		}

		// Node spans the target — descend into children.
		rn.Descend()
	}
}

// Descend descends into the children of the current node.
// If the current node is a leaf, it is equivalent to Advance.
func (rn *ReusableNode) Descend() {
	if len(rn.stack) == 0 {
		return
	}

	top := &rn.stack[len(rn.stack)-1]
	tree := top.tree
	children := GetChildren(tree, rn.arena)

	if len(children) == 0 {
		// Leaf node — advance past it.
		rn.Advance()
		return
	}

	// SummarizeChildren sets the node's padding = first child's padding.
	// So the first child starts at the same byteOffset as the parent.
	// Subsequent children start after the previous child ends.

	// Remove the current node from stack.
	parentByteOffset := top.byteOffset
	rn.stack = rn.stack[:len(rn.stack)-1]

	// Push children in reverse order so the first child is on top.
	childOffset := parentByteOffset
	for i := 0; i < len(children); i++ {
		child := children[i]
		childPadding := GetPadding(child, rn.arena)
		childSize := GetSize(child, rn.arena)

		rn.stack = append(rn.stack, reusableNodeEntry{
			tree:       child,
			childIndex: uint32(i),
			byteOffset: childOffset,
		})

		childOffset += childPadding.Bytes + childSize.Bytes
	}

	// We pushed children in forward order, but we want the first child
	// on top of the stack. Reverse the just-pushed entries.
	start := len(rn.stack) - len(children)
	for i, j := start, len(rn.stack)-1; i < j; i, j = i+1, j-1 {
		rn.stack[i], rn.stack[j] = rn.stack[j], rn.stack[i]
	}
}

// advancePastByte is a helper that advances sibling iteration in the
// parent frame until we reach a node that starts at or after the given byte.
// This is called after consuming or skipping a node.
func (rn *ReusableNode) advancePastByte(targetByte uint32) {
	// After popping a node, the next element on the stack (if any) is
	// the next sibling or an ancestor. No additional work needed since
	// Descend pushes siblings in order.
}

// LastExternalToken returns the last external token encountered during traversal.
func (rn *ReusableNode) LastExternalToken() Subtree {
	return rn.lastExternalToken
}
