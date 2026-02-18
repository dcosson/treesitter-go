package treesitter

import (
	"bytes"
	"context"
	"math"
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

	// ctx holds the current parse context for cancellation checks
	// in deep call paths (condenseStack, handleError, recover).
	ctx context.Context

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
	MaxCostDifference   = 18 * ErrorCostPerSkippedTree // = 1800, matches C master
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
	p.ctx = ctx

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

		// Condense: merge/prune versions, resume paused versions.
		minErrorCost := p.condenseStack()

		// If the finished tree is better than all remaining versions, stop.
		if !p.finishedTree.IsZero() && GetErrorCost(p.finishedTree, p.arena) < minErrorCost {
			p.stack.Clear()
			break
		}
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

// findActiveVersion returns the index of the active version to advance next,
// prioritizing the version with the lowest position (furthest behind).
// Returns -1 if no active versions exist.
//
// This matches C tree-sitter's ts_parser__select_next_version behavior.
// With PreferRight swaps in condenseStack, the best version naturally
// occupies the lowest index, so simple lowest-position selection works
// correctly without starvation detection.
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
			// No valid action — pause this version for error recovery.
			p.stack.Pause(version, token)
			return true
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
				p.recover(splitVersion, token)
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
			// Call recover() which handles EOF (wraps in ERROR + accepts),
			// tries summary-based popback, or falls back to skipping the token.
			// This matches C's ts_parser__advance which always calls
			// ts_parser__recover for RECOVER actions, including at EOF.
			p.recover(version, token)
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

			// Serialize the scanner state to check if it changed.
			externalScannerStateLen = p.externalScannerSerialize()
			externalScannerStateChanged = !ExternalScannerStateEqual(
				lastExtToken, p.arena,
				p.serializationBuffer[:externalScannerStateLen],
				externalScannerStateLen,
			)

			// Reject empty external tokens that would cause infinite loops.
			// Matches C tree-sitter logic: empty tokens (tokenEnd <= position)
			// with no scanner state change are rejected when the parser is in
			// error recovery (state 0) or the token is "extra" (doesn't change
			// parse state). Tokens WITH scanner state changes are always
			// accepted, even if zero-width (e.g. Ruby HeredocBodyStart,
			// Python indent/dedent).
			if p.lexer.TokenEndPosition.Bytes <= position.Bytes && !externalScannerStateChanged {
				extTokenIndex := p.lexer.ResultSymbol
				var grammarSymbol Symbol
				if int(extTokenIndex) < len(p.language.ExternalSymbolMap) {
					grammarSymbol = p.language.ExternalSymbolMap[extTokenIndex]
				}
				nextParseState := p.language.nextState(state, grammarSymbol)
				tokenIsExtra := nextParseState == state

				// Reject empty external tokens that would cause infinite loops.
				// Matches C: error_mode || !has_advanced_since_error || token_is_extra
				if state == 0 || !p.stack.HasAdvancedSinceError(version) || tokenIsExtra {
					// Fall through to internal lex — reject this empty token.
				} else {
					// Accept: not in error recovery, has advanced, and not extra.
					if int(extTokenIndex) < len(p.language.ExternalSymbolMap) {
						p.lexer.ResultSymbol = grammarSymbol
					}
					foundExternalToken = true
				}
			} else {
				// Non-empty token or scanner state changed — always accept.
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
			keywordMatched := p.language.KeywordLexFn(p.lexer, 0) &&
				p.lexer.CurrentPosition() == keywordEndPos

			if keywordMatched {
				keywordSymbol := p.lexer.ResultSymbol
				// A keyword is accepted if the parser has actions for it in
				// the current state OR if it's a reserved word. This matches
				// the C tree-sitter runtime: keywords with valid parse actions
				// are always kept; reserved words are kept even without actions
				// (for error recovery). Keywords with neither are reverted to
				// the keyword capture token (e.g., `blank_identifier` → `identifier`).
				if p.language.lookup(state, keywordSymbol) != 0 ||
					p.language.IsReservedWord(uint32(lexMode.ReservedWordSetID), keywordSymbol) {
					p.lexer.ResultSymbol = keywordSymbol
				} else {
					p.lexer.ResultSymbol = origSymbol
				}
			} else {
				p.lexer.ResultSymbol = origSymbol
			}
			p.lexer.TokenEndPosition = keywordEndPos
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

	// Remove trailing extras (e.g. comments) from children before creating
	// the parent node. In C tree-sitter, ts_subtree_array_remove_trailing_extras
	// strips extras from the end of the children array. They are then re-pushed
	// onto the stack after the parent, so they become siblings of the parent
	// rather than children - allowing them to float up to the correct level.
	var trailingExtras []Subtree
	for len(children) > 0 {
		last := children[len(children)-1]
		if !IsExtra(last, p.arena) {
			break
		}
		trailingExtras = append(trailingExtras, last)
		children = children[:len(children)-1]
	}

	// Create the internal node (without trailing extras).
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

	// If the goto state equals the base state, this reduced node is an "extra"
	// (like comments, whitespace, or heredoc_body in Ruby). Extra nodes don't
	// change the parser state and are skipped during pop operations.
	// This matches C tree-sitter's ts_parser__reduce behavior.
	if gotoState == baseState {
		node = SetExtra(node, p.arena)
	}

	// Compute new position.
	position := p.stack.Position(version)
	nodePadding := GetPadding(node, p.arena)
	nodeSize := GetSize(node, p.arena)
	newPosition := LengthAdd(LengthAdd(position, nodePadding), nodeSize)

	p.stack.Push(version, gotoState, node, false, newPosition)

	// Re-push trailing extras onto the stack as siblings of the parent.
	// Push in tree order (reverse of collection order) so that when the
	// next pop collects them in stack order and reverses, they end up
	// in the correct source order.
	for i := len(trailingExtras) - 1; i >= 0; i-- {
		extra := trailingExtras[i]
		extraPadding := GetPadding(extra, p.arena)
		extraSize := GetSize(extra, p.arena)
		newPosition = LengthAdd(LengthAdd(newPosition, extraPadding), extraSize)
		p.stack.Push(version, gotoState, extra, false, newPosition)
	}

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

		// Apply dynamic precedence to alternate paths too.
		if dynPrec != 0 && !altNode.IsInline() {
			altData := p.arena.Get(altNode)
			altData.DynamicPrecedence += int32(dynPrec)
		}

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
		altVersion := p.stack.ForkAtNode(path.node, version)
		if altVersion >= 0 {
			p.stack.Push(altVersion, altGotoState, altNode, false, altNewPosition)
		}
	}
}

// doAccept marks a version as accepted and stores the finished tree.
//
// This matches C tree-sitter's ts_parser__accept: pop all subtrees from
// the stack, find the root (non-extra) node, splice its children into the
// full array (replacing the root entry), and create a new root from
// everything - ensuring extras re-pushed by doReduce become children of
// the final root node rather than being stranded on the stack.
//
// When multiple versions accept, the tree with lower error cost wins;
// ties are broken by higher dynamic precedence. If both are equal,
// the newer tree wins (later accepts are typically more complete).
func (p *Parser) doAccept(version StackVersion) {
	tree := p.acceptTree(version)
	if !tree.IsZero() {
		if p.finishedTree.IsZero() {
			p.finishedTree = tree
		} else {
			oldCost := GetErrorCost(p.finishedTree, p.arena)
			newCost := GetErrorCost(tree, p.arena)
			if newCost < oldCost {
				p.finishedTree = tree
			} else if newCost == oldCost {
				oldPrec := GetDynamicPrecedence(p.finishedTree, p.arena)
				newPrec := GetDynamicPrecedence(tree, p.arena)
				if newPrec >= oldPrec {
					p.finishedTree = tree
				}
			}
		}
	}
	p.acceptCount++
	p.stack.Halt(version)
}

// acceptTree pops all subtrees from the stack and reconstructs the root node
// with any extras (comments) that were re-pushed by doReduce trailing
// extras handling. If there are no extras on the stack, it returns the root
// node directly (fast path).
func (p *Parser) acceptTree(version StackVersion) Subtree {
	allSubtrees := p.stack.PopAll(version)
	if len(allSubtrees) == 0 {
		return SubtreeZero
	}

	// Fast path: if there is only one subtree (no re-pushed extras), use it directly.
	if len(allSubtrees) == 1 {
		return allSubtrees[0]
	}

	// Reverse to tree order (stack is top-first, tree is left-first in source).
	for i, j := 0, len(allSubtrees)-1; i < j; i, j = i+1, j-1 {
		allSubtrees[i], allSubtrees[j] = allSubtrees[j], allSubtrees[i]
	}

	// Find the root node (the non-extra subtree). Search backward from end,
	// matching C tree-sitter ts_parser__accept.
	rootIdx := -1
	for j := len(allSubtrees) - 1; j >= 0; j-- {
		if allSubtrees[j].IsZero() {
			continue
		}
		if !IsExtra(allSubtrees[j], p.arena) {
			rootIdx = j
			break
		}
	}

	if rootIdx < 0 {
		// No non-extra subtree found. Should not happen in valid parses.
		return allSubtrees[0]
	}

	root := allSubtrees[rootIdx]
	rootSymbol := GetSymbol(root, p.arena)
	rootProdID := GetProductionID(root, p.arena)

	// Splice: replace the root node with its children in the full array.
	rootChildren := GetChildren(root, p.arena)
	newChildren := make([]Subtree, 0, len(allSubtrees)-1+len(rootChildren))
	newChildren = append(newChildren, allSubtrees[:rootIdx]...)
	newChildren = append(newChildren, rootChildren...)
	newChildren = append(newChildren, allSubtrees[rootIdx+1:]...)

	// Create new root from all subtrees (including extras).
	newRoot := NewNodeSubtree(p.arena, rootSymbol, newChildren, rootProdID, p.language)
	SummarizeChildren(newRoot, p.arena, p.language)

	return newRoot
}

// handleError attempts error recovery for a version that has been resumed
// from a paused state. Called from condenseStack, not inline from advanceVersion.
// Ports C tree-sitter's ts_parser__handle_error:
//  1. doAllPotentialReductions (try all reductions, unfiltered with symbol=0)
//  2. Try inserting missing tokens
//  3. Push ERROR_STATE (state 0) onto all relevant versions, try merge
//  4. Record stack summary for popback recovery
//  5. Call recover (try popback to previous state, then skip token)
func (p *Parser) handleError(version StackVersion, token Subtree) {
	// Check cancellation before expensive error recovery.
	if p.ctx != nil {
		select {
		case <-p.ctx.Done():
			p.stack.Halt(version)
			return
		default:
		}
	}

	previousVersionCount := p.stack.VersionCount()
	position := p.stack.Position(version)
	lookaheadSymbol := GetSymbol(token, p.arena)

	// Step 1: doAllPotentialReductions (unfiltered, matches C's symbol=0 call).
	// This tries all reductions from the current state regardless of whether
	// they help with the lookahead. Creates split versions for each reduction.
	p.doAllPotentialReductionsUnfiltered(version)

	// Step 1.5: Check if any reduced version can already handle the lookahead.
	// If so, keep those versions and halt the rest — we've recovered via
	// reduction without needing ERROR_STATE. This matches the behavior of
	// main's filtered doAllPotentialReductions, which only keeps reductions
	// that lead to states where the lookahead is valid.
	if lookaheadSymbol != SymbolError && lookaheadSymbol != SymbolEnd {
		recovered := false
		for v := version; int(v) < p.stack.VersionCount(); v++ {
			if int(v) < previousVersionCount && v != version {
				continue
			}
			if p.stack.IsHalted(v) {
				continue
			}
			newState := p.stack.State(v)
			newEntry := p.language.tableEntry(newState, lookaheadSymbol)
			if newEntry.ActionCount > 0 && v != version {
				// This reduced version can handle the lookahead.
				p.stack.AddErrorCost(v, ErrorCostPerRecovery)
				recovered = true
			}
		}
		if recovered {
			// Halt the original version and any reduced versions that can't
			// handle the lookahead.
			for v := version; int(v) < p.stack.VersionCount(); v++ {
				if int(v) < previousVersionCount && v != version {
					continue
				}
				if p.stack.IsHalted(v) {
					continue
				}
				newState := p.stack.State(v)
				newEntry := p.language.tableEntry(newState, lookaheadSymbol)
				if newEntry.ActionCount == 0 || v == version {
					p.stack.Halt(v)
				}
			}
			p.cachedTokenValid = false
			return
		}
	}

	// Step 2: Try inserting missing tokens on all versions created so far.
	// For each version (original + any splits from step 1), try each visible
	// symbol as a missing token. If shifting the missing token followed by
	// a reduce leads to a state that can handle the lookahead, keep it.
	if lookaheadSymbol != SymbolError {
		for v := version; int(v) < p.stack.VersionCount(); v++ {
			if int(v) < previousVersionCount && v != version {
				continue
			}
			if p.stack.IsHalted(v) {
				continue
			}

			state := p.stack.State(v)
			for sym := Symbol(1); sym < Symbol(p.language.SymbolCount); sym++ {
				if p.stack.VersionCount() >= MaxVersionCount {
					break
				}
				meta := p.language.SymbolMetadata[sym]
				if !meta.Visible || meta.Supertype {
					continue
				}

				entry := p.language.tableEntry(state, sym)
				if entry.ActionCount == 0 {
					continue
				}

				// Check if the first action is a shift.
				hasShift := false
				for i := 0; i < int(entry.ActionCount); i++ {
					if entry.Actions[i].Type == ParseActionTypeShift {
						hasShift = true
						break
					}
				}
				if !hasShift {
					continue
				}

				missingVersion := p.stack.Split(v)
				if missingVersion < 0 {
					continue
				}

				missingToken := p.createMissingToken(sym)
				shiftState := entry.Actions[0].ShiftState
				p.stack.Push(missingVersion, shiftState, missingToken, false, position)
				p.stack.AddErrorCost(missingVersion, ErrorCostPerMissingTree)

				// Check if the lookahead is valid in the post-missing state.
				newState := p.stack.State(missingVersion)
				newEntry := p.language.tableEntry(newState, lookaheadSymbol)
				if newEntry.ActionCount == 0 {
					p.stack.Halt(missingVersion)
				}
			}
		}
	}

	// Step 3: Push ERROR_STATE (state 0) onto all relevant versions.
	// This transitions each version into error recovery mode where RECOVER
	// actions in the parse table will handle token skipping/popback.
	for v := version; int(v) < p.stack.VersionCount(); v++ {
		if int(v) < previousVersionCount && v != version {
			continue
		}
		if p.stack.IsHalted(v) {
			continue
		}

		vPos := p.stack.Position(v)
		p.stack.Push(v, 0, SubtreeZero, false, vPos)

		// Try to merge this version with a prior one.
		if v > version {
			merged := false
			for priorV := version; priorV < v; priorV++ {
				if p.stack.Merge(priorV, v) {
					merged = true
					break
				}
			}
			if merged {
				v--
			}
		}
	}

	// Step 4: Record stack summary for popback recovery.
	p.stack.RecordSummary(version, MaxSummaryDepth)

	// Step 5: Call recover.
	p.recover(version, token)
}

// doAllPotentialReductionsUnfiltered tries every possible reduction from the
// current state, keeping all results regardless of whether they can handle
// the lookahead. This is used in handleError to "compact" the stack before
// pushing ERROR_STATE.
// Matches C: ts_parser__do_all_potential_reductions(self, version, 0)
func (p *Parser) doAllPotentialReductionsUnfiltered(version StackVersion) {
	state := p.stack.State(version)

	for sym := Symbol(0); sym < Symbol(p.language.SymbolCount); sym++ {
		entry := p.language.tableEntry(state, sym)
		for i := 0; i < int(entry.ActionCount); i++ {
			action := entry.Actions[i]
			if action.Type != ParseActionTypeReduce {
				continue
			}

			if p.stack.VersionCount() >= MaxVersionCount {
				return
			}

			testVersion := p.stack.Split(version)
			if testVersion < 0 {
				continue
			}

			p.doReduce(testVersion, action)
		}
	}
}

// recover attempts to continue parsing after an error. Called from handleError
// after ERROR_STATE has been pushed and summary recorded, and also called
// from advanceVersion when a Recover action is encountered.
// This is a faithful port of C tree-sitter's ts_parser__recover.
func (p *Parser) recover(version StackVersion, lookahead Subtree) {
	didRecover := false
	previousVersionCount := p.stack.VersionCount()
	position := p.stack.Position(version)
	summary := p.stack.GetSummary(version)
	nodeCountSinceError := p.stack.NodeCountSinceError(version)
	currentErrorCost := p.stack.ErrorCost(version)
	lookaheadSymbol := GetSymbol(lookahead, p.arena)

	// Strategy 1: Find a previous state on the stack where the lookahead is valid.
	if summary != nil && lookaheadSymbol != SymbolError {
		for _, entry := range summary {
			if entry.State == 0 {
				continue // Skip ERROR_STATE
			}
			if entry.Position.Bytes == position.Bytes {
				continue
			}

			depth := entry.Depth
			if nodeCountSinceError > 0 {
				depth++
			}

			// Check for redundant recovery (would merge with existing version).
			wouldMerge := false
			for j := 0; j < previousVersionCount; j++ {
				if p.stack.State(StackVersion(j)) == entry.State &&
					p.stack.Position(StackVersion(j)).Bytes == position.Bytes {
					wouldMerge = true
					break
				}
			}
			if wouldMerge {
				continue
			}

			// Estimate recovery cost.
			newCost := currentErrorCost +
				entry.Depth*ErrorCostPerSkippedTree +
				(position.Bytes-entry.Position.Bytes)*ErrorCostPerSkippedChar
			if position.Point.Row > entry.Position.Point.Row {
				newCost += (position.Point.Row - entry.Position.Point.Row) * ErrorCostPerSkippedLine
			}
			if p.betterVersionExists(version, false, newCost) {
				break
			}

			// Check if the lookahead token is valid in this previous state.
			tableEntry := p.language.tableEntry(entry.State, lookaheadSymbol)
			if tableEntry.ActionCount > 0 {
				if p.recoverToState(version, depth, entry.State) {
					didRecover = true
					break
				}
			}
		}
	}

	// Remove any inactive versions created during recovery attempts.
	for i := previousVersionCount; i < p.stack.VersionCount(); i++ {
		if !p.stack.IsActive(StackVersion(i)) {
			p.stack.RemoveVersion(StackVersion(i))
			i--
		}
	}

	// At EOF, wrap everything in ERROR and accept.
	if lookaheadSymbol == SymbolEnd {
		errNode := p.createErrorNode(nil)
		p.stack.Push(version, 1, errNode, false, position)
		p.doAccept(version)
		return
	}

	// Strategy 2: Skip the lookahead token by wrapping it in an error_repeat.

	// Don't pursue this if there are already too many versions.
	if didRecover && p.stack.VersionCount() > MaxVersionCount {
		p.stack.Halt(version)
		return
	}

	// Don't skip if recovery would be worse than existing versions.
	tokenSize := GetSize(lookahead, p.arena)
	newCost := currentErrorCost + ErrorCostPerSkippedTree +
		tokenSize.Bytes*ErrorCostPerSkippedChar +
		tokenSize.Point.Row*ErrorCostPerSkippedLine
	if p.betterVersionExists(version, false, newCost) {
		p.stack.Halt(version)
		return
	}

	// Build error_repeat node with the skipped token.
	children := []Subtree{lookahead}

	// If tokens have already been skipped (node_count_since_error > 0),
	// pop the existing error_repeat and merge it with the new token.
	if nodeCountSinceError > 0 {
		results := p.stack.Pop(version, 1)
		if len(results) > 0 {
			prevSubtrees := results[0].subtrees
			allChildren := make([]Subtree, 0, len(prevSubtrees)+1)
			for i := len(prevSubtrees) - 1; i >= 0; i-- {
				if !prevSubtrees[i].IsZero() {
					allChildren = append(allChildren, prevSubtrees[i])
				}
			}
			allChildren = append(allChildren, lookahead)
			children = allChildren
		}
	}

	errorRepeat := p.createErrorRepeatNode(children)

	// Compute new position after skipping the token.
	tokenPadding := GetPadding(lookahead, p.arena)
	newPosition := LengthAdd(LengthAdd(position, tokenPadding), tokenSize)

	// Push error_repeat with ERROR_STATE (state 0).
	p.stack.Push(version, 0, errorRepeat, false, newPosition)
	p.cachedTokenValid = false

	// Track external tokens.
	if HasExternalTokens(lookahead, p.arena) {
		p.stack.SetLastExternalToken(version, lookahead)
	}
}

// recoverToState pops `depth` items from the stack, wraps them in an ERROR
// node, and pushes the ERROR node with the goal state. This allows the parser
// to "pop back" to a previous state where the lookahead is valid.
// Mirrors C tree-sitter's ts_parser__recover_to_state.
func (p *Parser) recoverToState(version StackVersion, depth uint32, goalState StateID) bool {
	results := p.stack.Pop(version, depth)
	if len(results) == 0 {
		return false
	}

	// Create ERROR node from popped subtrees (reverse from stack to tree order).
	// Filter out SubtreeZero entries which come from ERROR_STATE pushes.
	popped := results[0].subtrees
	children := make([]Subtree, 0, len(popped))
	for i := len(popped) - 1; i >= 0; i-- {
		if !popped[i].IsZero() {
			children = append(children, popped[i])
		}
	}

	errNode := p.createErrorNode(children)

	// Compute position and error cost.
	position := p.stack.Position(version)
	errPadding := GetPadding(errNode, p.arena)
	errSize := GetSize(errNode, p.arena)
	newPosition := LengthAdd(LengthAdd(position, errPadding), errSize)

	errCost := ErrorCostPerRecovery +
		ErrorCostPerSkippedTree*depth +
		errSize.Bytes*ErrorCostPerSkippedChar +
		errSize.Point.Row*ErrorCostPerSkippedLine

	p.stack.Push(version, goalState, errNode, false, newPosition)
	p.stack.AddErrorCost(version, errCost)
	p.cachedTokenValid = false

	return true
}

// betterVersionExists checks if any other stack version is clearly better
// than the given version would be at the proposed cost. Used during error
// recovery to avoid pursuing recovery paths that are already dominated.
// Mirrors C tree-sitter's ts_parser__better_version_exists.
func (p *Parser) betterVersionExists(version StackVersion, isInError bool, cost uint32) bool {
	for i := 0; i < p.stack.VersionCount(); i++ {
		v := StackVersion(i)
		if v == version || p.stack.IsHalted(v) || p.stack.IsPaused(v) {
			continue
		}
		otherStatus := p.versionStatus(v)
		proposed := errorStatus{cost: cost, isInError: isInError}
		switch p.compareVersions(otherStatus, proposed) {
		case errorComparisonTakeLeft:
			return true
		}
	}
	return false
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

// createErrorRepeatNode creates an error_repeat node wrapping skipped tokens.
// Unlike createErrorNode (which uses SymbolError), this uses SymbolErrorRepeat
// which is the internal symbol for accumulating multiple skipped tokens during
// streaming error recovery. Matches C's ts_subtree_new_node with error_repeat.
func (p *Parser) createErrorRepeatNode(children []Subtree) Subtree {
	return NewNodeSubtree(p.arena, SymbolErrorRepeat, children, 0, p.language)
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

// errorStatus captures the quality metrics for a parse version,
// used by compareVersions to determine which version to keep.
// Mirrors C tree-sitter's ErrorStatus struct.
type errorStatus struct {
	cost              uint32
	nodeCount         uint32
	dynamicPrecedence int32
	isInError         bool
}

// errorComparison is the result of comparing two version statuses.
// Mirrors C tree-sitter's ErrorComparison enum.
type errorComparison int

const (
	errorComparisonTakeLeft    errorComparison = iota // left is decisively better — kill right
	errorComparisonPreferLeft                         // left is better, but try merge first
	errorComparisonNone                               // equivalent, try merge
	errorComparisonPreferRight                        // right is better, swap or merge
	errorComparisonTakeRight                          // right is decisively better — kill left
)

// versionStatus builds an errorStatus for a stack version.
// Mirrors ts_parser__version_status in C.
func (p *Parser) versionStatus(version StackVersion) errorStatus {
	cost := p.stack.ErrorCost(version)
	return errorStatus{
		cost:              cost,
		nodeCount:         p.stack.NodeCountSinceError(version),
		dynamicPrecedence: p.stack.DynamicPrecedence(version),
		isInError:         p.stack.IsPaused(version) || p.stack.State(version) == 0,
	}
}

// compareVersions compares two version statuses to determine which
// version should be kept. This is the core GLR version pruning logic.
// Mirrors ts_parser__compare_versions in C.
//
// The comparison uses 4 dimensions:
//  1. Error state: non-error versions strongly beat in-error versions
//  2. Cost with node_count amplification: (cost_diff) * (1 + node_count)
//     This ensures slightly-worse versions die quickly as they accumulate nodes
//  3. Dynamic precedence: breaks ties when costs are equal
func (p *Parser) compareVersions(a, b errorStatus) errorComparison {
	// Rule 1: Non-error vs in-error — strong preference for non-error.
	if !a.isInError && b.isInError {
		if a.cost < b.cost {
			return errorComparisonTakeLeft
		}
		return errorComparisonPreferLeft
	}
	if a.isInError && !b.isInError {
		if b.cost < a.cost {
			return errorComparisonTakeRight
		}
		return errorComparisonPreferRight
	}

	// Rule 2: Cost comparison with node_count amplification.
	if a.cost < b.cost {
		if uint64(b.cost-a.cost)*uint64(1+a.nodeCount) > uint64(MaxCostDifference) {
			return errorComparisonTakeLeft
		}
		return errorComparisonPreferLeft
	}
	if b.cost < a.cost {
		if uint64(a.cost-b.cost)*uint64(1+b.nodeCount) > uint64(MaxCostDifference) {
			return errorComparisonTakeRight
		}
		return errorComparisonPreferRight
	}

	// Rule 3: Equal cost — break ties by dynamic precedence.
	if a.dynamicPrecedence > b.dynamicPrecedence {
		return errorComparisonPreferLeft
	}
	if b.dynamicPrecedence > a.dynamicPrecedence {
		return errorComparisonPreferRight
	}

	return errorComparisonNone
}

// condenseStack merges compatible versions, prunes inferior ones, and
// resumes paused versions for error recovery.
// This is a faithful port of C tree-sitter's ts_parser__condense_stack:
// single-pass algorithm that handles all 5 comparison outcomes, uses
// swaps for PreferRight when merge fails, removes from the end for hard
// cap, and resumes one paused version per round for bounded error recovery.
// Returns the minimum error cost of non-error versions.
func (p *Parser) condenseStack() uint32 {
	// Check cancellation at the start of condenseStack.
	if p.ctx != nil {
		select {
		case <-p.ctx.Done():
			return 0
		default:
		}
	}

	minErrorCost := uint32(math.MaxUint32)

	for i := 0; i < p.stack.VersionCount(); i++ {
		// Remove halted versions immediately.
		if p.stack.IsHalted(StackVersion(i)) {
			p.stack.RemoveVersion(StackVersion(i))
			i--
			continue
		}

		statusI := p.versionStatus(StackVersion(i))
		if !statusI.isInError && statusI.cost < minErrorCost {
			minErrorCost = statusI.cost
		}

		// Compare version i against all prior versions j < i.
		for j := 0; j < i; j++ {
			statusJ := p.versionStatus(StackVersion(j))

			switch p.compareVersions(statusJ, statusI) {
			case errorComparisonTakeLeft:
				// j is decisively better — kill i.
				p.stack.RemoveVersion(StackVersion(i))
				i--
				goto nextVersion

			case errorComparisonPreferLeft, errorComparisonNone:
				// j is better or equal — try merge (requires same state).
				if p.stack.Merge(StackVersion(j), StackVersion(i)) {
					i--
					goto nextVersion
				}

			case errorComparisonPreferRight:
				// i is better — try merge, or swap positions.
				if p.stack.Merge(StackVersion(j), StackVersion(i)) {
					i--
					goto nextVersion
				}
				p.stack.SwapVersions(StackVersion(i), StackVersion(j))

			case errorComparisonTakeRight:
				// i is decisively better — kill j.
				p.stack.RemoveVersion(StackVersion(j))
				i--
				j--
			}
		}
	nextVersion:
	}

	// Hard cap: remove from the end (relies on swap ordering).
	for p.stack.VersionCount() > MaxVersionCount {
		p.stack.RemoveVersion(StackVersion(MaxVersionCount))
	}

	// Resume paused versions for error recovery.
	// Matches C tree-sitter's ts_parser__condense_stack paused handling:
	// if no unpaused version exists, resume one paused version and call
	// handleError. Otherwise, remove all paused versions.
	if p.stack.VersionCount() > 0 {
		hasUnpausedVersion := false
		for i := 0; i < p.stack.VersionCount(); i++ {
			v := StackVersion(i)
			if p.stack.IsPaused(v) {
				if !hasUnpausedVersion && p.acceptCount < uint32(p.stack.VersionCount()) {
					// Resume this version and handle error recovery.
					lookahead := p.stack.Resume(v)
					p.handleError(v, lookahead)
					hasUnpausedVersion = true
				} else {
					// Remove extra paused versions.
					p.stack.RemoveVersion(v)
					i--
				}
			} else {
				hasUnpausedVersion = true
			}
		}
	}

	return minErrorCost
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
