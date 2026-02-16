package treesitter

import (
	"bytes"
	"context"
)

// Parser is the GLR parsing engine. It drives the Lexer and Language to
// produce a parse Tree from input text. The parser supports:
//   - Standard LR(1) parsing for unambiguous states
//   - GLR (multiple stack versions) for ambiguous states
//   - Error recovery with cost-based version pruning
//   - context.Context cancellation
//   - Incremental parsing (via old tree reuse — future work)
type Parser struct {
	language *Language
	lexer    *Lexer
	stack    *Stack
	arena    *SubtreeArena

	// finishedTree holds the result after a successful parse.
	finishedTree Subtree

	// Token cache: avoids re-lexing when the parser inspects the
	// current token multiple times (e.g. across versions).
	cachedToken            Subtree
	cachedTokenState       StateID
	cachedTokenExtState    uint16
	cachedTokenPosition    Length
	cachedTokenValid       bool

	// External scanner support.
	externalScanner      ExternalScanner
	serializationBuffer  [TreeSitterSerializationBufferSize]byte

	// Error recovery state.
	acceptCount       uint32
	operationCount    uint32
	skippedErrorTrees []Subtree

	// Incremental parsing state.
	reusableNode *ReusableNode
	oldTree      *Tree

	// Cancellation check interval.
	cancellationCheckInterval uint32

	// debug enables trace output.
	debug bool
}

// Error cost constants matching C tree-sitter.
const (
	ErrorCostPerRecovery    = 500
	ErrorCostPerMissingTree = 110
	ErrorCostPerSkippedTree = 100
	ErrorCostPerSkippedLine = 30
	ErrorCostPerSkippedChar = 1

	MaxVersionCount     = 6
	MaxCostDifference   = 1800
	MaxSummaryDepth     = 16

	defaultCancellationInterval = 100
)

// NewParser creates a new Parser.
func NewParser() *Parser {
	arena := NewSubtreeArena(0)
	return &Parser{
		lexer:                     NewLexer(),
		arena:                     arena,
		stack:                     NewStack(arena),
		cancellationCheckInterval: defaultCancellationInterval,
	}
}

// SetDebug enables debug trace output.
func (p *Parser) SetDebug(on bool) {
	p.debug = on
}

// SetLanguage sets the language (grammar) for the parser.
// If the language has an external scanner factory, a new scanner is created.
func (p *Parser) SetLanguage(lang *Language) {
	p.language = lang
	p.externalScanner = nil
	if lang != nil && lang.NewExternalScanner != nil {
		p.externalScanner = lang.NewExternalScanner()
	}
}

// Language returns the parser's current language.
func (p *Parser) Language() *Language {
	return p.language
}

// Reset clears all parser state, preparing for a new parse.
func (p *Parser) Reset() {
	p.stack.Clear()
	p.finishedTree = SubtreeZero
	p.cachedToken = SubtreeZero
	p.cachedTokenPosition = LengthZero
	p.cachedTokenState = 0
	p.cachedTokenExtState = 0
	p.cachedTokenValid = false
	p.acceptCount = 0
	p.operationCount = 0
	p.skippedErrorTrees = p.skippedErrorTrees[:0]
	p.reusableNode = nil
	p.oldTree = nil
}

// Parse parses the input and returns a Tree. If ctx is cancelled, the
// parser stops and returns nil. If oldTree is non-nil, the parser performs
// incremental parsing by reusing unchanged subtrees from the old tree.
func (p *Parser) Parse(ctx context.Context, input Input, oldTree *Tree) *Tree {
	if p.language == nil {
		return nil
	}

	p.Reset()
	p.lexer.SetInput(input)

	// Set up arena: if we have an old tree, fork its arena so old subtree
	// references remain valid and new allocations go into fresh blocks.
	if oldTree != nil && oldTree.Arena() != nil {
		p.arena = oldTree.Arena().Fork()
		p.oldTree = oldTree
		p.reusableNode = NewReusableNode(oldTree.root, p.arena)
	} else {
		p.arena = NewSubtreeArena(0)
		p.oldTree = nil
		p.reusableNode = nil
	}
	p.stack = NewStack(p.arena)

	// Initialize the stack with a single version at state 1.
	// State 0 is reserved as the error/recovery state by tree-sitter convention.
	// State 1 is the start state for the grammar's top-level rule.
	p.stack.AddVersion(1, LengthZero)

	for {
		// Check for cancellation periodically.
		p.operationCount++
		if p.operationCount%p.cancellationCheckInterval == 0 {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
		}

		// Find an active version to advance.
		version := p.findActiveVersion()
		if version < 0 {
			break
		}

		// Advance this version.
		if !p.advanceVersion(StackVersion(version)) {
			// If advance failed and no versions remain, we're done.
			if p.stack.ActiveVersionCount() == 0 {
				break
			}
		}

		// Condense: merge/prune versions.
		p.condenseStack()
	}

	if p.finishedTree.IsZero() {
		return nil
	}

	return NewTree(p.finishedTree, p.language, nil, []*SubtreeArena{p.arena})
}

// ParseString is a convenience method that parses a []byte string.
// If oldTree is non-nil, incremental parsing is performed.
func (p *Parser) ParseString(ctx context.Context, source []byte, oldTree ...*Tree) *Tree {
	var old *Tree
	if len(oldTree) > 0 {
		old = oldTree[0]
	}
	return p.Parse(ctx, NewStringInput(source), old)
}

// findActiveVersion returns the index of the first active version,
// prioritizing the version with the lowest position (furthest behind).
// Returns -1 if no active versions exist.
func (p *Parser) findActiveVersion() int {
	bestVersion := -1
	var bestPosition Length

	for i := 0; i < p.stack.VersionCount(); i++ {
		v := StackVersion(i)
		if !p.stack.IsActive(v) {
			continue
		}
		pos := p.stack.Position(v)
		if bestVersion < 0 || pos.Bytes < bestPosition.Bytes {
			bestVersion = i
			bestPosition = pos
		}
	}
	return bestVersion
}

// advanceVersion advances a single stack version by one token.
// Returns true if the version was successfully advanced, false if it was halted.
func (p *Parser) advanceVersion(version StackVersion) bool {
	state := p.stack.State(version)
	position := p.stack.Position(version)

	// Step 1: Try to reuse a node from the old tree (incremental parsing).
	// Only attempt reuse when there's a single active version (no GLR ambiguity).
	allowReuse := p.reusableNode != nil && p.stack.ActiveVersionCount() == 1
	var token Subtree
	reused := false

	if allowReuse {
		token, reused = p.tryReuseNode(version, state, position)
	}

	// If we reused a non-terminal subtree (one with children), push it
	// directly using the GOTO state transition. Non-terminals can't go
	// through the normal action loop because the action table only has
	// SHIFT/REDUCE entries for terminal symbols. Non-terminals use GOTO.
	if reused {
		children := GetChildren(token, p.arena)
		if children != nil && len(children) > 0 {
			sym := GetSymbol(token, p.arena)
			gotoState := p.language.nextState(state, sym)
			if gotoState != 0 {
				tokenPadding := GetPadding(token, p.arena)
				tokenSize := GetSize(token, p.arena)
				newPosition := LengthAdd(LengthAdd(position, tokenPadding), tokenSize)
				p.stack.Push(version, gotoState, token, false, newPosition)
				if HasExternalTokens(token, p.arena) {
					p.stack.SetLastExternalToken(version, token)
				}
				p.cachedTokenValid = false
				return true
			}
			// No valid GOTO — can't reuse; fall through to lex.
			reused = false
		}
	}

	// Step 2: If no reuse, lex a token.
	if !reused {
		token = p.lexToken(version, state, position)
	}

	tokenSymbol := GetSymbol(token, p.arena)

	// Inner reduce loop: keep reducing until we can shift or accept.
	// This avoids re-lexing after every reduce.
	for reduceCount := 0; reduceCount < 1000; reduceCount++ {
		// Look up parse actions.
		state = p.stack.State(version)
		entry := p.language.tableEntry(state, tokenSymbol)

		if entry.ActionCount == 0 {
			// No valid action — try error recovery.
			return p.handleError(version, token)
		}

		action := entry.Actions[0]

		// Handle additional actions (GLR ambiguity) by splitting.
		for i := 1; i < int(entry.ActionCount); i++ {
			extraAction := entry.Actions[i]
			// Skip repetition shifts — these are markers for the parser's
			// repeat optimization and should not be executed as actual shifts.
			if extraAction.Type == ParseActionTypeShift && extraAction.ShiftRepetition {
				continue
			}
			splitVersion := p.stack.Split(version)
			if splitVersion < 0 {
				continue
			}
			switch extraAction.Type {
			case ParseActionTypeShift:
				p.doShift(splitVersion, extraAction, token)
			case ParseActionTypeReduce:
				p.doReduce(splitVersion, extraAction)
			case ParseActionTypeAccept:
				p.doAccept(splitVersion)
			case ParseActionTypeRecover:
				if tokenSymbol == SymbolEnd {
					p.stack.Halt(splitVersion)
				} else {
					p.doShift(splitVersion, extraAction, token)
				}
			}
		}
		// Execute the primary action.
		// Skip repetition shifts in primary action too.
		if action.Type == ParseActionTypeShift && action.ShiftRepetition {
			continue
		}
		switch action.Type {
		case ParseActionTypeShift:
			p.doShift(version, action, token)
			return true
		case ParseActionTypeReduce:
			p.doReduce(version, action)
			// Continue the inner loop to check the new state.
			continue
		case ParseActionTypeAccept:
			p.doAccept(version)
			return true
		case ParseActionTypeRecover:
			// At EOF, a RECOVER action cannot make progress (no more input
			// to skip). Halt the version — it's stuck in error recovery.
			if tokenSymbol == SymbolEnd {
				p.stack.Halt(version)
				return false
			}
			p.doShift(version, action, token)
			return true
		}
	}

	// Safety: too many reduces, something is wrong.
	p.stack.Halt(version)
	return false
}

// lexToken lexes the next token from the current position.
// Uses caching to avoid re-lexing across multiple stack versions.
// The version parameter is needed for external scanner state (lastExternalToken).
func (p *Parser) lexToken(version StackVersion, state StateID, position Length) Subtree {
	lexMode := p.language.LexModes[state]

	// Check cache: only valid if same lex state, external lex state, and position.
	if p.cachedTokenValid &&
		p.cachedTokenState == StateID(lexMode.LexState) &&
		p.cachedTokenExtState == lexMode.ExternalLexState &&
		p.cachedTokenPosition.Bytes == position.Bytes {
		return p.cachedToken
	}

	foundExternalToken := false
	var externalScannerStateLen uint32
	var externalScannerStateChanged bool

	// Try external scanner first if this state enables external tokens.
	if lexMode.ExternalLexState != 0 && p.externalScanner != nil {
		p.lexer.Start(position)

		// Deserialize from the last external token for this version.
		lastExtToken := p.stack.LastExternalToken(version)
		p.externalScannerDeserialize(lastExtToken)

		// Get valid symbols for this external lex state.
		validSymbols := p.language.EnabledExternalTokens(lexMode.ExternalLexState)

		// Call the external scanner.
		if validSymbols != nil && p.externalScanner.Scan(p.lexer, validSymbols) {
			// If the scanner didn't call MarkEnd, default the token end to
			// the current position (matching AcceptToken behavior). Without
			// this, TokenEndPosition stays at Length{} (zero), causing size
			// underflow when the token is at a non-zero position.
			if !p.lexer.markEndCalled {
				p.lexer.TokenEndPosition = p.lexer.currentPosition
			}

			// Reject zero-width external tokens where the scanner didn't
			// call MarkEnd and didn't advance. These indicate the scanner
			// matched a default/fallback rule (e.g. EmptyValue on whitespace)
			// without actually consuming input. Accepting them causes infinite
			// loops because zero-width tokens don't advance position.
			if !p.lexer.markEndCalled && p.lexer.currentPosition.Bytes == position.Bytes {
				// Fall through to internal lex.
			} else {
				// Scanner recognized a token. Serialize the new state.
				externalScannerStateLen = p.externalScannerSerialize()

				// Check if state changed from previous external token.
				externalScannerStateChanged = !ExternalScannerStateEqual(
					lastExtToken, p.arena,
					p.serializationBuffer[:externalScannerStateLen],
					externalScannerStateLen,
				)

				// Map the external token index to a grammar symbol.
				extTokenIndex := p.lexer.ResultSymbol
				if int(extTokenIndex) < len(p.language.ExternalSymbolMap) {
					p.lexer.ResultSymbol = p.language.ExternalSymbolMap[extTokenIndex]
				}

				foundExternalToken = true
			}
		}
	}

	// If no external token, run the internal lex function.
	if !foundExternalToken {
		p.lexer.Start(position)

		found := false
		if p.language.LexFn != nil {
			found = p.language.LexFn(p.lexer, StateID(lexMode.LexState))
		}

		// If the result is the keyword capture token, try keyword lex.
		// Keyword lex always starts at state 0 (initial keyword DFA state).
		// The keyword match is only accepted if the keyword lex consumed the
		// ENTIRE identifier text (current position == original token end).
		if found && p.language.KeywordLexFn != nil && p.lexer.ResultSymbol == p.language.KeywordCaptureToken {
			keywordEndPos := p.lexer.TokenEndPosition
			origSymbol := p.lexer.ResultSymbol

			p.lexer.Start(position)
			if p.language.KeywordLexFn(p.lexer, 0) &&
				p.lexer.CurrentPosition() == keywordEndPos {
				p.lexer.TokenEndPosition = keywordEndPos
			} else {
				p.lexer.ResultSymbol = origSymbol
				p.lexer.TokenEndPosition = keywordEndPos
			}
		}

		if !found && p.lexer.EOF() {
			// At EOF — produce end-of-input token.
			p.lexer.MarkEnd()
			p.lexer.AcceptToken(SymbolEnd)
		} else if !found {
			// No token found — advance by one character and report error.
			if !p.lexer.EOF() {
				p.lexer.Advance(false)
			}
			p.lexer.MarkEnd()
			p.lexer.AcceptToken(SymbolError)
		}
	}

	// Compute padding and size.
	tokenStart := p.lexer.TokenStartPosition()
	tokenEnd := p.lexer.TokenEndPosition

	padding := LengthSub(tokenStart, position)
	size := LengthSub(tokenEnd, tokenStart)

	symbol := p.lexer.ResultSymbol

	// Create the leaf subtree for this token.
	isKeyword := false
	dependsOnColumn := padding.Point.Row > 0

	token := NewLeafSubtree(
		p.arena,
		symbol,
		padding,
		size,
		state,
		foundExternalToken,
		dependsOnColumn,
		isKeyword,
		p.language,
	)

	// Set lookahead bytes: how far the lexer scanned past the token end.
	// This is needed for incremental parsing: if an edit falls within the
	// lookahead range, the token must be re-lexed.
	if !token.IsInline() {
		lookahead := p.lexer.CurrentPosition().Bytes - tokenEnd.Bytes
		if lookahead > 0 {
			p.arena.Get(token).LookaheadBytes = lookahead
		}
	}

	// If this was an external token, attach the serialized scanner state.
	if foundExternalToken {
		SetExternalScannerState(
			token, p.arena,
			p.serializationBuffer[:externalScannerStateLen],
		)
		if externalScannerStateChanged && !token.IsInline() {
			p.arena.Get(token).SetFlag(SubtreeFlagHasExternalScannerStateChange, true)
		}
	}

	// Cache the token.
	p.cachedToken = token
	p.cachedTokenPosition = position
	p.cachedTokenState = StateID(lexMode.LexState)
	p.cachedTokenExtState = lexMode.ExternalLexState
	p.cachedTokenValid = true

	return token
}

// tryReuseNode attempts to reuse a subtree from the old tree at the current
// parse position. Returns (subtree, true) if a reusable node was found,
// or (SubtreeZero, false) if the parser should fall back to lexing.
func (p *Parser) tryReuseNode(version StackVersion, state StateID, position Length) (Subtree, bool) {
	rn := p.reusableNode

	// Advance the reusable node iterator to the current position.
	rn.AdvanceToByteOffset(position.Bytes)

	for !rn.Done() {
		candidate := rn.Tree()
		byteOffset := rn.ByteOffset()

		// Skip past nodes that end before the current position.
		candidatePadding := GetPadding(candidate, p.arena)
		candidateSize := GetSize(candidate, p.arena)
		candidateEnd := byteOffset + candidatePadding.Bytes + candidateSize.Bytes

		if candidateEnd <= position.Bytes {
			rn.Advance()
			continue
		}

		// No reusable node at this position — node starts after current position.
		if byteOffset > position.Bytes {
			break
		}

		// Node starts at current position. Check reusability.
		sym := GetSymbol(candidate, p.arena)

		// Check basic reusability conditions.
		if HasChanges(candidate, p.arena) ||
			sym == SymbolError || sym == SymbolErrorRepeat ||
			IsMissing(candidate, p.arena) ||
			IsFragileLeft(candidate, p.arena) || IsFragileRight(candidate, p.arena) {
			// Not reusable — descend into children.
			rn.Descend()
			continue
		}

		// For non-terminal subtrees (with children), check GOTO validity.
		children := GetChildren(candidate, p.arena)
		if children != nil && len(children) > 0 {
			gotoState := p.language.nextState(state, sym)
			if gotoState == 0 {
				// No GOTO for this non-terminal in current state. Descend.
				rn.Descend()
				continue
			}
		} else {
			// Terminal/leaf: check the action table and lex mode compatibility.
			leafSym := sym
			var firstLeaf FirstLeaf
			if !candidate.IsInline() {
				firstLeaf = GetFirstLeaf(candidate, p.arena)
				leafSym = firstLeaf.Symbol
			}
			entry := p.language.tableEntry(state, leafSym)
			if entry.ActionCount == 0 || !entry.Reusable {
				rn.Advance()
				break
			}

			// Check lex mode compatibility.
			if !candidate.IsInline() && firstLeaf.ParseState != 0 {
				currentLexMode := p.language.LexModes[state]
				origLexMode := p.language.LexModes[StateID(firstLeaf.ParseState)]
				if origLexMode.LexState != currentLexMode.LexState {
					rn.Advance()
					break
				}
			}
		}

		// Check external scanner state match.
		if p.externalScanner != nil {
			lastExtToken := p.stack.LastExternalToken(version)
			candidateExtState := GetExternalScannerState(candidate, p.arena)
			lastExtState := GetExternalScannerState(lastExtToken, p.arena)
			if !bytes.Equal(candidateExtState, lastExtState) {
				rn.Descend()
				continue
			}
		}

		// Reuse! Advance the iterator past this node.
		rn.Advance()
		return candidate, true
	}

	return SubtreeZero, false
}

// doShift pushes a token onto the stack and transitions to a new state.
func (p *Parser) doShift(version StackVersion, action ParseActionEntry, token Subtree) {
	state := action.ShiftState

	// Mark extra tokens. For SHIFT_EXTRA, stay in the current state.
	if action.ShiftExtra {
		token = SetExtra(token, p.arena)
		state = p.stack.State(version)
	}

	position := p.stack.Position(version)
	tokenPadding := GetPadding(token, p.arena)
	tokenSize := GetSize(token, p.arena)
	newPosition := LengthAdd(LengthAdd(position, tokenPadding), tokenSize)

	p.stack.Push(version, state, token, false, newPosition)

	// Track external tokens for scanner state restoration.
	if HasExternalTokens(token, p.arena) {
		p.stack.SetLastExternalToken(version, token)
	}

	// Invalidate token cache after consuming.
	p.cachedTokenValid = false
}

// doReduce pops children from the stack and creates an internal node.
func (p *Parser) doReduce(version StackVersion, action ParseActionEntry) {
	childCount := uint32(action.ReduceChildCount)
	symbol := action.ReduceSymbol
	productionID := action.ReduceProdID
	dynPrec := action.ReduceDynPrec

	// Pop children.
	results := p.stack.Pop(version, childCount)
	if len(results) == 0 {
		p.stack.Halt(version)
		return
	}

	// For the primary path (first result), create the internal node.
	primary := results[0]

	// The children come in stack order (top first). Reverse for tree order.
	children := make([]Subtree, len(primary.subtrees))
	for i, s := range primary.subtrees {
		children[len(children)-1-i] = s
	}

	// Create the internal node.
	node := NewNodeSubtree(p.arena, symbol, children, productionID, p.language)
	SummarizeChildren(node, p.arena, p.language)

	// Apply dynamic precedence.
	if dynPrec != 0 && !node.IsInline() {
		data := p.arena.Get(node)
		data.DynamicPrecedence += int32(dynPrec)
	}

	// Look up the goto state.
	baseState := p.stack.State(version)
	gotoState := p.language.nextState(baseState, symbol)

	// Compute new position.
	position := p.stack.Position(version)
	nodePadding := GetPadding(node, p.arena)
	nodeSize := GetSize(node, p.arena)
	newPosition := LengthAdd(LengthAdd(position, nodePadding), nodeSize)

	p.stack.Push(version, gotoState, node, false, newPosition)

	// Handle additional pop paths (GLR ambiguity).
	// Each alt path may have a different base state (from merged stack DAG),
	// so we must create a version pointing at each path's base node and
	// compute goto state and position per-path.
	for i := 1; i < len(results); i++ {
		path := results[i]
		altChildren := make([]Subtree, len(path.subtrees))
		for j, s := range path.subtrees {
			altChildren[len(altChildren)-1-j] = s
		}
		altNode := NewNodeSubtree(p.arena, symbol, altChildren, productionID, p.language)
		SummarizeChildren(altNode, p.arena, p.language)

		// Compute goto state from this path's base state.
		altBaseState := path.node.state
		altGotoState := p.language.nextState(altBaseState, symbol)

		// Compute position from this path's base position.
		altBasePosition := path.node.position
		altNodePadding := GetPadding(altNode, p.arena)
		altNodeSize := GetSize(altNode, p.arena)
		altNewPosition := LengthAdd(LengthAdd(altBasePosition, altNodePadding), altNodeSize)

		// Create a new version pointing at this path's base node (not the
		// primary's pushed result). This is critical for correct goto states.
		altVersion := p.stack.ForkAtNode(path.node)
		if altVersion >= 0 {
			p.stack.Push(altVersion, altGotoState, altNode, false, altNewPosition)
		}
	}
}

// doAccept marks a version as accepted and stores the finished tree.
func (p *Parser) doAccept(version StackVersion) {
	tree := p.stack.TopSubtree(version)
	if !tree.IsZero() {
		p.finishedTree = tree
	}
	p.acceptCount++
	p.stack.Halt(version)
}

// handleError attempts error recovery for a version that encountered
// an invalid token.
func (p *Parser) handleError(version StackVersion, token Subtree) bool {
	state := p.stack.State(version)

	// At end-of-input, skipping won't advance position (zero-size token).
	// If we already have a finished tree, just halt this error version.
	// Otherwise, force-accept what we have.
	tokenSymbol := GetSymbol(token, p.arena)
	if tokenSymbol == SymbolEnd {
		if !p.finishedTree.IsZero() {
			p.stack.Halt(version)
			return false
		}
		p.doAccept(version)
		return true
	}

	// Strategy 1: Try all possible reductions from current state.
	// This creates split versions with reductions applied. If any split can
	// shift the lookahead, we halt the original version (it's superseded by
	// the splits) and invalidate the token cache so the splits re-lex fresh.
	recovered := p.doAllPotentialReductions(version, token)
	if recovered {
		p.stack.Halt(version)
		p.cachedTokenValid = false
		return true
	}

	// Strategy 2: Skip the current token.
	tokenSize := GetSize(token, p.arena)
	skipCost := ErrorCostPerSkippedTree + ErrorCostPerSkippedChar*tokenSize.Bytes

	if p.stack.VersionCount() < MaxVersionCount {
		skipVersion := p.stack.Split(version)
		if skipVersion >= 0 {
			errNode := p.createErrorNode([]Subtree{token})
			position := p.stack.Position(skipVersion)
			tokenPadding := GetPadding(token, p.arena)
			newPosition := LengthAdd(LengthAdd(position, tokenPadding), tokenSize)
			p.stack.Push(skipVersion, state, errNode, false, newPosition)
			p.cachedTokenValid = false

			p.stack.AddErrorCost(skipVersion, skipCost)
		}
	}

	// Strategy 3: Try inserting missing tokens.
	recovered = p.tryMissingTokens(version, token)

	if recovered {
		// Missing token recovery created split versions that supersede this one.
		// Halt the original version — it has no valid action for the lookahead.
		p.stack.Halt(version)
		p.cachedTokenValid = false
		return true
	}

	// If no recovery and multiple versions exist, halt this one.
	if p.stack.ActiveVersionCount() > 1 {
		p.stack.Halt(version)
		return false
	}

	// Last resort: skip the token on the current version (only version left).
	position := p.stack.Position(version)
	tokenPadding := GetPadding(token, p.arena)
	newPosition := LengthAdd(LengthAdd(position, tokenPadding), tokenSize)
	errNode := p.createErrorNode([]Subtree{token})
	p.stack.Push(version, state, errNode, false, newPosition)
	p.cachedTokenValid = false
	p.stack.AddErrorCost(version, skipCost+ErrorCostPerRecovery)

	return true
}

// doAllPotentialReductions tries every possible reduction from the current
// state to see if any lead to a state that can shift the lookahead.
func (p *Parser) doAllPotentialReductions(version StackVersion, lookahead Subtree) bool {
	lookaheadSymbol := GetSymbol(lookahead, p.arena)
	state := p.stack.State(version)
	recovered := false

	for sym := Symbol(0); sym < Symbol(p.language.SymbolCount); sym++ {
		entry := p.language.tableEntry(state, sym)
		for i := 0; i < int(entry.ActionCount); i++ {
			action := entry.Actions[i]
			if action.Type != ParseActionTypeReduce {
				continue
			}

			if p.stack.VersionCount() >= MaxVersionCount {
				break
			}

			testVersion := p.stack.Split(version)
			if testVersion < 0 {
				continue
			}

			p.doReduce(testVersion, action)

			newState := p.stack.State(testVersion)
			newEntry := p.language.tableEntry(newState, lookaheadSymbol)
			if newEntry.ActionCount > 0 {
				recovered = true
				p.stack.AddErrorCost(testVersion, ErrorCostPerRecovery)
			} else {
				p.stack.Halt(testVersion)
			}
		}
	}

	return recovered
}

// tryMissingTokens tries hypothesizing that a token was missing.
func (p *Parser) tryMissingTokens(version StackVersion, lookahead Subtree) bool {
	state := p.stack.State(version)
	lookaheadSymbol := GetSymbol(lookahead, p.arena)
	recovered := false

	for sym := Symbol(0); sym < Symbol(p.language.SymbolCount); sym++ {
		if sym == lookaheadSymbol || sym == SymbolError || sym == SymbolErrorRepeat {
			continue
		}

		entry := p.language.tableEntry(state, sym)
		if entry.ActionCount == 0 {
			continue
		}

		firstAction := entry.Actions[0]
		if firstAction.Type != ParseActionTypeShift {
			continue
		}

		if p.stack.VersionCount() >= MaxVersionCount {
			break
		}

		missingToken := p.createMissingToken(sym)
		testVersion := p.stack.Split(version)
		if testVersion < 0 {
			continue
		}

		p.doShift(testVersion, firstAction, missingToken)

		newState := p.stack.State(testVersion)
		newEntry := p.language.tableEntry(newState, lookaheadSymbol)
		if newEntry.ActionCount > 0 {
			recovered = true
			p.stack.AddErrorCost(testVersion, ErrorCostPerMissingTree)
		} else {
			p.stack.Halt(testVersion)
		}
	}

	return recovered
}

// createErrorNode creates an ERROR node wrapping skipped tokens.
func (p *Parser) createErrorNode(skippedTokens []Subtree) Subtree {
	children := make([]Subtree, len(skippedTokens))
	copy(children, skippedTokens)

	st, data := p.arena.Alloc()
	*data = SubtreeHeapData{
		Symbol:     SymbolError,
		ChildCount: uint32(len(children)),
		Children:   children,
	}
	data.SetFlag(SubtreeFlagVisible, true)

	if len(children) > 0 {
		firstPadding := GetPadding(children[0], p.arena)
		data.Padding = firstPadding
		data.Size = computeSizeFromChildren(children, p.arena, firstPadding)
	}

	data.ErrorCost = ErrorCostPerRecovery + ErrorCostPerSkippedChar*data.Size.Bytes
	if data.Size.Point.Row > 0 {
		data.ErrorCost += ErrorCostPerSkippedLine * data.Size.Point.Row
	}

	return st
}

// createMissingToken creates a MISSING token (zero-width, for error recovery).
func (p *Parser) createMissingToken(symbol Symbol) Subtree {
	st, data := p.arena.Alloc()
	meta := p.language.SymbolMetadata[symbol]
	*data = SubtreeHeapData{
		Symbol: symbol,
	}
	data.SetFlag(SubtreeFlagVisible, meta.Visible)
	data.SetFlag(SubtreeFlagNamed, meta.Named)
	data.SetFlag(SubtreeFlagMissing, true)
	return st
}

// condenseStack merges compatible versions and prunes costly versions.
func (p *Parser) condenseStack() {
	// Merge compatible versions (same state).
	for i := 0; i < p.stack.VersionCount(); i++ {
		vi := StackVersion(i)
		if !p.stack.IsActive(vi) {
			continue
		}
		for j := i + 1; j < p.stack.VersionCount(); j++ {
			vj := StackVersion(j)
			if !p.stack.IsActive(vj) {
				continue
			}
			if p.stack.CanMerge(vi, vj) {
				p.stack.Merge(vi, vj)
			}
		}
	}

	// Prune by cost.
	if p.stack.ActiveVersionCount() > 1 {
		bestCost := uint32(0)
		first := true
		for i := 0; i < p.stack.VersionCount(); i++ {
			v := StackVersion(i)
			if !p.stack.IsActive(v) {
				continue
			}
			cost := p.stack.ErrorCost(v)
			if first || cost < bestCost {
				bestCost = cost
				first = false
			}
		}

		for i := 0; i < p.stack.VersionCount(); i++ {
			v := StackVersion(i)
			if !p.stack.IsActive(v) {
				continue
			}
			if p.stack.ErrorCost(v) > bestCost+MaxCostDifference {
				p.stack.Halt(v)
			}
		}
	}

	// Enforce max version count.
	activeCount := p.stack.ActiveVersionCount()
	if activeCount > MaxVersionCount {
		halted := 0
		for i := p.stack.VersionCount() - 1; i >= 0 && halted < activeCount-MaxVersionCount; i-- {
			v := StackVersion(i)
			if p.stack.IsActive(v) {
				p.stack.Halt(v)
				halted++
			}
		}
	}

	// Remove halted versions.
	p.stack.CompactHaltedVersions()
}

// --- External scanner helpers ---

// externalScannerSerialize serializes the external scanner's current state
// into the parser's serialization buffer. Returns the number of bytes written.
func (p *Parser) externalScannerSerialize() uint32 {
	if p.externalScanner == nil {
		return 0
	}
	return p.externalScanner.Serialize(p.serializationBuffer[:])
}

// externalScannerDeserialize restores the external scanner's state from
// the given external token subtree. If the subtree is zero, deserializes
// with empty data (default/initial state).
func (p *Parser) externalScannerDeserialize(externalToken Subtree) {
	if p.externalScanner == nil {
		return
	}
	state := GetExternalScannerState(externalToken, p.arena)
	p.externalScanner.Deserialize(state)
}
