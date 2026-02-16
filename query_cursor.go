package treesitter

// queryState tracks an in-progress pattern match in the QueryCursor.
type queryState struct {
	id                         uint32
	captureListID              uint32
	startDepth                 uint16
	stepIndex                  uint16
	patternIndex               uint16
	consumedCaptureCount       uint16
	seekingImmediateMatch      bool
	hasInProgressAlternatives  bool
	dead                       bool
	needsParent                bool
}

// captureListPool manages reusable capture lists for query states.
type captureListPool struct {
	lists [][]QueryCapture
	free  []uint32
	max   uint32
}

func newCaptureListPool(maxCount uint32) captureListPool {
	return captureListPool{
		max: maxCount,
	}
}

func (pool *captureListPool) acquire() (uint32, bool) {
	if len(pool.free) > 0 {
		id := pool.free[len(pool.free)-1]
		pool.free = pool.free[:len(pool.free)-1]
		pool.lists[id] = pool.lists[id][:0]
		return id, true
	}
	if uint32(len(pool.lists)) >= pool.max {
		return 0, false
	}
	id := uint32(len(pool.lists))
	pool.lists = append(pool.lists, nil)
	return id, true
}

func (pool *captureListPool) release(id uint32) {
	if int(id) < len(pool.lists) {
		pool.lists[id] = pool.lists[id][:0]
		pool.free = append(pool.free, id)
	}
}

func (pool *captureListPool) get(id uint32) []QueryCapture {
	if int(id) < len(pool.lists) {
		return pool.lists[id]
	}
	return nil
}

func (pool *captureListPool) addCapture(id uint32, capture QueryCapture) {
	if int(id) < len(pool.lists) {
		pool.lists[id] = append(pool.lists[id], capture)
	}
}

func (pool *captureListPool) clone(srcID uint32) (uint32, bool) {
	newID, ok := pool.acquire()
	if !ok {
		return 0, false
	}
	src := pool.get(srcID)
	if src != nil {
		dst := make([]QueryCapture, len(src))
		copy(dst, src)
		pool.lists[newID] = dst
	}
	return newID, true
}

// QueryCursor executes a compiled query against a parse tree.
type QueryCursor struct {
	query           *Query
	cursor          TreeCursor
	states          []queryState
	finishedStates  []queryState
	capturePool     captureListPool
	depth           uint32
	startByte       uint32
	endByte         uint32
	nextStateID     uint32
	ascending       bool
	halted          bool
	didExceedLimit  bool
	maxStartDepth   uint32
}

const (
	defaultMaxCaptureListCount = 32
	maxQueryCursorStartDepth   = 0xFFFFFFFF
)

// NewQueryCursor creates a new QueryCursor for the given query.
func NewQueryCursor(query *Query) *QueryCursor {
	return &QueryCursor{
		query:         query,
		capturePool:   newCaptureListPool(defaultMaxCaptureListCount),
		startByte:     0,
		endByte:       0xFFFFFFFF,
		maxStartDepth: maxQueryCursorStartDepth,
	}
}

// SetByteRange restricts the cursor to only match nodes within [startByte, endByte).
func (qc *QueryCursor) SetByteRange(startByte, endByte uint32) {
	qc.startByte = startByte
	qc.endByte = endByte
}

// Exec starts executing the query on the given node.
func (qc *QueryCursor) Exec(node Node) {
	qc.cursor = NewTreeCursor(node)
	qc.states = qc.states[:0]
	qc.finishedStates = qc.finishedStates[:0]
	qc.depth = 0
	qc.ascending = false
	qc.halted = false
	qc.didExceedLimit = false
	qc.nextStateID = 0
	qc.capturePool = newCaptureListPool(defaultMaxCaptureListCount)
}

// NextMatch returns the next complete match, or false if no more matches.
func (qc *QueryCursor) NextMatch() (QueryMatch, bool) {
	for {
		if len(qc.finishedStates) > 0 {
			state := qc.finishedStates[0]
			qc.finishedStates = qc.finishedStates[1:]
			captures := qc.capturePool.get(state.captureListID)
			match := QueryMatch{
				ID:           state.id,
				PatternIndex: state.patternIndex,
				Captures:     make([]QueryCapture, len(captures)),
			}
			copy(match.Captures, captures)
			qc.capturePool.release(state.captureListID)
			return match, true
		}

		if qc.halted {
			return QueryMatch{}, false
		}

		if !qc.advance() {
			return QueryMatch{}, false
		}
	}
}

// NextCapture returns the next capture from the next match.
// Returns the match, capture index within the match, and whether a result was found.
func (qc *QueryCursor) NextCapture() (QueryMatch, uint32, bool) {
	match, ok := qc.NextMatch()
	if !ok {
		return QueryMatch{}, 0, false
	}
	return match, 0, true
}

// advance drives the cursor forward one step, processing nodes.
func (qc *QueryCursor) advance() bool {
	for {
		if qc.halted {
			return false
		}

		if qc.ascending {
			// Ascending: check for finished states and move up.
			qc.checkFinishedStates()

			if qc.cursor.GotoNextSibling() {
				qc.ascending = false
				// Continue to descend phase.
			} else if qc.cursor.GotoParent() {
				if qc.depth > 0 {
					qc.depth--
				}
				continue
			} else {
				// At root, done.
				qc.halted = true
				qc.checkFinishedStates()
				return len(qc.finishedStates) > 0
			}
		}

		// Descend phase: process the current node.
		node := qc.cursor.CurrentNode()
		if node.IsNull() {
			qc.halted = true
			return len(qc.finishedStates) > 0
		}

		// Range restriction.
		nodeStart := node.StartByte()
		nodeEnd := node.EndByte()
		if nodeEnd <= qc.startByte {
			// Node is before range — skip.
			if !qc.cursor.GotoNextSibling() {
				qc.ascending = true
			}
			continue
		}
		if nodeStart >= qc.endByte {
			// Node is after range — skip subtree.
			qc.ascending = true
			continue
		}

		// Start new pattern states for this node.
		if qc.depth <= qc.maxStartDepth {
			qc.introduceNewStates(node)
		}

		// Advance existing states.
		qc.advanceStates(node)

		// Try to descend.
		if qc.cursor.GotoFirstChild() {
			qc.depth++
		} else {
			qc.ascending = true
		}

		// Check if we have finished states to return.
		if len(qc.finishedStates) > 0 {
			return true
		}
	}
}

// introduceNewStates creates new query states for patterns whose first step
// could match the given node.
func (qc *QueryCursor) introduceNewStates(node Node) {
	sym := node.Symbol()
	isNamed := node.IsNamed()

	// Find patterns in the pattern map that could match this symbol.
	for _, entry := range qc.query.patternMap {
		step := &qc.query.steps[entry.stepIndex]

		// Check if the step matches this node.
		if !qc.stepMatchesNode(step, node, sym, isNamed) {
			continue
		}

		// Create a new state for this pattern.
		listID, ok := qc.capturePool.acquire()
		if !ok {
			qc.didExceedLimit = true
			continue
		}

		// Add captures for the first step.
		for i := 0; i < maxStepCaptureCount; i++ {
			if step.captureIDs[i] == noneValue {
				break
			}
			qc.capturePool.addCapture(listID, QueryCapture{
				Node:  node,
				Index: uint32(step.captureIDs[i]),
			})
		}

		state := queryState{
			id:            qc.nextStateID,
			captureListID: listID,
			startDepth:    uint16(qc.depth),
			stepIndex:     entry.stepIndex + 1, // advance past the matched first step
			patternIndex:  entry.patternIndex,
		}
		qc.nextStateID++

		// Check if this single-step pattern is already done.
		if qc.isStepDone(state.stepIndex) {
			qc.finishedStates = append(qc.finishedStates, state)
		} else {
			qc.states = append(qc.states, state)
		}
	}
}

// advanceStates tries to advance all active query states with the current node.
func (qc *QueryCursor) advanceStates(node Node) {
	sym := node.Symbol()
	isNamed := node.IsNamed()

	i := 0
	for i < len(qc.states) {
		state := &qc.states[i]
		if state.dead {
			qc.capturePool.release(state.captureListID)
			qc.states = append(qc.states[:i], qc.states[i+1:]...)
			continue
		}

		step := &qc.query.steps[state.stepIndex]

		// Check depth: the step's depth should match the current relative depth.
		expectedDepth := state.startDepth + step.depth
		if uint32(expectedDepth) != qc.depth {
			i++
			continue
		}

		// Handle pass-through steps (from alternation/quantifier).
		if step.isPassThrough() {
			// Split: one state continues to next step, one goes to alternative.
			if step.alternativeIndex != noneValue {
				altListID, ok := qc.capturePool.clone(state.captureListID)
				if ok {
					altState := *state
					altState.stepIndex = step.alternativeIndex
					altState.captureListID = altListID
					altState.id = qc.nextStateID
					qc.nextStateID++
					qc.states = append(qc.states, altState)
				}
			}
			state.stepIndex++
			continue
		}

		// Handle dead-end steps (redirect only).
		if step.isDeadEnd() {
			if step.alternativeIndex != noneValue {
				state.stepIndex = step.alternativeIndex
			} else {
				state.dead = true
			}
			continue
		}

		// Try to match this step against the current node.
		if !qc.stepMatchesNode(step, node, sym, isNamed) {
			// Check if this is an immediate step that can't be skipped.
			if state.seekingImmediateMatch {
				state.dead = true
			}
			i++
			continue
		}

		// Check field constraint.
		if step.field != 0 {
			fieldID := qc.currentFieldID()
			if fieldID != step.field {
				i++
				continue
			}
		}

		// Check negated fields.
		if step.negatedFieldListID != noneValue {
			if int(step.negatedFieldListID) < len(qc.query.negatedFields) {
				negFields := qc.query.negatedFields[step.negatedFieldListID]
				hasNegatedField := false
				for _, nf := range negFields {
					if !node.ChildByFieldID(nf).IsNull() {
						hasNegatedField = true
						break
					}
				}
				if hasNegatedField {
					i++
					continue
				}
			}
		}

		// Match! Add captures.
		for ci := 0; ci < maxStepCaptureCount; ci++ {
			if step.captureIDs[ci] == noneValue {
				break
			}
			qc.capturePool.addCapture(state.captureListID, QueryCapture{
				Node:  node,
				Index: uint32(step.captureIDs[ci]),
			})
		}

		// Handle alternatives for this step (split).
		if step.alternativeIndex != noneValue && !step.isDeadEnd() {
			altListID, ok := qc.capturePool.clone(state.captureListID)
			if ok {
				altState := *state
				altState.stepIndex = step.alternativeIndex
				altState.captureListID = altListID
				altState.id = qc.nextStateID
				qc.nextStateID++
				qc.states = append(qc.states, altState)
			}
		}

		// Advance the step.
		state.stepIndex++
		state.seekingImmediateMatch = step.isImmediate()

		// Check if pattern is done.
		if qc.isStepDone(state.stepIndex) {
			qc.finishedStates = append(qc.finishedStates, *state)
			qc.states = append(qc.states[:i], qc.states[i+1:]...)
			continue
		}

		i++
	}
}

// checkFinishedStates moves completed states to finishedStates.
func (qc *QueryCursor) checkFinishedStates() {
	i := 0
	for i < len(qc.states) {
		state := &qc.states[i]

		// Check if the pattern is done based on depth.
		step := &qc.query.steps[state.stepIndex]
		if step.depth != patternDoneMarker {
			// Check if we've ascended past this state's expected depth.
			expectedDepth := state.startDepth + step.depth
			if uint32(expectedDepth) > qc.depth+1 {
				// We've ascended past where this step would match — check if
				// there are alternatives or if this state is dead.
				if step.isLastChild() || (step.depth > 0 && uint32(state.startDepth+step.depth) > qc.depth+1) {
					// Pattern expected more children; mark dead.
					state.dead = true
					qc.capturePool.release(state.captureListID)
					qc.states = append(qc.states[:i], qc.states[i+1:]...)
					continue
				}
			}
		}

		if qc.isStepDone(state.stepIndex) {
			qc.finishedStates = append(qc.finishedStates, *state)
			qc.states = append(qc.states[:i], qc.states[i+1:]...)
			continue
		}
		i++
	}
}

// stepMatchesNode checks if a query step matches a tree node.
func (qc *QueryCursor) stepMatchesNode(step *queryStep, node Node, sym Symbol, isNamed bool) bool {
	// Wildcard matches everything (or only named nodes if step.isNamed).
	if step.symbol == wildcardSymbol {
		if step.isNamed() && !isNamed {
			return false
		}
		return true
	}

	// Check symbol match.
	if step.symbol != sym {
		return false
	}

	// Check named/anonymous constraint.
	if step.isNamed() != isNamed {
		return false
	}

	return true
}

// isStepDone checks if a step index points to a DONE marker.
func (qc *QueryCursor) isStepDone(stepIndex uint16) bool {
	if int(stepIndex) >= len(qc.query.steps) {
		return true
	}
	return qc.query.steps[stepIndex].depth == patternDoneMarker
}

// currentFieldID returns the field ID of the current cursor position.
func (qc *QueryCursor) currentFieldID() FieldID {
	// The TreeCursor doesn't currently expose field IDs directly.
	// This would need a CurrentFieldID() method on TreeCursor.
	// For now, return 0 (no field).
	return 0
}
