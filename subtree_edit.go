package treesitter

// subtree_edit.go implements edit propagation for incremental parsing.
// When the user calls Tree.Edit(), the edit is propagated through the tree
// using structural sharing: only nodes on the edit path are cloned.
// This is the Go equivalent of C ts_subtree_edit (subtree.c:633-786).

// Fork creates a new SubtreeArena that shares all existing blocks from the
// parent arena. Old Subtree references remain valid in the forked arena
// because the block layout is identical. New allocations go into fresh blocks
// (or unused slots in the last shared block).
//
// The parent arena must not receive any more allocations after forking.
// This invariant holds because arenas are per-parse and trees are immutable
// after construction.
func (a *SubtreeArena) Fork() *SubtreeArena {
	forked := &SubtreeArena{
		blocks:    make([][]SubtreeHeapData, len(a.blocks), len(a.blocks)+16),
		current:   a.current,
		offset:    a.offset,
		blockSize: a.blockSize,
	}
	// Share block data arrays. The slice headers are copied but the underlying
	// arrays are shared. Since old blocks are frozen, this is safe.
	copy(forked.blocks, a.blocks)

	// Freeze the parent: force it to allocate a fresh block on next Alloc.
	// This prevents accidental writes to shared memory if someone mistakenly
	// allocates in the parent after forking.
	a.offset = a.blockSize

	return forked
}

// editSubtree propagates an InputEdit through a subtree, returning a new
// subtree with structural sharing. Only nodes on the edit path are cloned
// into the arena; unaffected siblings remain as-is (they reference the same
// shared blocks in the forked arena).
//
// The arena must be a forked arena that contains both old and new blocks.
//
// The returned subtree has:
//   - has_changes = true on all affected nodes
//   - Adjusted padding and size based on the edit
//   - depends_on_column invalidation propagated
func editSubtree(s Subtree, edit *InputEdit, arena *SubtreeArena) Subtree {
	if s.IsZero() {
		return s
	}

	// No-op edit.
	if edit.OldEndByte == edit.StartByte && edit.NewEndByte == edit.StartByte {
		return s
	}

	padding := GetPadding(s, arena)
	size := GetSize(s, arena)
	totalBytes := padding.Bytes + size.Bytes

	// Include lookahead bytes when checking if the edit affects this node.
	// The lexer may have peeked past the token boundary to determine where
	// it ends; if the edit is within that lookahead range, the token must
	// be re-parsed.
	lookahead := GetLookaheadBytes(s, arena)
	if edit.StartByte > totalBytes+lookahead {
		return s
	}

	// Clone this node into the arena (allocates a new slot, sharing children).
	newTree := cloneSubtreeInArena(s, arena)

	// Set has_changes on the clone.
	if newTree.IsInline() {
		newTree.data |= inlineHasChangesBit
	} else {
		arena.Get(newTree).SetFlag(SubtreeFlagHasChanges, true)
	}

	// Adjust padding and size on the cloned node.
	adjustNodePaddingAndSize(newTree, arena, edit)

	// For leaf nodes (no children), we're done.
	if newTree.IsInline() {
		return newTree
	}

	data := arena.Get(newTree)
	if data.ChildCount == 0 {
		return newTree
	}

	// Clone the children slice before modifying it.
	oldChildren := data.Children
	newChildren := make([]Subtree, len(oldChildren))
	copy(newChildren, oldChildren)
	data.Children = newChildren

	// Walk children, pushing the edit into each overlapping child.
	// Per SummarizeChildren convention, child 0 starts at byte 0 relative
	// to the parent (parent.Padding == child0.Padding), so childOffset
	// starts at 0, not padding.Bytes.
	childOffset := uint32(0)
	editAbsorbed := false

	for i := 0; i < len(newChildren); i++ {
		child := newChildren[i]
		childPadding := GetPadding(child, arena)
		childSize := GetSize(child, arena)
		childEnd := childOffset + childPadding.Bytes + childSize.Bytes

		if edit.OldEndByte <= childOffset && editAbsorbed {
			// Edit is before this child and was already absorbed. No overlap.
			break
		}

		if edit.StartByte >= childEnd {
			// Edit starts after this child's span. Check if it falls in the
			// child's lookahead range (the lexer may have peeked past the
			// token boundary). If so, recursively editSubtree so the change
			// propagates to nested children that also have lookahead.
			childLookahead := GetLookaheadBytes(child, arena)
			if childLookahead > 0 && edit.StartByte < childEnd+childLookahead {
				childEdit := InputEdit{
					StartByte:   saturatingSub(edit.StartByte, childOffset),
					OldEndByte:  saturatingSub(edit.OldEndByte, childOffset),
					NewEndByte:  saturatingSub(edit.NewEndByte, childOffset),
					StartPoint:  edit.StartPoint,
					OldEndPoint: edit.OldEndPoint,
					NewEndPoint: edit.NewEndPoint,
				}
				newChildren[i] = editSubtree(child, &childEdit, arena)
			}
			childOffset = childEnd
			continue
		}

		// Edit overlaps this child. Transform edit into child-local coordinates.
		childEdit := InputEdit{
			StartByte:   saturatingSub(edit.StartByte, childOffset),
			OldEndByte:  saturatingSub(edit.OldEndByte, childOffset),
			NewEndByte:  saturatingSub(edit.NewEndByte, childOffset),
			StartPoint:  edit.StartPoint,
			OldEndPoint: edit.OldEndPoint,
			NewEndPoint: edit.NewEndPoint,
		}

		if !editAbsorbed {
			// First overlapping child absorbs the full insertion.
			newChildren[i] = editSubtree(child, &childEdit, arena)
			editAbsorbed = true
		} else {
			// Subsequent overlapping children: only deletion (no insertion).
			shrinkEdit := InputEdit{
				StartByte:   childEdit.StartByte,
				OldEndByte:  childEdit.OldEndByte,
				NewEndByte:  childEdit.StartByte,
				StartPoint:  childEdit.StartPoint,
				OldEndPoint: childEdit.OldEndPoint,
				NewEndPoint: childEdit.StartPoint,
			}
			newChildren[i] = editSubtree(child, &shrinkEdit, arena)
		}

		// Handle depends_on_column: if the edit shifts columns or rows,
		// continue invalidating subsequent children that depend on column.
		if edit.OldEndPoint.Column != edit.NewEndPoint.Column ||
			edit.OldEndPoint.Row != edit.NewEndPoint.Row {
			for j := i + 1; j < len(newChildren); j++ {
				if DependsOnColumn(newChildren[j], arena) {
					cloned := cloneSubtreeInArena(newChildren[j], arena)
					if !cloned.IsInline() {
						arena.Get(cloned).SetFlag(SubtreeFlagHasChanges, true)
					}
					newChildren[j] = cloned
				}
			}
			break // Column invalidation handled all remaining children.
		}

		childOffset = childEnd
	}

	return newTree
}

// EditSubtree is an exported wrapper around editSubtree for internal package
// tests that have moved out of the root package.
func EditSubtree(s Subtree, edit *InputEdit, arena *SubtreeArena) Subtree {
	return editSubtree(s, edit, arena)
}

// adjustNodePaddingAndSize adjusts a subtree's padding and size based on an edit.
// This handles three cases based on where the edit falls relative to the node:
//  1. Edit entirely in padding: adjust padding by delta
//  2. Edit starts in padding, extends into content: reset padding, shrink content
//  3. Edit within content: resize content
func adjustNodePaddingAndSize(s Subtree, arena *SubtreeArena, edit *InputEdit) {
	if s.IsInline() {
		// Inline subtrees are value types — their padding/size are encoded
		// in the 8-byte value. We don't adjust them here because:
		// 1. The parent's adjustNodePaddingAndSize handles the total size change.
		// 2. Inline tokens with has_changes will be re-lexed by the parser,
		//    producing a new token with correct padding/size.
		// 3. tryReuseNode checks has_changes before reusing, so stale inline
		//    padding/size values are never used for position comparisons of
		//    reused subtrees.
		// Note: children's sizes may not sum to parent's size after editing,
		// but this is harmless because changed children are always re-parsed.
		return
	}

	data := arena.Get(s)
	paddingBytes := data.Padding.Bytes
	totalBytes := paddingBytes + data.Size.Bytes

	if edit.OldEndByte <= paddingBytes {
		// Case 1: Edit entirely in padding.
		delta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
		data.Padding.Bytes = uint32(int64(paddingBytes) + delta)
		if edit.OldEndPoint.Row < data.Padding.Point.Row {
			data.Padding.Point.Row = uint32(int64(data.Padding.Point.Row) +
				int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row))
		} else if edit.OldEndPoint.Row == data.Padding.Point.Row {
			colDelta := int64(edit.NewEndPoint.Column) - int64(edit.OldEndPoint.Column)
			data.Padding.Point.Column = uint32(int64(data.Padding.Point.Column) + colDelta)
		}
	} else if edit.StartByte < paddingBytes {
		// Case 2: Edit starts in padding, extends into content.
		overlap := edit.OldEndByte - paddingBytes
		data.Size.Bytes = saturatingSub(data.Size.Bytes, overlap)
		data.Padding = Length{
			Bytes: edit.NewEndByte,
			Point: edit.NewEndPoint,
		}
	} else if edit.StartByte < totalBytes {
		// Case 3: Edit within content.
		delta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
		data.Size.Bytes = uint32(int64(data.Size.Bytes) + delta)
		if edit.OldEndPoint.Row < data.Padding.Point.Row+data.Size.Point.Row {
			rowDelta := int64(edit.NewEndPoint.Row) - int64(edit.OldEndPoint.Row)
			data.Size.Point.Row = uint32(int64(data.Size.Point.Row) + rowDelta)
		}
	}
}

// cloneSubtreeInArena allocates a new copy of a subtree in the same arena.
// For inline subtrees, returns the value as-is (no allocation needed).
// For heap subtrees, copies all fields; the children slice is shared.
func cloneSubtreeInArena(s Subtree, arena *SubtreeArena) Subtree {
	if s.IsInline() || s.IsZero() {
		return s
	}

	oldData := arena.Get(s)
	newSt, newData := arena.Alloc()

	// Copy all scalar fields and the children slice header.
	// Children themselves are not cloned — structural sharing.
	*newData = *oldData

	return newSt
}

// saturatingSub returns a - b, or 0 if a < b.
func saturatingSub(a, b uint32) uint32 {
	if a < b {
		return 0
	}
	return a - b
}

// LengthSaturatingSub returns a - b with saturating subtraction.
func LengthSaturatingSub(a, b Length) Length {
	result := Length{Bytes: saturatingSub(a.Bytes, b.Bytes)}
	if a.Point.Row == b.Point.Row {
		result.Point.Column = saturatingSub(a.Point.Column, b.Point.Column)
	} else {
		result.Point.Row = saturatingSub(a.Point.Row, b.Point.Row)
		result.Point.Column = a.Point.Column
	}
	return result
}
