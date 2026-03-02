package query

import (
	st "github.com/treesitter-go/treesitter/internal/subtree"
	itree "github.com/treesitter-go/treesitter/internal/tree"
)

// Local type aliases for tree types.
type (
	TreeCursor      = itree.TreeCursor
	TreeCursorEntry = itree.TreeCursorEntry
)

// Re-import functions from subtree and tree packages.
var (
	NewTreeCursor   = itree.NewTreeCursor
	IsVisible       = st.IsVisible
	GetProductionID = st.GetProductionID
)

// queryState tracks an in-progress pattern match in the QueryCursor.
type queryState struct {
	id                    uint32
	captureListID         uint32
	startDepth            uint16
	stepIndex             uint16
	patternIndex          uint16
	seekingImmediateMatch bool
	dead                  bool
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
	query          *Query
	cursor         TreeCursor
	states         []queryState
	finishedStates []queryState
	capturePool    captureListPool
	depth          uint32
	startByte      uint32
	endByte        uint32
	nextStateID    uint32
	ascending      bool
	halted         bool
	didExceedLimit bool
	maxStartDepth  uint32
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
func (qc *QueryCursor) NextCapture() (QueryMatch, uint32, bool) {
	match, ok := qc.NextMatch()
	if !ok {
		return QueryMatch{}, 0, false
	}
	return match, 0, true
}

func (qc *QueryCursor) advance() bool {
	for {
		if qc.halted {
			return false
		}

		if qc.ascending {
			qc.checkFinishedStates()

			if qc.cursor.GotoNextSibling() {
				qc.ascending = false
			} else if qc.cursor.GotoParent() {
				if qc.depth > 0 {
					qc.depth--
				}
				continue
			} else {
				qc.halted = true
				qc.checkFinishedStates()
				return len(qc.finishedStates) > 0
			}
		}

		node := qc.cursor.CurrentNode()
		if node.IsNull() {
			qc.halted = true
			return len(qc.finishedStates) > 0
		}

		nodeStart := node.StartByte()
		nodeEnd := node.EndByte()
		if nodeEnd <= qc.startByte {
			if !qc.cursor.GotoNextSibling() {
				qc.ascending = true
			}
			continue
		}
		if nodeStart >= qc.endByte {
			qc.ascending = true
			continue
		}

		if qc.depth <= qc.maxStartDepth {
			qc.introduceNewStates(node)
		}

		qc.advanceStates(node)

		if qc.cursor.GotoFirstChild() {
			qc.depth++
		} else {
			qc.ascending = true
		}

		if len(qc.finishedStates) > 0 {
			return true
		}
	}
}

func (qc *QueryCursor) introduceNewStates(node Node) {
	sym := node.Symbol()
	isNamed := node.IsNamed()

	for _, entry := range qc.findMatchingPatternEntries(sym) {
		step := &qc.query.steps[entry.stepIndex]

		if !qc.stepMatchesNode(step, node, sym, isNamed) {
			continue
		}

		listID, ok := qc.capturePool.acquire()
		if !ok {
			qc.didExceedLimit = true
			continue
		}

		for i := 0; i < maxStepCaptureCount; i++ {
			if step.captureIDs[i] == noneValue {
				break
			}
			qc.capturePool.addCapture(listID, QueryCapture{
				Node:  node,
				Index: uint32(step.captureIDs[i]),
			})
		}

		nextStepIdx := entry.stepIndex + 1
		if int(nextStepIdx) < len(qc.query.steps) {
			ns := &qc.query.steps[nextStepIdx]
			if ns.isPassThrough() && (ns.alternativeIndex == noneValue || ns.alternativeIndex > nextStepIdx) {
				pat := qc.query.patterns[entry.patternIndex]
				nextStepIdx = uint16(pat.stepsOffset + pat.stepsLength - 1)
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

		if qc.isStepDone(state.stepIndex) {
			qc.finishedStates = append(qc.finishedStates, state)
		} else {
			qc.states = append(qc.states, state)
		}
	}
}

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

		expectedDepth := qc.states[i].startDepth + step.depth
		if uint32(expectedDepth) != qc.depth {
			i++
			continue
		}

		if step.isPassThrough() {
			if step.alternativeIndex != noneValue {
				altListID, ok := qc.capturePool.clone(qc.states[i].captureListID)
				if ok {
					altState := qc.states[i]
					altState.stepIndex = step.alternativeIndex
					altState.captureListID = altListID
					altState.id = qc.nextStateID
					qc.nextStateID++
					qc.states = append(qc.states, altState)
				}
			}
			qc.states[i].stepIndex++
			continue
		}

		if step.isDeadEnd() {
			if step.alternativeIndex != noneValue {
				qc.states[i].stepIndex = step.alternativeIndex
			} else {
				qc.states[i].dead = true
			}
			continue
		}

		if !qc.stepMatchesNode(step, node, sym, isNamed) {
			if qc.states[i].seekingImmediateMatch {
				qc.states[i].dead = true
			}
			i++
			continue
		}

		if step.field != 0 {
			fieldID := qc.currentFieldID()
			if fieldID != step.field {
				i++
				continue
			}
		}

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

		if step.alternativeIndex != noneValue && !step.isDeadEnd() {
			altListID, ok := qc.capturePool.clone(qc.states[i].captureListID)
			if ok {
				altState := qc.states[i]
				altState.stepIndex = step.alternativeIndex
				altState.captureListID = altListID
				altState.id = qc.nextStateID
				qc.nextStateID++
				qc.states = append(qc.states, altState)
				step = &qc.query.steps[qc.states[i].stepIndex]
			}
		}

		for ci := 0; ci < maxStepCaptureCount; ci++ {
			if step.captureIDs[ci] == noneValue {
				break
			}
			qc.capturePool.addCapture(qc.states[i].captureListID, QueryCapture{
				Node:  node,
				Index: uint32(step.captureIDs[ci]),
			})
		}

		qc.states[i].stepIndex++
		if int(qc.states[i].stepIndex) < len(qc.query.steps) {
			ns := &qc.query.steps[qc.states[i].stepIndex]
			if ns.isPassThrough() && (ns.alternativeIndex == noneValue || ns.alternativeIndex > qc.states[i].stepIndex) {
				pat := qc.query.patterns[qc.states[i].patternIndex]
				qc.states[i].stepIndex = uint16(pat.stepsOffset + pat.stepsLength - 1)
			}
		}
		qc.states[i].seekingImmediateMatch = step.isImmediate()

		if qc.isStepDone(qc.states[i].stepIndex) {
			qc.finishedStates = append(qc.finishedStates, qc.states[i])
			qc.states = append(qc.states[:i], qc.states[i+1:]...)
			continue
		}

		i++
	}
}

func (qc *QueryCursor) checkFinishedStates() {
	i := 0
	for i < len(qc.states) {
		step := &qc.query.steps[qc.states[i].stepIndex]
		if step.depth != patternDoneMarker {
			expectedDepth := qc.states[i].startDepth + step.depth
			if uint32(expectedDepth) > qc.depth+1 {
				skipped := false
				for hops := 0; hops < len(qc.query.steps); hops++ {
					s := &qc.query.steps[qc.states[i].stepIndex]
					if s.alternativeIndex == noneValue || s.isPassThrough() || s.isDeadEnd() {
						break
					}
					qc.states[i].stepIndex = s.alternativeIndex
					skipped = true
					s2 := &qc.query.steps[qc.states[i].stepIndex]
					if s2.depth == patternDoneMarker || uint32(qc.states[i].startDepth+s2.depth) <= qc.depth+1 {
						break
					}
				}
				if skipped {
					continue
				}

				step = &qc.query.steps[qc.states[i].stepIndex]
				if step.isLastChild() || (step.depth > 0 && uint32(qc.states[i].startDepth+step.depth) > qc.depth+1) {
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

func (qc *QueryCursor) stepMatchesNode(step *queryStep, node Node, sym Symbol, isNamed bool) bool {
	if step.symbol == wildcardSymbol {
		if step.isNamed() && !isNamed {
			return false
		}
		return true
	}

	if step.symbol != sym {
		return false
	}

	if step.isNamed() != isNamed {
		return false
	}

	return true
}

func (qc *QueryCursor) findMatchingPatternEntries(sym Symbol) []patternEntry {
	pm := qc.query.patternMap
	if len(pm) == 0 {
		return nil
	}

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

func (qc *QueryCursor) isStepDone(stepIndex uint16) bool {
	if int(stepIndex) >= len(qc.query.steps) {
		return true
	}
	return qc.query.steps[stepIndex].depth == patternDoneMarker
}

func (qc *QueryCursor) currentFieldID() FieldID {
	stack := qc.cursor.Stack
	if len(stack) < 2 {
		return 0
	}
	arena := qc.cursor.Tree.Arena()
	childStructuralIndex := stack[len(stack)-1].StructuralChildIndex

	for depth := len(stack) - 2; depth >= 0; depth-- {
		parentEntry := &stack[depth]
		if !IsVisible(parentEntry.Subtree, arena) && depth > 0 {
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
