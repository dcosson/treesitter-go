package stack

import (
	"bytes"
	"sync"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/core"
)

type StateID = ts.StateID
type Length = ts.Length
type Symbol = ts.Symbol
type Subtree = ts.Subtree
type SubtreeArena = ts.SubtreeArena

var SubtreeZero = ts.SubtreeZero
var LengthZero = ts.LengthZero

const (
	SymbolError          Symbol = ts.SymbolError
	SymbolErrorRepeat    Symbol = ts.SymbolErrorRepeat
	ErrorCostPerRecovery        = core.ErrorCostPerRecovery
)

var (
	GetTotalBytes             = ts.GetTotalBytes
	GetErrorCost              = ts.GetErrorCost
	GetDynamicPrecedence      = ts.GetDynamicPrecedence
	GetPadding                = ts.GetPadding
	GetSize                   = ts.GetSize
	GetChildCount             = ts.GetChildCount
	GetSymbol                 = ts.GetSymbol
	GetVisibleDescendantCount = ts.GetVisibleDescendantCount
	GetExternalScannerState   = ts.GetExternalScannerState
	IsExtra                   = ts.IsExtra
	IsVisible                 = ts.IsVisible
)

// Graph-Structured Stack (GSS) for GLR parsing.
//
// The stack supports multiple parse "versions" (heads) that can split
// (on ambiguity), merge (when versions converge to the same state), and
// be paused/resumed (for error recovery). Each version is a path through
// a DAG of StackNodes connected by StackLinks, where each link carries
// a Subtree.
//
// Key operations:
//   - Push: add a new state+subtree to a version
//   - Pop: fan out across merged links (bounded by MaxIteratorCount)
//   - Split: fork a version into two
//   - Merge: combine two versions at the same state
//   - Pause/Resume: error recovery version management
//   - Halt: discard a version

const (
	// MaxLinkCount is the maximum number of links per StackNode.
	// When GLR versions merge, a node can have multiple predecessor links.
	MaxLinkCount = 8

	// MaxIteratorCount bounds the number of paths explored during pop.
	MaxIteratorCount = 64

	// stackNodePoolSize is the target size for the sync.Pool-based free list.
	stackNodePoolSize = 50
)

// StackNode is a node in the graph-structured stack.
// Each node represents a parse state at a particular position.
type StackNode struct {
	state             StateID
	position          Length
	links             [MaxLinkCount]StackLink
	linkCount         uint16
	errorCost         uint32
	nodeCount         uint32
	dynamicPrecedence int32
}

// State returns this node's parse state.
func (n *StackNode) State() StateID {
	if n == nil {
		return 0
	}
	return n.state
}

// Position returns this node's input position.
func (n *StackNode) Position() Length {
	if n == nil {
		return LengthZero
	}
	return n.position
}

// StackLink connects two StackNodes, carrying a subtree (the result of
// a shift or reduce action).
type StackLink struct {
	node      *StackNode
	subtree   Subtree
	isPending bool
}

// stackNodePool manages recycling of StackNodes to reduce GC pressure.
var stackNodePool = sync.Pool{
	New: func() any {
		return &StackNode{}
	},
}

func newStackNode() *StackNode {
	node := stackNodePool.Get().(*StackNode)
	*node = StackNode{} // Zero out for reuse.
	return node
}

func freeStackNode(n *StackNode) {
	if n != nil {
		stackNodePool.Put(n)
	}
}

// StackVersion identifies a stack version (head index).
type StackVersion int

// StackHead represents one active parse version.
type StackHead struct {
	node    *StackNode
	status  StackStatus
	summary []StackSummaryEntry
	// lastExternalToken tracks external scanner state for this version.
	lastExternalToken Subtree
	// nodeCountAtLastError records the node count when the last error
	// occurred on this version. Used by NodeCountSinceError to compute
	// the number of nodes parsed since the last error, which is used
	// by compareVersions for cost amplification.
	nodeCountAtLastError uint32
	// lookaheadWhenPaused stores the token that caused the error, so
	// that handleError can use it when the version is resumed.
	// Mirrors C tree-sitter's ts_stack_pause / ts_stack_resume.
	lookaheadWhenPaused Subtree
}

// StackStatus indicates whether a version is active, paused, or halted.
type StackStatus uint8

const (
	StackStatusActive StackStatus = iota
	StackStatusPaused
	StackStatusHalted
)

// StackSummaryEntry records a parse state reachable by walking back through
// the stack from the current head. Used by error recovery to find previous
// states where the lookahead token might be valid.
// Mirrors C tree-sitter's StackSummaryEntry.
type StackSummaryEntry struct {
	Position Length
	Depth    uint32
	State    StateID
}

// StackIterator holds the result of one pop path.
type StackIterator struct {
	node     *StackNode
	subtrees []Subtree
	depth    uint32
}

// StackSlice is one pop result path paired with the stack version that owns
// the corresponding base node. Mirrors C's StackSlice.
type StackSlice struct {
	version  StackVersion
	subtrees []Subtree
}

// Node returns the base stack node for this pop path.
func (it StackIterator) Node() *StackNode {
	return it.node
}

// Subtrees returns the popped subtrees for this pop path.
func (it StackIterator) Subtrees() []Subtree {
	return it.subtrees
}

// Version returns the stack version for this slice.
func (s StackSlice) Version() StackVersion {
	return s.version
}

// Subtrees returns the popped subtrees for this slice.
func (s StackSlice) Subtrees() []Subtree {
	return s.subtrees
}

// Stack is the Graph-Structured Stack for GLR parsing.
type Stack struct {
	heads []StackHead
	arena *SubtreeArena
}

// NewStack creates a new empty stack.
func NewStack(arena *SubtreeArena) *Stack {
	return &Stack{
		heads: make([]StackHead, 0, 4),
		arena: arena,
	}
}

// VersionCount returns the number of active + paused versions.
func (s *Stack) VersionCount() int {
	return len(s.heads)
}

// HaltedVersionCount returns the number of halted versions.
func (s *Stack) HaltedVersionCount() int {
	count := 0
	for i := range s.heads {
		if s.heads[i].status == StackStatusHalted {
			count++
		}
	}
	return count
}

// ActiveVersionCount returns the number of active (non-paused, non-halted) versions.
func (s *Stack) ActiveVersionCount() int {
	count := 0
	for i := range s.heads {
		if s.heads[i].status == StackStatusActive {
			count++
		}
	}
	return count
}

// State returns the parse state at the top of the given version.
func (s *Stack) State(version StackVersion) StateID {
	if int(version) >= len(s.heads) {
		return 0
	}
	head := &s.heads[version]
	if head.node == nil {
		return 0
	}
	return head.node.state
}

// Position returns the byte position of the given version.
func (s *Stack) Position(version StackVersion) Length {
	if int(version) >= len(s.heads) {
		return LengthZero
	}
	head := &s.heads[version]
	if head.node == nil {
		return LengthZero
	}
	return head.node.position
}

// ErrorCost returns the accumulated error cost for a version.
func (s *Stack) ErrorCost(version StackVersion) uint32 {
	if int(version) >= len(s.heads) {
		return 0
	}
	head := &s.heads[version]
	if head.node == nil {
		return 0
	}
	cost := head.node.errorCost
	// Add recovery bonus for paused versions or versions in ERROR_STATE
	// that have SubtreeZero as their first link (just pushed ERROR_STATE).
	// This matches C's ts_stack_error_cost which adds ERROR_COST_PER_RECOVERY
	// (= 500) for versions in error recovery state.
	if head.status == StackStatusPaused {
		cost += ErrorCostPerRecovery
	} else if head.node.state == 0 && head.node.linkCount > 0 && head.node.links[0].subtree.IsZero() {
		cost += ErrorCostPerRecovery
	}
	return cost
}

// NodeCount returns the node count for a version.
func (s *Stack) NodeCount(version StackVersion) uint32 {
	if int(version) >= len(s.heads) {
		return 0
	}
	head := &s.heads[version]
	if head.node == nil {
		return 0
	}
	return head.node.nodeCount
}

// DynamicPrecedence returns the dynamic precedence for a version.
func (s *Stack) DynamicPrecedence(version StackVersion) int32 {
	if int(version) >= len(s.heads) {
		return 0
	}
	head := &s.heads[version]
	if head.node == nil {
		return 0
	}
	return head.node.dynamicPrecedence
}

// NodeCountSinceError returns the number of nodes parsed since the last
// error on this version. Mirrors ts_stack_node_count_since_error in C.
func (s *Stack) NodeCountSinceError(version StackVersion) uint32 {
	if int(version) >= len(s.heads) {
		return 0
	}
	head := &s.heads[version]
	if head.node == nil {
		return 0
	}
	// If the node count dropped below the error marker (e.g. after a pop),
	// reset the marker to the current count.
	if head.node.nodeCount < head.nodeCountAtLastError {
		head.nodeCountAtLastError = head.node.nodeCount
	}
	return head.node.nodeCount - head.nodeCountAtLastError
}

// HasAdvancedSinceError returns true if the parser has made meaningful
// progress since the last error on this version. It walks the primary
// link chain looking for non-zero-width subtrees. If no error has
// occurred (errorCost == 0), returns true.
// Mirrors C tree-sitter's ts_stack_has_advanced_since_error.
func (s *Stack) HasAdvancedSinceError(version StackVersion) bool {
	if int(version) >= len(s.heads) {
		return false
	}
	head := &s.heads[version]
	node := head.node
	if node == nil {
		return false
	}
	if node.errorCost == 0 {
		return true
	}
	for node != nil {
		if node.linkCount > 0 {
			subtree := node.links[0].subtree
			if !subtree.IsZero() {
				if GetTotalBytes(subtree, s.arena) > 0 {
					return true
				} else if node.nodeCount > head.nodeCountAtLastError &&
					GetErrorCost(subtree, s.arena) == 0 {
					node = node.links[0].node
					continue
				}
			}
		}
		break
	}
	return false
}

// SwapVersions swaps two version heads. Used by condenseStack when a
// higher-indexed version is preferred over a lower-indexed one, to maintain
// the ordering invariant that better versions occupy lower indices.
// Mirrors ts_stack_swap_versions in C.
func (s *Stack) SwapVersions(v1, v2 StackVersion) {
	if int(v1) >= len(s.heads) || int(v2) >= len(s.heads) {
		return
	}
	s.heads[v1], s.heads[v2] = s.heads[v2], s.heads[v1]
}

// Status returns the status of a version.
func (s *Stack) Status(version StackVersion) StackStatus {
	if int(version) >= len(s.heads) {
		return StackStatusHalted
	}
	return s.heads[version].status
}

// IsActive returns true if the version is active.
func (s *Stack) IsActive(version StackVersion) bool {
	return s.Status(version) == StackStatusActive
}

// IsPaused returns true if the version is paused (error recovery).
func (s *Stack) IsPaused(version StackVersion) bool {
	return s.Status(version) == StackStatusPaused
}

// IsHalted returns true if the version is halted (discarded).
func (s *Stack) IsHalted(version StackVersion) bool {
	return s.Status(version) == StackStatusHalted
}

// Push adds a new state to the given version, linked by the given subtree.
func (s *Stack) Push(version StackVersion, state StateID, subtree Subtree, isPending bool, position Length) {
	if int(version) >= len(s.heads) {
		return
	}
	head := &s.heads[version]
	oldNode := head.node

	newNode := newStackNode()
	newNode.state = state
	newNode.position = position
	if oldNode != nil {
		newNode.errorCost = oldNode.errorCost
		newNode.nodeCount = oldNode.nodeCount + 1
		newNode.dynamicPrecedence = oldNode.dynamicPrecedence
	} else {
		newNode.nodeCount = 1
	}

	// Add error cost and dynamic precedence from the subtree.
	if !subtree.IsZero() {
		newNode.errorCost += GetErrorCost(subtree, s.arena)
		newNode.dynamicPrecedence += GetDynamicPrecedence(subtree, s.arena)
	}

	// Link to the old node.
	if oldNode != nil {
		newNode.links[0] = StackLink{
			node:      oldNode,
			subtree:   subtree,
			isPending: isPending,
		}
		newNode.linkCount = 1
	}

	// Match C: if (!subtree.ptr) head->node_count_at_last_error = new_node->node_count;
	// Pushing a zero subtree means entering error state — reset the error
	// baseline so NodeCountSinceError() returns 0.
	if subtree.IsZero() {
		head.nodeCountAtLastError = newNode.nodeCount
	}

	head.node = newNode
}

// Pop performs a fan-out pop on the given version, returning all paths
// of the given depth. Each path consists of the subtrees along that path
// and the StackNode reached at the bottom.
//
// For a simple stack (no merges), this returns exactly one path.
// For merged stacks, it fans out, bounded by MaxIteratorCount.
//
// IMPORTANT: Pop mutates the version's head — after the call, the version's
// top node is advanced to the first result's bottom node (results[0].node).
// This matches C tree-sitter's stack_pop behavior where pop removes the
// top N entries from the stack. Use PopCount for a read-only check.

func (s *Stack) Pop(version StackVersion, count uint32) []StackIterator {
	if int(version) >= len(s.heads) {
		return nil
	}
	head := &s.heads[version]
	if head.node == nil {
		return nil
	}

	var results []StackIterator
	type popFrame struct {
		node     *StackNode
		subtrees []Subtree
		depth    uint32
	}

	// BFS/DFS through the DAG of links.
	// Extra subtrees (e.g. comments) are collected but do NOT count toward
	// the pop depth — matching C tree-sitter's stack__iter which skips extras
	// when incrementing subtree_count.
	queue := []popFrame{{
		node:     head.node,
		subtrees: make([]Subtree, 0, count),
		depth:    0,
	}}

	for len(queue) > 0 && len(results) < MaxIteratorCount {
		frame := queue[0]
		queue = queue[1:]

		if frame.depth == count {
			results = append(results, StackIterator{
				node:     frame.node,
				subtrees: frame.subtrees,
				depth:    frame.depth,
			})
			continue
		}

		if frame.node == nil || frame.node.linkCount == 0 {
			// Reached the bottom of the stack before popping enough.
			continue
		}

		for i := uint16(0); i < frame.node.linkCount; i++ {
			link := &frame.node.links[i]
			newSubtrees := make([]Subtree, len(frame.subtrees)+1)
			copy(newSubtrees, frame.subtrees)
			newSubtrees[len(frame.subtrees)] = link.subtree

			// Extra subtrees don't count toward the pop depth.
			// SubtreeZero links (from ERROR_STATE push with null subtree)
			// always count toward depth.
			newDepth := frame.depth
			if link.subtree.IsZero() || !IsExtra(link.subtree, s.arena) {
				newDepth++
			}

			queue = append(queue, popFrame{
				node:     link.node,
				subtrees: newSubtrees,
				depth:    newDepth,
			})
		}
	}

	// After pop, update the head to point to the first result's node.
	if len(results) > 0 {
		head.node = results[0].node
	}

	return results
}

// PopAll pops all subtrees from the stack for the given version,
// from the head down to the bottom, traversing ALL links at merge points.
// Returns one path per traversal through merged nodes, where each path
// is a []Subtree in source order (leftmost/oldest first).
//
// For a simple (non-merged) stack, returns exactly one path.
// For merged stacks, fans out across all links, bounded by MaxIteratorCount.
//
// This matches C tree-sitter's ts_stack_pop_all which uses stack__iter
// with pop_all_callback to BFS through all links at merged nodes.
func (s *Stack) PopAll(version StackVersion) [][]Subtree {
	if int(version) >= len(s.heads) {
		return nil
	}
	head := &s.heads[version]
	if head.node == nil {
		return nil
	}

	type popFrame struct {
		node     *StackNode
		subtrees []Subtree
	}

	var results [][]Subtree
	queue := []popFrame{{
		node:     head.node,
		subtrees: nil,
	}}

	for len(queue) > 0 && len(results) < MaxIteratorCount {
		frame := queue[0]
		queue = queue[1:]

		// Bottom of stack — complete path.
		if frame.node == nil || frame.node.linkCount == 0 {
			// Reverse to source order (collected top-to-bottom, need bottom-to-top).
			path := make([]Subtree, len(frame.subtrees))
			for i := 0; i < len(frame.subtrees); i++ {
				path[i] = frame.subtrees[len(frame.subtrees)-1-i]
			}
			results = append(results, path)
			continue
		}

		// Fan out across all links at this node.
		for i := uint16(0); i < frame.node.linkCount; i++ {
			link := &frame.node.links[i]
			var newSubtrees []Subtree
			if !link.subtree.IsZero() {
				// Non-null subtree — include in path.
				newSubtrees = make([]Subtree, len(frame.subtrees)+1)
				copy(newSubtrees, frame.subtrees)
				newSubtrees[len(frame.subtrees)] = link.subtree
			} else {
				// SubtreeZero links (from ERROR_STATE push with null subtree) —
				// don't include in output but continue traversal. Matches C's
				// stack__iter which only adds non-null subtrees to the array.
				newSubtrees = make([]Subtree, len(frame.subtrees))
				copy(newSubtrees, frame.subtrees)
			}
			queue = append(queue, popFrame{
				node:     link.node,
				subtrees: newSubtrees,
			})
		}
	}

	// Mark head as consumed. The version will be halted by the caller.
	head.node = nil

	return results
}

// PopPending pops a single pending subtree from the top of the stack.
// A subtree is pending when it was pushed with isPending=true, indicating
// it was a composite reused subtree that may need to be broken down.
// Returns the pending subtree and true if one was found, or SubtreeZero
// and false if the top link is not pending.
// Matches C: ts_stack_pop_pending
func (s *Stack) PopPending(version StackVersion) (Subtree, bool) {
	if int(version) >= len(s.heads) {
		return SubtreeZero, false
	}
	head := &s.heads[version]
	if head.node == nil || head.node.linkCount == 0 {
		return SubtreeZero, false
	}

	link := &head.node.links[0]
	if !link.isPending {
		return SubtreeZero, false
	}

	subtree := link.subtree
	head.node = link.node
	return subtree, true
}

// PopError pops a single error subtree from the top of the stack.
// Checks if any link from the current node has an error subtree, and if so,
// pops it. Returns the error subtree and true if found, or SubtreeZero and
// false if no error subtree is at the top.
// Matches C: ts_stack_pop_error
func (s *Stack) PopError(version StackVersion) (Subtree, bool) {
	if int(version) >= len(s.heads) {
		return SubtreeZero, false
	}
	head := &s.heads[version]
	if head.node == nil || head.node.linkCount == 0 {
		return SubtreeZero, false
	}

	// Check if any link has an error subtree.
	hasError := false
	for i := uint16(0); i < head.node.linkCount; i++ {
		link := &head.node.links[i]
		if !link.subtree.IsZero() && GetSymbol(link.subtree, s.arena) == SymbolError {
			hasError = true
			break
		}
	}
	if !hasError {
		return SubtreeZero, false
	}

	// Pop via the primary link (following C which uses stack__iter with count 1).
	link := &head.node.links[0]
	if link.subtree.IsZero() || GetSymbol(link.subtree, s.arena) != SymbolError {
		return SubtreeZero, false
	}

	subtree := link.subtree
	head.node = link.node
	return subtree, true
}

// Split forks a version, creating a new version at the same position.
// Returns the new version index.
func (s *Stack) Split(version StackVersion) StackVersion {
	if int(version) >= len(s.heads) {
		return -1
	}
	head := s.heads[version]
	newVersion := StackVersion(len(s.heads))
	s.heads = append(s.heads, StackHead{
		node:                 head.node,
		status:               head.status,
		summary:              nil, // Don't share summary reference; each version builds its own
		lastExternalToken:    head.lastExternalToken,
		nodeCountAtLastError: head.nodeCountAtLastError,
		lookaheadWhenPaused:  head.lookaheadWhenPaused,
	})
	return newVersion
}

// ForkAtNode creates a new active version pointing at the given node.
// Used during multi-path reduce and error recovery to create versions for
// alt pop paths. Inherits lastExternalToken and nodeCountAtLastError from
// the source version, matching C's ts_stack__add_version.
func (s *Stack) ForkAtNode(node *StackNode, sourceVersion StackVersion) StackVersion {
	newVersion := StackVersion(len(s.heads))
	var extToken Subtree
	var nodeCountAtLastError uint32
	if int(sourceVersion) < len(s.heads) {
		extToken = s.heads[sourceVersion].lastExternalToken
		nodeCountAtLastError = s.heads[sourceVersion].nodeCountAtLastError
	}
	s.heads = append(s.heads, StackHead{
		node:                 node,
		status:               StackStatusActive,
		lastExternalToken:    extToken,
		nodeCountAtLastError: nodeCountAtLastError,
	})
	return newVersion
}

// subtreeNodeCount returns the number of nodes in a subtree for progress
// tracking. Matches C's stack__subtree_node_count.
func subtreeNodeCount(s Subtree, arena *SubtreeArena) uint32 {
	count := GetVisibleDescendantCount(s, arena)
	if IsVisible(s, arena) {
		count++
	}
	// Count intermediate error nodes even though they are not visible,
	// because a stack version's node count is used to check whether it
	// has made any progress since the last time it encountered an error.
	if GetSymbol(s, arena) == SymbolErrorRepeat {
		count++
	}
	return count
}

// subtreeIsEquivalent checks if two subtrees are equivalent for merge
// deduplication purposes. Matches C's stack__subtree_is_equivalent.
func (s *Stack) subtreeIsEquivalent(left, right Subtree) bool {
	if left == right {
		return true
	}
	if left.IsZero() || right.IsZero() {
		return false
	}

	// Symbols must match.
	if GetSymbol(left, s.arena) != GetSymbol(right, s.arena) {
		return false
	}

	// If both have errors, don't bother keeping both.
	if GetErrorCost(left, s.arena) > 0 && GetErrorCost(right, s.arena) > 0 {
		return true
	}

	return GetPadding(left, s.arena).Bytes == GetPadding(right, s.arena).Bytes &&
		GetSize(left, s.arena).Bytes == GetSize(right, s.arena).Bytes &&
		GetChildCount(left, s.arena) == GetChildCount(right, s.arena) &&
		IsExtra(left, s.arena) == IsExtra(right, s.arena) &&
		bytes.Equal(GetExternalScannerState(left, s.arena), GetExternalScannerState(right, s.arena))
}

// nodeAddLink adds a link to a stack node, handling three cases for
// deduplication. This is a faithful port of C's stack_node_add_link.
//
// Case 1: If an existing link has an equivalent subtree AND the same target
// node, replace the subtree if the new one has higher DynPrec (disambiguation).
//
// Case 2: If an existing link has an equivalent subtree AND the target node
// has the same state/position/errorCost, recursively merge the target nodes.
//
// Case 3: Otherwise, add as a new link.
//
// In all cases, update the node's accumulated dynamicPrecedence.
func (s *Stack) nodeAddLink(node *StackNode, link StackLink) {
	// Prevent self-loops.
	if link.node == node {
		return
	}

	for i := uint16(0); i < node.linkCount; i++ {
		existing := &node.links[i]
		if s.subtreeIsEquivalent(existing.subtree, link.subtree) {
			// Case 1: Same target node — disambiguation.
			// Keep only the higher DynPrec subtree.
			if existing.node == link.node {
				if !link.subtree.IsZero() && !existing.subtree.IsZero() {
					newPrec := GetDynamicPrecedence(link.subtree, s.arena)
					oldPrec := GetDynamicPrecedence(existing.subtree, s.arena)
					if newPrec > oldPrec {
						existing.subtree = link.subtree
						node.dynamicPrecedence = link.node.dynamicPrecedence +
							GetDynamicPrecedence(link.subtree, s.arena)
					}
				}
				return
			}

			// Case 2: Same state — recursive merge.
			if existing.node.state == link.node.state &&
				existing.node.position.Bytes == link.node.position.Bytes &&
				existing.node.errorCost == link.node.errorCost {
				for j := uint16(0); j < link.node.linkCount; j++ {
					s.nodeAddLink(existing.node, link.node.links[j])
				}
				dynPrec := link.node.dynamicPrecedence
				if !link.subtree.IsZero() {
					dynPrec += GetDynamicPrecedence(link.subtree, s.arena)
				}
				if dynPrec > node.dynamicPrecedence {
					node.dynamicPrecedence = dynPrec
				}
				return
			}
		}
	}

	// Case 3: No match — add as new link.
	if node.linkCount >= MaxLinkCount {
		return
	}

	nodeCount := link.node.nodeCount
	dynPrec := link.node.dynamicPrecedence
	node.links[node.linkCount] = link
	node.linkCount++

	if !link.subtree.IsZero() {
		nodeCount += subtreeNodeCount(link.subtree, s.arena)
		dynPrec += GetDynamicPrecedence(link.subtree, s.arena)
	}

	if nodeCount > node.nodeCount {
		node.nodeCount = nodeCount
	}
	if dynPrec > node.dynamicPrecedence {
		node.dynamicPrecedence = dynPrec
	}
}

// Merge combines two versions that have reached the same state.
// The source version's links are added to the target version's node
// using nodeAddLink which handles deduplication and DynPrec updates.
// The source version is removed (matching C's ts_stack_merge).
//
// Returns true if the merge was successful.
func (s *Stack) Merge(target, source StackVersion) bool {
	if !s.CanMerge(target, source) {
		return false
	}

	targetHead := &s.heads[target]
	sourceHead := &s.heads[source]

	// If merging in error state, update the error marker.
	// Matches C: if (head1->node->state == ERROR_STATE)
	//   head1->node_count_at_last_error = head1->node->node_count;
	if targetHead.node.state == 0 {
		targetHead.nodeCountAtLastError = targetHead.node.nodeCount
	}

	// Add source's links to target using the three-case add logic.
	targetNode := targetHead.node
	sourceNode := sourceHead.node
	for i := uint16(0); i < sourceNode.linkCount; i++ {
		s.nodeAddLink(targetNode, sourceNode.links[i])
	}

	// Remove the source version (matches C's ts_stack_merge behavior).
	// After merge, the source version is gone and indices shift down.
	s.RemoveVersion(source)
	return true
}

// CanMerge returns true if two versions can be merged.
// Matches C tree-sitter's ts_stack_can_merge: requires same state, both
// active, same position, same error cost, and same external scanner state.
func (s *Stack) CanMerge(v1, v2 StackVersion) bool {
	if int(v1) >= len(s.heads) || int(v2) >= len(s.heads) {
		return false
	}
	h1 := &s.heads[v1]
	h2 := &s.heads[v2]
	if h1.node == nil || h2.node == nil {
		return false
	}
	if h1.status != StackStatusActive || h2.status != StackStatusActive {
		return false
	}
	if h1.node.state != h2.node.state {
		return false
	}
	if h1.node.position.Bytes != h2.node.position.Bytes {
		return false
	}
	if h1.node.errorCost != h2.node.errorCost {
		return false
	}
	// Compare external scanner states.
	state1 := GetExternalScannerState(h1.lastExternalToken, s.arena)
	state2 := GetExternalScannerState(h2.lastExternalToken, s.arena)
	if !bytes.Equal(state1, state2) {
		return false
	}
	return true
}

// Pause pauses a version and stores the lookahead token that triggered the error.
// The token is returned when the version is resumed via Resume.
// Mirrors C tree-sitter's ts_stack_pause.
func (s *Stack) Pause(version StackVersion, lookahead Subtree) {
	if int(version) >= len(s.heads) {
		return
	}
	head := &s.heads[version]
	head.status = StackStatusPaused
	head.lookaheadWhenPaused = lookahead
	// Record the current node count as the error point, matching C's
	// ts_stack_pause which sets node_count_at_last_error.
	if head.node != nil {
		head.nodeCountAtLastError = head.node.nodeCount
	}
}

// Resume resumes a paused version and returns the stored lookahead token.
// Mirrors C tree-sitter's ts_stack_resume.
func (s *Stack) Resume(version StackVersion) Subtree {
	if int(version) >= len(s.heads) {
		return SubtreeZero
	}
	head := &s.heads[version]
	if head.status == StackStatusPaused {
		head.status = StackStatusActive
	}
	lookahead := head.lookaheadWhenPaused
	head.lookaheadWhenPaused = SubtreeZero
	return lookahead
}

// Halt discards a version.
func (s *Stack) Halt(version StackVersion) {
	if int(version) >= len(s.heads) {
		return
	}
	s.heads[version].status = StackStatusHalted
}

// RemoveVersion removes a halted version from the heads slice.
// This shifts subsequent version indices down by one.
func (s *Stack) RemoveVersion(version StackVersion) {
	if int(version) >= len(s.heads) {
		return
	}
	s.heads = append(s.heads[:version], s.heads[version+1:]...)
}

// Clear resets the stack, removing all versions.
func (s *Stack) Clear() {
	s.heads = s.heads[:0]
}

// AddVersion adds a new version with the given initial state and position.
// Returns the new version index.
func (s *Stack) AddVersion(state StateID, position Length) StackVersion {
	node := newStackNode()
	node.state = state
	node.position = position
	node.nodeCount = 1

	version := StackVersion(len(s.heads))
	s.heads = append(s.heads, StackHead{
		node:   node,
		status: StackStatusActive,
	})
	return version
}

// TopSubtree returns the subtree on the top link of the given version.
// Returns SubtreeZero if the version has no links.
func (s *Stack) TopSubtree(version StackVersion) Subtree {
	if int(version) >= len(s.heads) {
		return SubtreeZero
	}
	head := &s.heads[version]
	if head.node == nil || head.node.linkCount == 0 {
		return SubtreeZero
	}
	return head.node.links[0].subtree
}

// SetLastExternalToken records the last external token for a version.
func (s *Stack) SetLastExternalToken(version StackVersion, token Subtree) {
	if int(version) >= len(s.heads) {
		return
	}
	s.heads[version].lastExternalToken = token
}

// LastExternalToken returns the last external token for a version.
func (s *Stack) LastExternalToken(version StackVersion) Subtree {
	if int(version) >= len(s.heads) {
		return SubtreeZero
	}
	return s.heads[version].lastExternalToken
}

// AddErrorCost adds to the error cost of a version's top node and records
// the current node count as the error baseline for NodeCountSinceError.
func (s *Stack) AddErrorCost(version StackVersion, cost uint32) {
	if int(version) >= len(s.heads) {
		return
	}
	head := &s.heads[version]
	if head.node != nil {
		head.node.errorCost += cost
		// Record the current node count as the "last error" baseline.
		// NodeCountSinceError will measure from this point forward.
		head.nodeCountAtLastError = head.node.nodeCount
	}
}

// SetNodeMetrics sets internal metrics for a version's top node.
// Intended for package-external tests that need to seed condense/recovery states.
func (s *Stack) SetNodeMetrics(version StackVersion, errorCost uint32, nodeCount uint32, dynPrec int32) {
	if int(version) >= len(s.heads) {
		return
	}
	head := &s.heads[version]
	if head.node == nil {
		return
	}
	head.node.errorCost = errorCost
	head.node.nodeCount = nodeCount
	head.node.dynamicPrecedence = dynPrec
}

// CompactHaltedVersions removes all halted versions from the stack.
func (s *Stack) CompactHaltedVersions() {
	n := 0
	for i := range s.heads {
		if s.heads[i].status != StackStatusHalted {
			s.heads[n] = s.heads[i]
			n++
		}
	}
	s.heads = s.heads[:n]
}

type stackAction uint8

const (
	stackActionNone stackAction = 0
	stackActionPop  stackAction = 1 << 0
	stackActionStop stackAction = 1 << 1
)

type stackIterState struct {
	node         *StackNode
	subtrees     []Subtree
	subtreeCount uint32
	isPending    bool
}

func (s *Stack) addVersionFromNode(original StackVersion, node *StackNode) StackVersion {
	if int(original) >= len(s.heads) {
		return -1
	}
	head := s.heads[original]
	version := StackVersion(len(s.heads))
	s.heads = append(s.heads, StackHead{
		node:                 node,
		status:               StackStatusActive,
		lastExternalToken:    head.lastExternalToken,
		nodeCountAtLastError: head.nodeCountAtLastError,
		lookaheadWhenPaused:  SubtreeZero,
	})
	return version
}

func (s *Stack) addSlice(slices *[]StackSlice, original StackVersion, node *StackNode, subtrees []Subtree) {
	// Match C ts_stack__add_slice: reuse existing version when it has same base node.
	for i := len(*slices) - 1; i >= 0; i-- {
		version := (*slices)[i].version
		if int(version) < len(s.heads) && s.heads[version].node == node {
			slice := StackSlice{version: version, subtrees: subtrees}
			*slices = append(*slices, StackSlice{})
			copy((*slices)[i+2:], (*slices)[i+1:])
			(*slices)[i+1] = slice
			return
		}
	}

	version := s.addVersionFromNode(original, node)
	if version < 0 {
		return
	}
	*slices = append(*slices, StackSlice{version: version, subtrees: subtrees})
}

func reverseSubtrees(items []Subtree) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func (s *Stack) iterate(
	version StackVersion,
	goalSubtreeCount int,
	callback func(*stackIterState) stackAction,
) []StackSlice {
	if int(version) >= len(s.heads) {
		return nil
	}
	head := &s.heads[version]
	if head.node == nil {
		return nil
	}

	includeSubtrees := goalSubtreeCount >= 0
	slices := make([]StackSlice, 0, 4)
	iterators := make([]stackIterState, 0, 4)
	initialCap := 0
	if goalSubtreeCount > 0 {
		initialCap = goalSubtreeCount
	}
	iterators = append(iterators, stackIterState{
		node:      head.node,
		subtrees:  make([]Subtree, 0, initialCap),
		isPending: true,
	})

	for len(iterators) > 0 {
		for i, size := 0, len(iterators); i < size; i++ {
			it := &iterators[i]
			node := it.node

			action := callback(it)
			shouldPop := action&stackActionPop != 0
			shouldStop := action&stackActionStop != 0 || node.linkCount == 0

			if shouldPop {
				subtrees := it.subtrees
				if !shouldStop {
					subtrees = append([]Subtree(nil), subtrees...)
				}
				reverseSubtrees(subtrees)
				s.addSlice(&slices, version, node, subtrees)
			}

			if shouldStop {
				iterators = append(iterators[:i], iterators[i+1:]...)
				i--
				size--
				continue
			}

			// Match C stack__iter fanout/link ordering.
			for j := uint16(1); j <= node.linkCount; j++ {
				var next *stackIterState
				var link StackLink
				if j == node.linkCount {
					link = node.links[0]
					next = &iterators[i]
				} else {
					if len(iterators) >= MaxIteratorCount {
						continue
					}
					link = node.links[j]
					current := iterators[i]
					current.subtrees = append([]Subtree(nil), current.subtrees...)
					iterators = append(iterators, current)
					next = &iterators[len(iterators)-1]
				}

				next.node = link.node
				if !link.subtree.IsZero() {
					if includeSubtrees {
						next.subtrees = append(next.subtrees, link.subtree)
					}
					if !IsExtra(link.subtree, s.arena) {
						next.subtreeCount++
						if !link.isPending {
							next.isPending = false
						}
					}
				} else {
					next.subtreeCount++
					next.isPending = false
				}
			}
		}
	}

	return slices
}

// PopCountSlices performs C-style pop-count traversal and returns one slice per
// pop path, with version-per-path grouping by base node.
func (s *Stack) PopCountSlices(version StackVersion, count uint32) []StackSlice {
	goal := count
	return s.iterate(version, int(count), func(it *stackIterState) stackAction {
		if it.subtreeCount == goal {
			return stackActionPop | stackActionStop
		}
		return stackActionNone
	})
}

// PopCount returns the number of pop results for the given version and depth,
// without modifying the stack. Used to check if a pop would produce multiple paths.
func (s *Stack) PopCount(version StackVersion, count uint32) int {
	return len(s.PopCountSlices(version, count))
}

// summaryIterator tracks a single path through the stack during summary recording.
type summaryIterator struct {
	node  *StackNode
	depth uint32
}

// RecordSummary walks back through the stack from the given version's head,
// recording states and positions at each depth up to maxDepth. The resulting
// summary is used by error recovery to find previous states where the
// lookahead token might be valid.
//
// Uses C tree-sitter's iterative BFS approach with MaxIteratorCount (64) cap
// on concurrent iterators to prevent worst-case blowup on merged stack DAGs.
func (s *Stack) RecordSummary(version StackVersion, maxDepth uint32) {
	if int(version) >= len(s.heads) {
		return
	}
	head := &s.heads[version]
	head.summary = nil
	if head.node == nil || head.node.linkCount == 0 {
		return
	}

	// Direct port of C's stack__iter with summarize_stack_callback.
	// C uses DFS with NO visited set — same node may be visited at different
	// depths via different GSS paths. This is critical for correct recovery:
	// if a state is reachable at depth 3 via one path and depth 5 via another,
	// both entries must be recorded.
	var entries []StackSummaryEntry

	type iter struct {
		node          *StackNode
		subtreeCount  uint32
	}

	iterators := make([]iter, 0, MaxIteratorCount)
	iterators = append(iterators, iter{node: head.node, subtreeCount: 0})

	for len(iterators) > 0 {
		// Process all current iterators, collecting new ones for next round.
		// Matches C's stack__iter: for (i = 0, size = self->iterators.size; i < size; i++)
		size := len(iterators)
		for i := 0; i < size; i++ {
			it := iterators[i]
			node := it.node
			depth := it.subtreeCount

			// Callback: summarize_stack_callback (reference/stack.c:606-622)
			// Stop if depth exceeds max.
			if depth > maxDepth {
				// Remove this iterator (stop).
				iterators = append(iterators[:i], iterators[i+1:]...)
				i--
				size--
				continue
			}

			// Record entry: deduplicate by (depth, state) scanning backward.
			// Matches C: for (i = summary->size - 1; i + 1 > 0; i--)
			state := node.state
			duplicate := false
			for j := len(entries) - 1; j >= 0; j-- {
				if entries[j].Depth < depth {
					break
				}
				if entries[j].Depth == depth && entries[j].State == state {
					duplicate = true
					break
				}
			}
			if !duplicate {
				entries = append(entries, StackSummaryEntry{
					Position: node.position,
					Depth:    depth,
					State:    state,
				})
			}

			// Stop if leaf node (no links).
			if node.linkCount == 0 {
				iterators = append(iterators[:i], iterators[i+1:]...)
				i--
				size--
				continue
			}

			// Follow links. Matches C's stack__iter link traversal
			// (reference/stack.c:382-414).
			// C processes links[1..N-1] first (spawning new iterators),
			// then reuses the current iterator for links[0].
			for j := uint16(1); j < node.linkCount; j++ {
				if len(iterators) >= MaxIteratorCount {
					continue
				}
				link := &node.links[j]
				if link.node == nil {
					continue
				}
				newCount := depth
				if link.subtree.IsZero() {
					newCount++
				} else if !IsExtra(link.subtree, s.arena) {
					newCount++
				}
				iterators = append(iterators, iter{
					node:         link.node,
					subtreeCount: newCount,
				})
			}

			// Reuse current iterator for link[0] (matches C).
			link := &node.links[0]
			if link.node == nil {
				iterators = append(iterators[:i], iterators[i+1:]...)
				i--
				size--
				continue
			}
			newCount := depth
			if link.subtree.IsZero() {
				newCount++
			} else if !IsExtra(link.subtree, s.arena) {
				newCount++
			}
			iterators[i] = iter{
				node:         link.node,
				subtreeCount: newCount,
			}
		}
	}

	head.summary = entries
}

// GetSummary returns the recorded summary for a version.
func (s *Stack) GetSummary(version StackVersion) []StackSummaryEntry {
	if int(version) >= len(s.heads) {
		return nil
	}
	return s.heads[version].summary
}

// RenumberVersion replaces version 'to' with version 'from', then removes
// 'from'. Requires from > to. Used during error recovery when multiple pop
// paths need to be consolidated.
// Mirrors C tree-sitter's ts_stack_renumber_version.
func (s *Stack) RenumberVersion(from, to StackVersion) {
	if from == to {
		return
	}
	if int(from) >= len(s.heads) || int(to) >= len(s.heads) {
		return
	}
	// If the target has a summary but the source doesn't, transfer it.
	// This preserves recovery information when renumbering after pop.
	// Matches C: if (target_head->summary && !source_head->summary)
	sourceHead := &s.heads[from]
	targetHead := &s.heads[to]
	if targetHead.summary != nil && sourceHead.summary == nil {
		sourceHead.summary = targetHead.summary
		targetHead.summary = nil
	}
	s.heads[to] = s.heads[from]
	s.RemoveVersion(from)
}
