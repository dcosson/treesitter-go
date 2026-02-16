package treesitter

import "sync"

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
	summary StackSummary
	// lastExternalToken tracks external scanner state for this version.
	lastExternalToken Subtree
}

// StackStatus indicates whether a version is active, paused, or halted.
type StackStatus uint8

const (
	StackStatusActive StackStatus = iota
	StackStatusPaused
	StackStatusHalted
)

// StackSummary accumulates metadata for a version's parse history.
type StackSummary struct {
	errorCost         uint32
	nodeCount         uint32
	dynamicPrecedence int32
}

// StackIterator holds the result of one pop path.
type StackIterator struct {
	node     *StackNode
	subtrees []Subtree
	depth    uint32
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
	return head.node.errorCost
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

	head.node = newNode
}

// Pop performs a fan-out pop on the given version, returning all paths
// of the given depth. Each path consists of the subtrees along that path
// and the StackNode reached at the bottom.
//
// For a simple stack (no merges), this returns exactly one path.
// For merged stacks, it fans out, bounded by MaxIteratorCount.
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

			queue = append(queue, popFrame{
				node:     link.node,
				subtrees: newSubtrees,
				depth:    frame.depth + 1,
			})
		}
	}

	// After pop, update the head to point to the first result's node.
	if len(results) > 0 {
		head.node = results[0].node
	}

	return results
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
		node:              head.node,
		status:            head.status,
		summary:           head.summary,
		lastExternalToken: head.lastExternalToken,
	})
	return newVersion
}

// Merge combines two versions that have reached the same state.
// The source version's top node is added as an additional link on the
// target version's top node. The source version is halted.
//
// Returns true if the merge was successful.
func (s *Stack) Merge(target, source StackVersion) bool {
	if int(target) >= len(s.heads) || int(source) >= len(s.heads) {
		return false
	}
	targetHead := &s.heads[target]
	sourceHead := &s.heads[source]

	if targetHead.node == nil || sourceHead.node == nil {
		return false
	}

	// States must match for merge.
	if targetHead.node.state != sourceHead.node.state {
		return false
	}

	// Add source's links to target.
	targetNode := targetHead.node
	sourceNode := sourceHead.node
	for i := uint16(0); i < sourceNode.linkCount; i++ {
		if targetNode.linkCount >= MaxLinkCount {
			break
		}
		targetNode.links[targetNode.linkCount] = sourceNode.links[i]
		targetNode.linkCount++
	}

	// Halt the source version.
	sourceHead.status = StackStatusHalted
	return true
}

// CanMerge returns true if two versions can be merged (same state).
func (s *Stack) CanMerge(v1, v2 StackVersion) bool {
	if int(v1) >= len(s.heads) || int(v2) >= len(s.heads) {
		return false
	}
	h1 := &s.heads[v1]
	h2 := &s.heads[v2]
	if h1.node == nil || h2.node == nil {
		return false
	}
	return h1.node.state == h2.node.state
}

// Pause pauses a version (for error recovery exploration).
func (s *Stack) Pause(version StackVersion) {
	if int(version) >= len(s.heads) {
		return
	}
	s.heads[version].status = StackStatusPaused
}

// Resume resumes a paused version.
func (s *Stack) Resume(version StackVersion) {
	if int(version) >= len(s.heads) {
		return
	}
	if s.heads[version].status == StackStatusPaused {
		s.heads[version].status = StackStatusActive
	}
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

// PopCount returns the number of pop results for the given version and depth,
// without modifying the stack. Used to check if a pop would produce multiple paths.
func (s *Stack) PopCount(version StackVersion, count uint32) int {
	if int(version) >= len(s.heads) {
		return 0
	}
	head := &s.heads[version]
	if head.node == nil {
		return 0
	}

	type walkFrame struct {
		node  *StackNode
		depth uint32
	}

	resultCount := 0
	queue := []walkFrame{{node: head.node, depth: 0}}

	for len(queue) > 0 && resultCount < MaxIteratorCount {
		frame := queue[0]
		queue = queue[1:]

		if frame.depth == count {
			resultCount++
			continue
		}

		if frame.node == nil || frame.node.linkCount == 0 {
			continue
		}

		for i := uint16(0); i < frame.node.linkCount; i++ {
			queue = append(queue, walkFrame{
				node:  frame.node.links[i].node,
				depth: frame.depth + 1,
			})
		}
	}

	return resultCount
}
