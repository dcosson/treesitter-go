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
	// Uses binary search since patternMap is sorted by symbol.
	// Pattern map entries point to resolved content steps (not pass-through/dead-end).
	for _, entry := range qc.findMatchingPatternEntries(sym) {
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

		// Compute next step. If the next step is an alternation branch pass-through
		// (pointing forward to another alternative), skip all remaining alternatives
		// and jump to the pattern's DONE marker. Quantifier repeat pass-throughs
		// (pointing backward) are NOT skipped — they need normal processing.
		nextStepIdx := entry.stepIndex + 1
		if int(nextStepIdx) < len(qc.query.steps) {
			ns := &qc.query.steps[nextStepIdx]
			if ns.isPassThrough() && (ns.alternativeIndex == noneValue || ns.alternativeIndex > nextStepIdx) {
				pat := qc.query.patterns[entry.patternIndex]
				nextStepIdx = uint16(pat.stepsOffset + pat.stepsLength - 1) // DONE marker
			}
		}

		state := queryState{
			id:            qc.nextStateID,
			captureListID: listID,
			startDepth:    uint16(qc.depth),
			stepIndex:     nextStepIdx,
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
// P1 FIX: Use index-based access throughout to avoid pointer invalidation
// when append() reallocates qc.states.
func (qc *QueryCursor) advanceStates(node Node) {
	sym := node.Symbol()
	isNamed := node.IsNamed()

	i := 0
	for i < len(qc.states) {
		if qc.states[i].dead {
			qc.capturePool.release(qc.states[i].captureListID)
			qc.states = append(qc.states[:i], qc.states[i+1:]...)
			continue
		}

		step := &qc.query.steps[qc.states[i].stepIndex]

		// Check depth: the step's depth should match the current relative depth.
		expectedDepth := qc.states[i].startDepth + step.depth
		if uint32(expectedDepth) != qc.depth {
			i++
			continue
		}

		// Handle pass-through steps (from alternation/quantifier).
		if step.isPassThrough() {
			// Split: one state continues to next step, one goes to alternative.
			if step.alternativeIndex != noneValue {
				altListID, ok := qc.capturePool.clone(qc.states[i].captureListID)
				if ok {
					altState := qc.states[i]
					altState.stepIndex = step.alternativeIndex
					altState.captureListID = altListID
					altState.id = qc.nextStateID
					qc.nextStateID++
					qc.states = append(qc.states, altState)
					// Re-read step since append may have reallocated.
					step = &qc.query.steps[qc.states[i].stepIndex]
				}
			}
			qc.states[i].stepIndex++
			continue
		}

		// Handle dead-end steps (redirect only).
		if step.isDeadEnd() {
			if step.alternativeIndex != noneValue {
				qc.states[i].stepIndex = step.alternativeIndex
			} else {
				qc.states[i].dead = true
			}
			continue
		}

		// Try to match this step against the current node.
		if !qc.stepMatchesNode(step, node, sym, isNamed) {
			if qc.states[i].seekingImmediateMatch {
				qc.states[i].dead = true
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

		// P2 FIX: Clone capture list for alternatives BEFORE adding captures,
		// so the alternative path doesn't get this step's captures.
		if step.alternativeIndex != noneValue && !step.isDeadEnd() {
			altListID, ok := qc.capturePool.clone(qc.states[i].captureListID)
			if ok {
				altState := qc.states[i]
				altState.stepIndex = step.alternativeIndex
				altState.captureListID = altListID
				altState.id = qc.nextStateID
				qc.nextStateID++
				qc.states = append(qc.states, altState)
				// Re-read step since append may have reallocated.
				step = &qc.query.steps[qc.states[i].stepIndex]
			}
		}

		// Match! Add captures (after cloning for alternatives).
		for ci := 0; ci < maxStepCaptureCount; ci++ {
			if step.captureIDs[ci] == noneValue {
				break
			}
			qc.capturePool.addCapture(qc.states[i].captureListID, QueryCapture{
				Node:  node,
				Index: uint32(step.captureIDs[ci]),
			})
		}

		// Advance the step. If the next step is an alternation branch pass-through
		// (pointing forward), skip to the pattern's DONE marker. Quantifier repeat
		// pass-throughs (pointing backward) are processed normally.
		qc.states[i].stepIndex++
		if int(qc.states[i].stepIndex) < len(qc.query.steps) {
			ns := &qc.query.steps[qc.states[i].stepIndex]
			if ns.isPassThrough() && (ns.alternativeIndex == noneValue || ns.alternativeIndex > qc.states[i].stepIndex) {
				pat := qc.query.patterns[qc.states[i].patternIndex]
				qc.states[i].stepIndex = uint16(pat.stepsOffset + pat.stepsLength - 1)
			}
		}
		qc.states[i].seekingImmediateMatch = step.isImmediate()

		// Check if pattern is done.
		if qc.isStepDone(qc.states[i].stepIndex) {
			qc.finishedStates = append(qc.finishedStates, qc.states[i])
			qc.states = append(qc.states[:i], qc.states[i+1:]...)
			continue
		}

		i++
	}
}

// checkFinishedStates moves completed states to finishedStates.
// P2.5 FIX: Uses index-based access (not pointer-to-element) for safety.
func (qc *QueryCursor) checkFinishedStates() {
	i := 0
	for i < len(qc.states) {
		// Check if the pattern is done based on depth.
		step := &qc.query.steps[qc.states[i].stepIndex]
		if step.depth != patternDoneMarker {
			// Check if we've ascended past this state's expected depth.
			expectedDepth := qc.states[i].startDepth + step.depth
			if uint32(expectedDepth) > qc.depth+1 {
				// We've ascended past where this step would match.
				// For optional steps (with alternativeIndex), follow the alternative
				// chain. P2.6 FIX: Limit iterations to prevent infinite loops.
				skipped := false
				for hops := 0; hops < len(qc.query.steps); hops++ {
					s := &qc.query.steps[qc.states[i].stepIndex]
					if s.alternativeIndex == noneValue || s.isPassThrough() || s.isDeadEnd() {
						break
					}
					qc.states[i].stepIndex = s.alternativeIndex
					skipped = true
					// Re-check depth with new step.
					s2 := &qc.query.steps[qc.states[i].stepIndex]
					if s2.depth == patternDoneMarker || uint32(qc.states[i].startDepth+s2.depth) <= qc.depth+1 {
						break
					}
				}
				if skipped {
					continue // Re-check this state from the top.
				}

				step = &qc.query.steps[qc.states[i].stepIndex]
				if step.isLastChild() || (step.depth > 0 && uint32(qc.states[i].startDepth+step.depth) > qc.depth+1) {
					// Pattern expected more children; mark dead.
					qc.states[i].dead = true
					qc.capturePool.release(qc.states[i].captureListID)
					qc.states = append(qc.states[:i], qc.states[i+1:]...)
					continue
				}
			}
		}

		if qc.isStepDone(qc.states[i].stepIndex) {
			qc.finishedStates = append(qc.finishedStates, qc.states[i])
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

// findMatchingPatternEntries returns pattern entries whose first step could
// match the given symbol. Uses binary search on the sorted patternMap, plus
// includes wildcard patterns (symbol=0) that always match.
func (qc *QueryCursor) findMatchingPatternEntries(sym Symbol) []patternEntry {
	pm := qc.query.patternMap
	if len(pm) == 0 {
		return nil
	}

	// Collect wildcard entries (symbol=0, at the front since map is sorted).
	var result []patternEntry
	wildcardEnd := 0
	for wildcardEnd < len(pm) {
		step := &qc.query.steps[pm[wildcardEnd].stepIndex]
		if step.symbol != wildcardSymbol {
			break
		}
		wildcardEnd++
	}
	if wildcardEnd > 0 {
		result = append(result, pm[:wildcardEnd]...)
	}

	// Binary search for the target symbol in the non-wildcard portion.
	lo, hi := wildcardEnd, len(pm)
	for lo < hi {
		mid := lo + (hi-lo)/2
		midSym := qc.query.steps[pm[mid].stepIndex].symbol
		if midSym < sym {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	// Collect all entries with matching symbol.
	for lo < len(pm) {
		step := &qc.query.steps[pm[lo].stepIndex]
		if step.symbol != sym {
			break
		}
		result = append(result, pm[lo])
		lo++
	}

	return result
}

// isStepDone checks if a step index points to a DONE marker.
func (qc *QueryCursor) isStepDone(stepIndex uint16) bool {
	if int(stepIndex) >= len(qc.query.steps) {
		return true
	}
	return qc.query.steps[stepIndex].depth == patternDoneMarker
}

// currentFieldID returns the field ID of the current cursor position.
// It looks up the parent node's production ID, then finds the field map entry
// matching the current child's structural child index.
func (qc *QueryCursor) currentFieldID() FieldID {
	stack := qc.cursor.Stack
	if len(stack) < 2 {
		return 0
	}
	// Walk up through hidden nodes to find the visible parent and child indices.
	arena := qc.cursor.Tree.Arena()
	childStructuralIndex := stack[len(stack)-1].StructuralChildIndex

	// Find the nearest visible ancestor with a production ID.
	for depth := len(stack) - 2; depth >= 0; depth-- {
		parentEntry := &stack[depth]
		if !IsVisible(parentEntry.Subtree, arena) && depth > 0 {
			// Hidden node: accumulate structuralChildIndex and keep going up.
			childStructuralIndex += parentEntry.StructuralChildIndex
			continue
		}
		prodID := GetProductionID(parentEntry.Subtree, arena)
		fieldEntries := qc.query.language.FieldMapForProduction(prodID)
		for _, entry := range fieldEntries {
			if uint16(childStructuralIndex) == entry.ChildIndex {
				return entry.FieldID
			}
		}
		break
	}
	return 0
}
