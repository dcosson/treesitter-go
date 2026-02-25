package treesitter

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
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

	// Check for null lookahead (non-terminal extra end state).
	// When lexToken returns a zero Subtree, this state is at the end of a
	// non-terminal extra rule (e.g., heredoc_body). The C runtime returns NULL
	// from ts_parser__lex and uses ts_builtin_sym_end for action lookup,
	// triggering reductions. After each reduce, it re-lexes with the new state.
	nullLookahead := token.IsZero()
	var tokenSymbol Symbol
	if nullLookahead {
		tokenSymbol = SymbolEnd
	} else {
		tokenSymbol = GetSymbol(token, p.arena)
	}

	if position.Bytes >= 48 && position.Bytes <= 68 {
		fmt.Fprintf(os.Stderr, "[DEBUG ADVANCE] ver=%d state=%d pos=%d tokenSym=%d null=%v\n",
			version, state, position.Bytes, tokenSymbol, nullLookahead)
	}

	// Inner reduce loop: keep reducing until we can shift or accept.
	// This avoids re-lexing after every reduce.
	for reduceCount := 0; reduceCount < 1000; reduceCount++ {
		// Look up parse actions.
		state = p.stack.State(version)
		entry := p.language.tableEntry(state, tokenSymbol)
		if entry.ActionCount == 0 {
			if nullLookahead {
				// No action for SymbolEnd after NTE reduction — need to re-lex
				// with the current state (which should have a valid lex mode).
				return true
			}
			// Keyword demotion: if the lookahead is a keyword with no
			// actions, but the word token (keyword_capture_token) DOES have
			// actions, demote the keyword back to the word token and retry.
			// This handles cases where a keyword is valid in one parse state
			// but not another (e.g., Java's `_` as underscore_pattern vs
			// identifier). Matches C runtime (reference/parser.c:1716-1742).
			if !token.IsZero() && GetIsKeyword(token, p.arena) &&
				tokenSymbol != p.language.KeywordCaptureToken &&
				!p.language.IsReservedWord(uint32(p.language.LexModes[state].ReservedWordSetID), tokenSymbol) {
				wordEntry := p.language.tableEntry(state, p.language.KeywordCaptureToken)
				if wordEntry.ActionCount > 0 {
					token = SetSubtreeSymbol(token, p.arena, p.language.KeywordCaptureToken, p.language)
					tokenSymbol = p.language.KeywordCaptureToken
					continue
				}
			}

			// No valid action — pause this version for error recovery.
			// This applies even in ERROR_STATE (state 0). C tree-sitter
			// always pauses and lets condenseStack resume with handleError,
			// which records a fresh summary matching the current stack state.
			if position.Bytes >= 56 && position.Bytes <= 68 {
				fmt.Fprintf(os.Stderr, "[DEBUG PARSER] PAUSE version=%d state=%d pos=%d tokenSym=%d\n",
					version, state, position.Bytes, tokenSymbol)
			}
			p.stack.Pause(version, token)
			return true
		}

		action := entry.Actions[0]

		// Match C runtime version ordering for GLR reduces.
		//
		// In C tree-sitter, all actions are processed in a flat loop. All
		// reduces create new versions via ts_stack_pop_count (which doesn't
		// modify the original version). After the loop,
		// ts_stack_renumber_version replaces the original version with the
		// LAST reduction's version, making it the "primary".
		//
		// This ordering matters for merge disambiguation: when versions merge
		// later (nodeAddLink Case 1), equal DynPrec keeps the "existing"
		// (primary) version's subtree. In C, the last reduce becomes primary.
		//
		// Our Go Pop modifies the version in place, so we can't reduce on
		// the same version twice. Instead, when the primary action is a reduce
		// AND there's a later reduce in the action list, we swap: the primary
		// reduce goes on a split, and the last reduce runs on the primary
		// version.
		lastReduceIdx := -1
		if entry.ActionCount > 1 && action.Type == ParseActionTypeReduce {
			for i := int(entry.ActionCount) - 1; i > 0; i-- {
				if entry.Actions[i].Type == ParseActionTypeReduce {
					// Only swap when the last reduce's DynPrec >= primary's.
					// When the last reduce has LOWER precedence (e.g. -1 vs 0),
					// swapping would make the lower-precedence version primary,
					// which is incorrect (e.g. TS/Arrow_functions where
					// type_assertion has negative precedence vs arrow_function).
					if entry.Actions[i].ReduceDynPrec >= action.ReduceDynPrec {
						lastReduceIdx = i
					}
					break
				}
			}
		}

		if position.Bytes >= 48 && position.Bytes <= 68 && entry.ActionCount > 1 {
			fmt.Fprintf(os.Stderr, "[DEBUG CONFLICT] ver=%d pos=%d state=%d actionCount=%d lastReduceIdx=%d\n",
				version, position.Bytes, state, entry.ActionCount, lastReduceIdx)
			for ai := 0; ai < int(entry.ActionCount); ai++ {
				a := entry.Actions[ai]
				fmt.Fprintf(os.Stderr, "[DEBUG CONFLICT]   action[%d]: type=%d shiftState=%d reduceSym=%d childCount=%d dynPrec=%d\n",
					ai, a.Type, a.ShiftState, a.ReduceSymbol, a.ReduceChildCount, a.ReduceDynPrec)
			}
		}

		// Handle additional actions (GLR ambiguity) by splitting.
		for i := 1; i < int(entry.ActionCount); i++ {
			extraAction := entry.Actions[i]
			// Skip repetition shifts — these are markers for the parser's
			// repeat optimization and should not be executed as actual shifts.
			if extraAction.Type == ParseActionTypeShift && extraAction.ShiftRepetition {
				continue
			}
			if nullLookahead && extraAction.Type == ParseActionTypeShift {
				continue // Can't shift a null token
			}
			// Skip the last reduce here — it will run on the primary below.
			if i == lastReduceIdx {
				continue
			}
			splitVersion := p.stack.Split(version)
			if splitVersion < 0 {
				continue
			}
			if position.Bytes >= 56 && position.Bytes <= 68 {
				fmt.Fprintf(os.Stderr, "[DEBUG SPLIT] extra action[%d] on splitVersion=%d (from ver=%d) type=%d reduceSym=%d\n",
					i, splitVersion, version, extraAction.Type, extraAction.ReduceSymbol)
			}
			switch extraAction.Type {
			case ParseActionTypeShift:
				shiftToken := token
				if GetChildCount(shiftToken, p.arena) > 0 {
					p.breakdownLookahead(&shiftToken, state)
					extraAction.ShiftState = p.language.nextState(state, GetSymbol(shiftToken, p.arena))
				}
				p.doShift(splitVersion, extraAction, shiftToken)
			case ParseActionTypeReduce:
				p.doReduce(splitVersion, extraAction, nullLookahead)
			case ParseActionTypeAccept:
				p.doAccept(splitVersion)
			case ParseActionTypeRecover:
				if !nullLookahead {
					recoverToken := token
					if GetChildCount(recoverToken, p.arena) > 0 {
						p.breakdownLookahead(&recoverToken, StateID(0))
					}
					p.recover(splitVersion, recoverToken)
				}
			}
		}

		// When swapping reduces: put the primary reduce on a split, and
		// the last reduce becomes the new primary action.
		if lastReduceIdx > 0 {
			splitVersion := p.stack.Split(version)
			if splitVersion >= 0 {
				if position.Bytes >= 56 && position.Bytes <= 68 {
					fmt.Fprintf(os.Stderr, "[DEBUG SWAP] original primary reduce sym=%d on splitVersion=%d, lastReduce sym=%d becomes primary on ver=%d\n",
						action.ReduceSymbol, splitVersion, entry.Actions[lastReduceIdx].ReduceSymbol, version)
				}
				p.doReduce(splitVersion, action, nullLookahead)
			}
			action = entry.Actions[lastReduceIdx]
		}

		// Execute the primary action.
		// Skip repetition shifts in primary action too.
		if action.Type == ParseActionTypeShift && action.ShiftRepetition {
			continue
		}
		switch action.Type {
		case ParseActionTypeShift:
			if nullLookahead {
				// Can't shift a null token — return to re-lex with new state.
				return true
			}
			if GetChildCount(token, p.arena) > 0 {
				p.breakdownLookahead(&token, state)
				action.ShiftState = p.language.nextState(state, GetSymbol(token, p.arena))
			}
			p.doShift(version, action, token)
			return true
		case ParseActionTypeReduce:
			p.doReduce(version, action, nullLookahead)
			if nullLookahead {
				// After NTE reduce, return to re-lex with the new state.
				return true
			}
			// Continue the inner loop to check the new state.
			continue
		case ParseActionTypeAccept:
			p.doAccept(version)
			return true
		case ParseActionTypeRecover:
			if nullLookahead {
				// Can't recover with a null token — return to re-lex.
				return true
			}
			// Call recover() which handles EOF (wraps in ERROR + accepts),
			// tries summary-based popback, or falls back to skipping the token.
			// This matches C's ts_parser__advance which always calls
			// ts_parser__recover for RECOVER actions, including at EOF.
			if GetChildCount(token, p.arena) > 0 {
				p.breakdownLookahead(&token, StateID(0))
			}
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

	// No-lookahead sentinel: this state is at the end of a non-terminal extra
	// rule (e.g., heredoc_body in Ruby). The C runtime returns NULL_SUBTREE here,
	// causing the parser to use SymbolEnd for action lookup (triggering reductions)
	// and then re-lex with the new state. We return a zero Subtree to signal this.
	if lexMode.LexState == LexStateNoLookahead {
		return Subtree{}
	}

	// Check cache: only valid if same lex state, external lex state, and position.
	// When the language has keyword extraction, the cached token must also pass
	// keyword reusability checks. A cached token whose symbol is the keyword
	// capture token (e.g., identifier) may need re-lexing if the parse state
	// differs (keyword acceptance is parse-state-dependent). Matches C tree-sitter's
	// ts_parser__can_reuse_first_leaf (reference/parser.c:488-495).
	if p.cachedTokenValid &&
		p.cachedTokenState == StateID(lexMode.LexState) &&
		p.cachedTokenExtState == lexMode.ExternalLexState &&
		p.cachedTokenPosition.Bytes == position.Bytes {
		canReuse := true
		if p.language.KeywordLexFn != nil {
			cachedSym := GetSymbol(p.cachedToken, p.arena)
			if cachedSym == p.language.KeywordCaptureToken {
				// Cached token is the keyword capture token (e.g., identifier).
				// Reuse only if it's NOT a keyword AND the parse state matches.
				// If it IS a keyword (was matched by keyword lex but rejected at
				// original state), a different state might accept the keyword.
				if GetIsKeyword(p.cachedToken, p.arena) || GetParseState(p.cachedToken, p.arena) != state {
					canReuse = false
				}
			}
		}
		if canReuse {
			return p.cachedToken
		}
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

				fmt.Fprintf(os.Stderr, "[DEBUG PARSER] zero-width ext token: extIdx=%d gramSym=%d state=%d nextState=%d isExtra=%v isError=%v hasAdvanced=%v pos=%d ver=%d\n",
					extTokenIndex, grammarSymbol, state, nextParseState, tokenIsExtra, state == 0, p.stack.HasAdvancedSinceError(version), position.Bytes, version)

				// Reject empty external tokens that would cause infinite loops.
				// Matches C: error_mode || !has_advanced_since_error || token_is_extra
				if state == 0 || !p.stack.HasAdvancedSinceError(version) || tokenIsExtra {
					// Fall through to internal lex — reject this empty token.
					fmt.Fprintf(os.Stderr, "[DEBUG PARSER] REJECTED zero-width ext token at pos=%d ver=%d\n", position.Bytes, version)
				} else {
					// Accept: not in error recovery, has advanced, and not extra.
					if int(extTokenIndex) < len(p.language.ExternalSymbolMap) {
						p.lexer.ResultSymbol = grammarSymbol
					}
					foundExternalToken = true
					fmt.Fprintf(os.Stderr, "[DEBUG PARSER] ACCEPTED zero-width ext token at pos=%d ver=%d gramSym=%d\n", position.Bytes, version, p.lexer.ResultSymbol)
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
	isKeyword := false
	if !foundExternalToken {
		p.lexer.Start(position)

		found := false
		if p.language.LexFn != nil {
			found = p.language.LexFn(p.lexer, StateID(lexMode.LexState))
		}

		// If the result is the keyword capture token, try keyword lex.
		// Keyword lex starts from token_start_position (after whitespace)
		// at state 0. The keyword match is accepted only if the keyword
		// lex's token_end_position matches the original token end (i.e.,
		// the keyword consumed the ENTIRE identifier text). This matches
		// the C tree-sitter runtime which resets to token_start_position
		// and compares token_end_position.bytes (not current_position).
		if found && p.language.KeywordLexFn != nil && p.lexer.ResultSymbol == p.language.KeywordCaptureToken {
			keywordEndPos := p.lexer.TokenEndPosition
			origSymbol := p.lexer.ResultSymbol

			// Start keyword lex from token_start_position (after whitespace),
			// matching C tree-sitter's ts_lexer_reset to token_start_position.
			p.lexer.Start(p.lexer.TokenStartPosition())
			kwLexResult := p.language.KeywordLexFn(p.lexer, 0)
			// Compare token_end_position (marked end), not current_position,
			// matching C tree-sitter's token_end_position.bytes comparison.
			keywordMatched := kwLexResult &&
				p.lexer.TokenEndPosition.Bytes == keywordEndPos.Bytes

			isKeyword = keywordMatched
			if keywordMatched {
				keywordSymbol := p.lexer.ResultSymbol
				// A keyword is accepted if the parser has actions for it in
				// the current state OR if it's a reserved word. This matches
				// the C tree-sitter runtime: keywords with valid parse actions
				// are always kept; reserved words are kept even without actions
				// (for error recovery). Keywords with neither are reverted to
				// the keyword capture token (e.g., `blank_identifier` → `identifier`).
				lookupResult := p.language.lookup(state, keywordSymbol)
				isReserved := p.language.IsReservedWord(uint32(lexMode.ReservedWordSetID), keywordSymbol)
				if lookupResult != 0 || isReserved {
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

// breakdownLookahead decomposes a reused composite node back into individual
// tokens when the node's parse state doesn't match the current parser state.
// This is needed for incremental parsing + error recovery when a previously
// reused non-terminal can't be shifted as-is.
// Matches C: ts_parser__breakdown_lookahead
func (p *Parser) breakdownLookahead(lookahead *Subtree, state StateID) {
	if p.reusableNode == nil {
		return
	}
	didDescend := false
	tree := p.reusableNode.Tree()
	for GetChildCount(tree, p.arena) > 0 && GetParseState(tree, p.arena) != state {
		p.reusableNode.Descend()
		tree = p.reusableNode.Tree()
		didDescend = true
	}
	if didDescend {
		*lookahead = tree
	}
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
// endOfNonTerminalExtra indicates the reduction is happening during
// non-terminal extra processing (null lookahead). Only when this is true
// AND the goto state equals the base state should the node be marked as
// extra. This matches C tree-sitter's ts_parser__reduce:
//
//	if (end_of_non_terminal_extra && next_state == state) parent->extra = true;
//
// When Pop returns multiple paths through a merged stack DAG, paths that
// converge to the same base node represent alternative children for the
// same reduction. These are resolved in-place using selectChildren (port
// of C's ts_parser__select_children) rather than creating separate
// versions. Only paths with different base nodes create new versions.
func (p *Parser) doReduce(version StackVersion, action ParseActionEntry, endOfNonTerminalExtra bool) {
	childCount := uint32(action.ReduceChildCount)
	symbol := action.ReduceSymbol
	productionID := action.ReduceProdID
	dynPrec := action.ReduceDynPrec

	pos := p.stack.Position(version)
	if pos.Bytes >= 56 && pos.Bytes <= 68 {
		fmt.Fprintf(os.Stderr, "[DEBUG REDUCE] ver=%d sym=%d childCount=%d prodID=%d dynPrec=%d pos=%d nResults=%d\n",
			version, symbol, childCount, productionID, dynPrec, pos.Bytes, 0)
	}

	// Pop children.
	// Enable trace for slice_expression (sym=440), _slice_expression_interpolation (sym=344),
	// _interpolation_fallbacks (sym=445)
	if symbol == 440 || symbol == 344 || symbol == 445 {
		PopDebugTrace = true
	}
	results := p.stack.Pop(version, childCount)
	if pos.Bytes >= 45 && pos.Bytes <= 68 {
		fmt.Fprintf(os.Stderr, "[DEBUG REDUCE] ver=%d sym=%d popResults=%d\n", version, symbol, len(results))
		for ri, r := range results {
			fmt.Fprintf(os.Stderr, "[DEBUG REDUCE]   result[%d]: node.state=%d nSubtrees=%d\n", ri, r.node.state, len(r.subtrees))
		}
	}
	// Special trace for slice_expression (sym=440) -- unconditional
	if symbol == 440 || symbol == 344 {
		fmt.Fprintf(os.Stderr, "[DEBUG SLICE_EXPR] ver=%d popResults=%d pos=%d\n", version, len(results), pos.Bytes)
		for ri, r := range results {
			fmt.Fprintf(os.Stderr, "[DEBUG SLICE_EXPR]   result[%d]: baseNode.state=%d nSubtrees=%d\n", ri, r.node.state, len(r.subtrees))
			for si, st := range r.subtrees {
				if !st.IsZero() {
					fmt.Fprintf(os.Stderr, "[DEBUG SLICE_EXPR]     subtree[%d]: sym=%d size=%d cc=%d\n",
						si, GetSymbol(st, p.arena), GetSize(st, p.arena).Bytes, GetChildCount(st, p.arena))
				}
			}
		}
	}
	if len(results) == 0 {
		p.stack.Halt(version)
		return
	}

	// Group pop results by base node. Paths that converge to the same
	// base node are alternative children for the same reduction — use
	// selectChildren to pick the best set in-place. Only paths with
	// different base nodes create separate stack versions.
	//
	// This matches C tree-sitter's ts_parser__reduce which groups
	// StackSlices by version (assigned by ts_stack__add_slice based
	// on the base node identity).
	type reduceGroup struct {
		node           *StackNode
		bestChildren   []Subtree
		trailingExtras []Subtree
	}
	groups := make([]reduceGroup, 0, 1)
	groupIndex := make(map[*StackNode]int)

	for _, result := range results {
		// Reverse subtrees from stack order to tree order.
		children := make([]Subtree, len(result.subtrees))
		for j, s := range result.subtrees {
			children[len(children)-1-j] = s
		}

		// Remove trailing extras (e.g. comments) from children.
		var extras []Subtree
		for len(children) > 0 {
			last := children[len(children)-1]
			if !IsExtra(last, p.arena) {
				break
			}
			extras = append(extras, last)
			children = children[:len(children)-1]
		}

		if idx, ok := groupIndex[result.node]; ok {
			// Same base node — compare using selectChildren.
			if p.selectChildren(symbol, productionID, dynPrec,
				groups[idx].bestChildren, children) {
				groups[idx].bestChildren = children
				groups[idx].trailingExtras = extras
			}
		} else {
			groupIndex[result.node] = len(groups)
			groups = append(groups, reduceGroup{
				node:           result.node,
				bestChildren:   children,
				trailingExtras: extras,
			})
		}
	}

	// Process each group. The first group is the primary version
	// (Pop already moved the head to results[0].node).
	for gIdx, group := range groups {
		if pos.Bytes >= 56 && pos.Bytes <= 68 && (symbol == 204 || symbol == 201 || symbol == 184) {
			fmt.Fprintf(os.Stderr, "[DEBUG REDUCE GROUPS] sym=%d ver=%d gIdx=%d/%d baseState=%d children:",
				symbol, version, gIdx, len(groups), group.node.state)
			for ci, ch := range group.bestChildren {
				chSym := GetSymbol(ch, p.arena)
				chCC := GetChildCount(ch, p.arena)
				fmt.Fprintf(os.Stderr, " [%d]sym=%d,cc=%d", ci, chSym, chCC)
				// Show one level deeper for the first child
				if ci == 0 && chCC > 0 {
					grandchildren := GetChildren(ch, p.arena)
					fmt.Fprintf(os.Stderr, "{")
					for _, gc := range grandchildren {
						fmt.Fprintf(os.Stderr, "sym=%d,", GetSymbol(gc, p.arena))
					}
					fmt.Fprintf(os.Stderr, "}")
				}
			}
			fmt.Fprintf(os.Stderr, "\n")
		}
		// Create the internal node from the best children.
		node := NewNodeSubtree(p.arena, symbol, group.bestChildren, productionID, p.language)
		SummarizeChildren(node, p.arena, p.language)

		// Apply dynamic precedence.
		if dynPrec != 0 && !node.IsInline() {
			data := p.arena.Get(node)
			data.DynamicPrecedence += int32(dynPrec)
		}

		if gIdx == 0 {
			// Primary version — push onto the existing version.
			baseState := p.stack.State(version)
			gotoState := p.language.nextState(baseState, symbol)

			// C tree-sitter: if (end_of_non_terminal_extra && next_state == state)
			if endOfNonTerminalExtra && gotoState == baseState {
				node = SetExtra(node, p.arena)
			}

			position := p.stack.Position(version)
			nodePadding := GetPadding(node, p.arena)
			nodeSize := GetSize(node, p.arena)
			newPosition := LengthAdd(LengthAdd(position, nodePadding), nodeSize)

			p.stack.Push(version, gotoState, node, false, newPosition)

			// Re-push trailing extras as siblings of the parent.
			for i := len(group.trailingExtras) - 1; i >= 0; i-- {
				extra := group.trailingExtras[i]
				extraPadding := GetPadding(extra, p.arena)
				extraSize := GetSize(extra, p.arena)
				newPosition = LengthAdd(LengthAdd(newPosition, extraPadding), extraSize)
				p.stack.Push(version, gotoState, extra, false, newPosition)
			}

		} else {
			// Alternative version — different base node, create new version.
			altBaseState := group.node.state
			altGotoState := p.language.nextState(altBaseState, symbol)

			if endOfNonTerminalExtra && altGotoState == altBaseState {
				node = SetExtra(node, p.arena)
			}

			altBasePosition := group.node.position
			altNodePadding := GetPadding(node, p.arena)
			altNodeSize := GetSize(node, p.arena)
			altNewPosition := LengthAdd(LengthAdd(altBasePosition, altNodePadding), altNodeSize)

			altVersion := p.stack.ForkAtNode(group.node, version)
			if altVersion >= 0 {
				p.stack.Push(altVersion, altGotoState, node, false, altNewPosition)

				// Re-push trailing extras as siblings, matching C behavior.
				for i := len(group.trailingExtras) - 1; i >= 0; i-- {
					extra := group.trailingExtras[i]
					extraPadding := GetPadding(extra, p.arena)
					extraSize := GetSize(extra, p.arena)
					altNewPosition = LengthAdd(LengthAdd(altNewPosition, extraPadding), extraSize)
					p.stack.Push(altVersion, altGotoState, extra, false, altNewPosition)
				}

			}
		}
	}
}

// selectChildren compares two sets of children for the same reduction.
// Returns true if the alternative children should replace the current best.
// Port of C tree-sitter's ts_parser__select_children.
func (p *Parser) selectChildren(symbol Symbol, productionID uint16, dynPrec int16,
	currentChildren, altChildren []Subtree) bool {
	// Create temporary nodes for comparison, matching C's approach.
	currentNode := NewNodeSubtree(p.arena, symbol, currentChildren, productionID, p.language)
	SummarizeChildren(currentNode, p.arena, p.language)
	if dynPrec != 0 && !currentNode.IsInline() {
		data := p.arena.Get(currentNode)
		data.DynamicPrecedence += int32(dynPrec)
	}

	altNode := NewNodeSubtree(p.arena, symbol, altChildren, productionID, p.language)
	SummarizeChildren(altNode, p.arena, p.language)
	if dynPrec != 0 && !altNode.IsInline() {
		data := p.arena.Get(altNode)
		data.DynamicPrecedence += int32(dynPrec)
	}

	result := p.selectTree(currentNode, altNode)
	pos := p.stack.Position(0) // just for context
	if pos.Bytes >= 56 && pos.Bytes <= 68 {
		fmt.Fprintf(os.Stderr, "[DEBUG selectChildren] sym=%d pos~%d: current[0]sym=%d alt[0]sym=%d selectAlt=%v\n",
			symbol, pos.Bytes,
			func() Symbol { if len(currentChildren) > 0 { return GetSymbol(currentChildren[0], p.arena) }; return 0 }(),
			func() Symbol { if len(altChildren) > 0 { return GetSymbol(altChildren[0], p.arena) }; return 0 }(),
			result)
	}
	return result
}

// doAccept marks a version as accepted and stores the finished tree.
//
// Matches C tree-sitter's ts_parser__accept (parser.c:1048-1099):
// Pop all subtrees from the stack (traversing all links at merge points),
// build a root tree from each path, and use selectTree to pick the best.
// This ensures that when multiple parse paths converge at accept time,
// the correct tree is chosen via error cost / dynamic precedence comparison.
func (p *Parser) doAccept(version StackVersion) {
	paths := p.stack.PopAll(version)
	for _, subtrees := range paths {
		root := p.buildAcceptTree(subtrees)
		if root.IsZero() {
			continue
		}
		p.acceptCount++
		if p.finishedTree.IsZero() {
			p.finishedTree = root
		} else if p.selectTree(p.finishedTree, root) {
			p.finishedTree = root
		}
	}
	p.stack.Halt(version)
}

// selectTree determines whether a new tree should replace the existing one.
// Returns true to select the new tree, false to keep the existing one.
// This is a port of C tree-sitter's ts_parser__select_tree.
func (p *Parser) selectTree(left, right Subtree) bool {
	if left.IsZero() {
		return true
	}
	if right.IsZero() {
		return false
	}

	// Lower error cost wins.
	rightCost := GetErrorCost(right, p.arena)
	leftCost := GetErrorCost(left, p.arena)
	if rightCost < leftCost {
		return true
	}
	if leftCost < rightCost {
		return false
	}

	// Higher dynamic precedence wins (strict >).
	rightPrec := GetDynamicPrecedence(right, p.arena)
	leftPrec := GetDynamicPrecedence(left, p.arena)
	if rightPrec > leftPrec {
		return true
	}
	if leftPrec > rightPrec {
		return false
	}

	// If both have errors, prefer the new tree.
	if leftCost > 0 {
		return true
	}

	// Structural comparison: prefer the tree with the lower symbol ID
	// (earlier grammar definition). This matches C's ts_subtree_compare.
	cmp := subtreeCompare(left, right, p.arena)
	fmt.Fprintf(os.Stderr, "[DEBUG selectTree] leftSym=%d rightSym=%d leftCost=%d rightCost=%d leftPrec=%d rightPrec=%d cmp=%d result=%v\n",
		GetSymbol(left, p.arena), GetSymbol(right, p.arena), leftCost, rightCost, leftPrec, rightPrec, cmp, cmp > 0)
	return cmp > 0 // right is "earlier" → select right
}

// subtreeCompare compares two subtrees structurally, returning:
//
//	-1 if left is "earlier" (lower symbol, fewer children)
//	 1 if right is "earlier"
//	 0 if structurally identical
//
// Port of C tree-sitter's ts_subtree_compare. Uses iterative stack
// to avoid recursion depth issues.
func subtreeCompare(left, right Subtree, arena *SubtreeArena) int {
	type pair struct{ left, right Subtree }
	stack := []pair{{left, right}}

	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		leftSym := GetSymbol(p.left, arena)
		rightSym := GetSymbol(p.right, arena)
		if leftSym < rightSym {
			return -1
		}
		if rightSym < leftSym {
			return 1
		}

		leftCC := GetChildCount(p.left, arena)
		rightCC := GetChildCount(p.right, arena)
		if leftCC < rightCC {
			return -1
		}
		if rightCC < leftCC {
			return 1
		}

		// Push children in reverse order (so first child is compared first).
		leftChildren := GetChildren(p.left, arena)
		rightChildren := GetChildren(p.right, arena)
		for i := int(leftCC) - 1; i >= 0; i-- {
			stack = append(stack, pair{leftChildren[i], rightChildren[i]})
		}
	}

	return 0
}

// buildAcceptTree reconstructs the root node from a path of subtrees
// (already in source order, leftmost first). Finds the root (non-extra)
// node, splices its children into the full array (replacing the root entry),
// and creates a new root — ensuring extras re-pushed by doReduce become
// children of the final root node.
//
// Matches C tree-sitter's inline tree construction in ts_parser__accept
// (parser.c:1060-1079).
func (p *Parser) buildAcceptTree(allSubtrees []Subtree) Subtree {
	if len(allSubtrees) == 0 {
		return SubtreeZero
	}

	// Fast path: if there is only one subtree (no re-pushed extras), use it directly.
	if len(allSubtrees) == 1 {
		return allSubtrees[0]
	}

	// Find the root node: the first non-extra subtree in source order.
	// Note: C's ts_parser__accept (parser.c:1061) searches from right to left
	// after pushing EOF. Go currently searches left to right. This difference
	// needs investigation — see bead tree-sitter-go-mgu for accept path work.
	rootIdx := -1
	for j := 0; j < len(allSubtrees); j++ {
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
// Faithfully ports C tree-sitter's ts_parser__handle_error (parser.c:1439-1534).
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

	// Step 1: Perform any reductions that can happen in this state, regardless
	// of the lookahead. After skipping one or more invalid tokens, the parser
	// might find a token that would have allowed a reduction to take place.
	// Matches C: ts_parser__do_all_potential_reductions(self, version, 0)
	p.doAllPotentialReductions(version, 0)
	versionCount := p.stack.VersionCount()
	position := p.stack.Position(version)

	if p.debug {
		leafSym := GetLeafSymbol(token, p.arena)
		symName := ""
		if p.language != nil && int(leafSym) < len(p.language.SymbolNames) {
			symName = p.language.SymbolNames[leafSym]
		}
		fmt.Printf("[handleError] v=%d state=%d pos=%d lookahead=%d(%s) prevVersions=%d newVersions=%d\n",
			version, p.stack.State(version), position.Bytes, leafSym, symName, previousVersionCount, versionCount)
	}

	// Step 2+3 combined: Push a discontinuity onto the stack. Try inserting
	// a missing token (once), then push ERROR_STATE onto all relevant versions.
	// Matches C's combined loop (parser.c:1456-1514).
	leafSymbol := GetLeafSymbol(token, p.arena)
	didInsertMissingToken := false

	// Compute lookahead_bytes for missing token creation (matches C parser.c:1482).
	lookaheadBytes := GetTotalBytes(token, p.arena) + GetLookaheadBytes(token, p.arena)

	for v := version; v < StackVersion(versionCount); {
		// Try missing token insertion (only once across all versions).
		// Matches C: parser.c:1457-1510
		if !didInsertMissingToken {
			state := p.stack.State(v)
			for missingSym := Symbol(1); missingSym < Symbol(p.language.TokenCount); missingSym++ {
				stateAfterMissing := p.language.nextState(state, missingSym)
				if stateAfterMissing == 0 || stateAfterMissing == state {
					continue
				}

				if p.language.hasReduceAction(stateAfterMissing, leafSymbol) {
					// Create a new version with the missing token inserted.
					// Padding: In C, the lexer is reset to compute padding for
					// included range snapping. For the common case (no ranges), zero.
					// TODO: implement lexer reset for included ranges.
					padding := Length{}

					missingVersion := p.stack.Split(v)
					if missingVersion < 0 {
						continue
					}
					missingTree := p.createMissingToken(missingSym, padding, lookaheadBytes)
					missingPos := p.stack.Position(missingVersion)
					p.stack.Push(missingVersion, stateAfterMissing, missingTree, false, missingPos)

					if p.doAllPotentialReductions(missingVersion, leafSymbol) {
						if p.debug {
							symName := ""
							if int(missingSym) < len(p.language.SymbolNames) {
								symName = p.language.SymbolNames[missingSym]
							}
							fmt.Printf("[handleError] recover_with_missing symbol:%s state:%d\n",
								symName, p.stack.State(missingVersion))
						}
						didInsertMissingToken = true
						break
					}
				}
			}
		}

		// Push ERROR_STATE (state 0) onto this version.
		// Matches C: ts_stack_push(self->stack, v, NULL_SUBTREE, false, ERROR_STATE)
		vPos := p.stack.Position(v)
		p.stack.Push(v, 0, SubtreeZero, false, vPos)

		// Advance: process version first, then skip to doAllPotentialReductions versions.
		// Matches C: v = (v == version) ? previous_version_count : v + 1
		if v == version {
			v = StackVersion(previousVersionCount)
		} else {
			v++
		}
	}

	// Merge all versions created by doAllPotentialReductions back into version.
	// Matches C: parser.c:1516-1519
	for i := previousVersionCount; i < int(versionCount); i++ {
		p.stack.Merge(version, StackVersion(previousVersionCount))
	}

	// Record stack summary for popback recovery.
	// Matches C: ts_stack_record_summary(self->stack, version, MAX_SUMMARY_DEPTH)
	p.stack.RecordSummary(version, MaxSummaryDepth)

	// Begin recovery with the current lookahead node. Break down if composite.
	// Matches C: parser.c:1528-1531
	if GetChildCount(token, p.arena) > 0 {
		p.breakdownLookahead(&token, StateID(0))
	}
	p.recover(version, token)
}

// doAllPotentialReductions tries every possible reduction from the current
// state. When lookaheadSymbol is 0 (unfiltered mode, used by handleError),
// it scans all terminal symbols. When non-zero (filtered mode), it only
// checks actions for that specific symbol. Returns true if the lookahead
// can be shifted. Faithful port of C's ts_parser__do_all_potential_reductions
// (reference/parser.c:1101-1189).
func (p *Parser) doAllPotentialReductions(startingVersion StackVersion, lookaheadSymbol Symbol) bool {
	initialVersionCount := p.stack.VersionCount()
	canShiftLookahead := false

	version := startingVersion
	for i := 0; ; i++ {
		versionCount := p.stack.VersionCount()
		if int(version) >= versionCount {
			break
		}

		// Try to merge with previously created versions.
		merged := false
		for j := StackVersion(initialVersionCount); j < version; j++ {
			if p.stack.Merge(j, version) {
				merged = true
				break
			}
		}
		if merged {
			continue
		}

		state := p.stack.State(version)
		hasShiftAction := false

		// Collect unique reduce actions (deduplication).
		type reduceAction struct {
			symbol    Symbol
			count     uint32
			dynPrec   int16
			prodID    uint16
		}
		var reduceActions []reduceAction

		// Determine symbol range.
		var firstSym, endSym Symbol
		if lookaheadSymbol != 0 {
			firstSym = lookaheadSymbol
			endSym = lookaheadSymbol + 1
		} else {
			firstSym = 1
			endSym = Symbol(p.language.TokenCount)
		}

		for sym := firstSym; sym < endSym; sym++ {
			entry := p.language.tableEntry(state, sym)
			for j := 0; j < int(entry.ActionCount); j++ {
				action := entry.Actions[j]
				switch action.Type {
				case ParseActionTypeShift, ParseActionTypeRecover:
					if !action.ShiftExtra && !action.ShiftRepetition {
						hasShiftAction = true
					}
				case ParseActionTypeReduce:
					if action.ReduceChildCount > 0 {
						ra := reduceAction{
							symbol:  action.ReduceSymbol,
							count:   uint32(action.ReduceChildCount),
							dynPrec: action.ReduceDynPrec,
							prodID:  action.ReduceProdID,
						}
						// Deduplicate.
						found := false
						for _, existing := range reduceActions {
							if existing == ra {
								found = true
								break
							}
						}
						if !found {
							reduceActions = append(reduceActions, ra)
						}
					}
				}
			}
		}

		// Execute all collected reduce actions.
		var reductionVersion StackVersion = -1
		for _, ra := range reduceActions {
			reductionVersion = p.doReduceForPotential(version, ra.symbol, ra.count, ra.dynPrec, ra.prodID)
		}

		if hasShiftAction {
			canShiftLookahead = true
		} else if reductionVersion >= 0 && i < MaxVersionCount {
			// No shift action but reductions succeeded — replace this version
			// with the last reduced version and re-process it. This chains
			// reductions until a state with a shift action is found.
			p.stack.RenumberVersion(reductionVersion, version)
			continue
		} else if lookaheadSymbol != 0 {
			// No shift and no reductions for this specific symbol — remove.
			p.stack.RemoveVersion(version)
		}

		// After processing the starting version, skip to newly created versions.
		if version == startingVersion {
			version = StackVersion(versionCount)
		} else {
			version++
		}
	}

	return canShiftLookahead
}



// doReduceForPotential performs a reduce for doAllPotentialReductions.
// It splits the version, pops child_count items, and creates a parent node.
// Returns the new version, or -1 on failure.
func (p *Parser) doReduceForPotential(version StackVersion, symbol Symbol, childCount uint32, dynPrec int16, prodID uint16) StackVersion {
	splitVersion := p.stack.Split(version)
	if splitVersion < 0 {
		return -1
	}

	action := ParseActionEntry{
		Type:              ParseActionTypeReduce,
		ReduceSymbol:      symbol,
		ReduceChildCount:  uint8(childCount),
		ReduceDynPrec:     dynPrec,
		ReduceProdID:      prodID,
	}
	p.doReduce(splitVersion, action, false)

	return splitVersion
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
				if p.stack.VersionCount() >= MaxVersionCount {
					break
				}
				// Split the version before attempting Strategy 1 popback.
				// Strategy 1 destructively modifies the version via Pop. If
				// the pop doesn't reach the goal state, the version is halted.
				// In C, this is acceptable because Pop can create version forks
				// via the GSS. In our single-path Go stack, we must split
				// explicitly to preserve the original version for Strategy 2.
				recoveryVersion := p.stack.Split(version)
				if recoveryVersion < 0 {
					break
				}
				if p.recoverToState(recoveryVersion, depth, entry.State) {
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

	// At EOF, terminate by accepting.
	// C pushes an empty ERROR node at state 1, then calls ts_parser__accept
	// (which also pushes the EOF lookahead). In Go's doAccept, we don't push
	// the lookahead, and pushing a visible empty ERROR would add a spurious
	// (ERROR) child. So just push SubtreeZero at state 1 to trigger acceptance.
	if lookaheadSymbol == SymbolEnd {
		p.stack.Push(version, 1, SubtreeZero, false, position)
		p.doAccept(version)
		return
	}

	// Strategy 2: Skip the lookahead token by wrapping it in an error_repeat.
	// In C tree-sitter, both Strategy 1 and Strategy 2 modify the same version.
	// Strategy 1's popback recovery gets "extended" by Strategy 2 wrapping the
	// lookahead into the error_repeat. This matches C's behavior where the error
	// region grows to include the current token.

	// Don't pursue this if there are already too many versions.
	if didRecover && p.stack.VersionCount() > MaxVersionCount {
		p.stack.Halt(version)
		return
	}

	// If recovery succeeded and the lookahead has an external scanner state
	// change, halt this version. Matches C: parser.c:1349-1356.
	if didRecover && HasExternalScannerStateChange(lookahead, p.arena) {
		p.stack.Halt(version)
		return
	}

	// Don't skip if recovery would be worse than existing versions.
	// Use total bytes (padding + size) matching C's ts_subtree_total_bytes.
	totalBytes := GetTotalBytes(lookahead, p.arena)
	tokenPadding := GetPadding(lookahead, p.arena)
	tokenSize := GetSize(lookahead, p.arena)
	totalRows := tokenPadding.Point.Row + tokenSize.Point.Row
	newCost := currentErrorCost + ErrorCostPerSkippedTree +
		totalBytes*ErrorCostPerSkippedChar +
		totalRows*ErrorCostPerSkippedLine
	if p.betterVersionExists(version, false, newCost) {
		p.stack.Halt(version)
		return
	}

	// If the current lookahead is an extra token (e.g., comment), mark it.
	// This means it won't be counted in error cost calculations.
	// Matches C: parser.c:1371-1377.
	extraEntry := p.language.tableEntry(1, GetSymbol(lookahead, p.arena))
	if extraEntry.ActionCount > 0 {
		lastAction := extraEntry.Actions[extraEntry.ActionCount-1]
		if lastAction.Type == ParseActionTypeShift && lastAction.ShiftExtra {
			lookahead = SetExtra(lookahead, p.arena)
		}
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
// Faithful port of C tree-sitter's ts_parser__recover_to_state (parser.c:1191-1248).
// Key difference from previous Go version: iterates ALL Pop results (GSS paths),
// creating separate versions for each, keeping those that match goalState and
// halting those that don't.
func (p *Parser) recoverToState(version StackVersion, depth uint32, goalState StateID) bool {
	results := p.stack.Pop(version, depth)
	if len(results) == 0 {
		return false
	}

	// Iterate all pop results. The first result is associated with `version`
	// (Pop updates the head). Additional results need new versions via ForkAtNode.
	// This matches C's loop over StackSliceArray in ts_parser__recover_to_state.
	var previousNode *StackNode
	didRecover := false

	for i, result := range results {
		// Determine the version for this pop result.
		var sliceVersion StackVersion
		if i == 0 {
			sliceVersion = version
		} else {
			// Skip duplicate nodes (same as C's previous_version check).
			if result.node == previousNode {
				continue
			}
			sliceVersion = p.stack.ForkAtNode(result.node, version)
		}

		// Check if this version's state matches the goal state.
		if p.stack.State(sliceVersion) != goalState {
			p.stack.Halt(sliceVersion)
			continue
		}

		// Check for an existing error node at the stack top. If found, splice
		// its children into the front of the popped subtrees so that consecutive
		// skipped tokens are collected in a single ERROR node.
		// Matches C: ts_stack_pop_error in ts_parser__recover_to_state.
		var existingErrorChildren []Subtree
		if errTree, ok := p.stack.PopError(sliceVersion); ok {
			errorChildren := GetChildren(errTree, p.arena)
			if len(errorChildren) > 0 {
				existingErrorChildren = make([]Subtree, len(errorChildren))
				copy(existingErrorChildren, errorChildren)
			}
		}

		// Build children array: existing error children + popped subtrees in
		// source order (reverse from stack order). Filter out SubtreeZero entries
		// which come from ERROR_STATE pushes.
		popped := result.subtrees
		children := make([]Subtree, 0, len(existingErrorChildren)+len(popped))
		children = append(children, existingErrorChildren...)
		for j := len(popped) - 1; j >= 0; j-- {
			if !popped[j].IsZero() {
				children = append(children, popped[j])
			}
		}

		// Separate trailing extras (comments, whitespace) from the ERROR node.
		// They get pushed individually after the ERROR node, matching C's
		// ts_subtree_array_remove_trailing_extras + re-push loop.
		var trailingExtras []Subtree
		for len(children) > 0 {
			last := children[len(children)-1]
			if !IsExtra(last, p.arena) {
				break
			}
			trailingExtras = append(trailingExtras, last)
			children = children[:len(children)-1]
		}

		// Create and push ERROR node (only if there are children).
		// Error cost is embedded in the error node via createErrorNode and
		// accumulated by Push — no explicit AddErrorCost needed (matching C).
		if len(children) > 0 {
			errNode := p.createErrorNode(children)
			position := p.stack.Position(sliceVersion)
			errPadding := GetPadding(errNode, p.arena)
			errSize := GetSize(errNode, p.arena)
			newPosition := LengthAdd(LengthAdd(position, errPadding), errSize)
			p.stack.Push(sliceVersion, goalState, errNode, false, newPosition)
		}

		// Re-push trailing extras individually at the goal state.
		// Matches C: parser.c:1239-1242.
		for j := len(trailingExtras) - 1; j >= 0; j-- {
			extra := trailingExtras[j]
			pos := p.stack.Position(sliceVersion)
			extraPadding := GetPadding(extra, p.arena)
			extraSize := GetSize(extra, p.arena)
			newPos := LengthAdd(LengthAdd(pos, extraPadding), extraSize)
			p.stack.Push(sliceVersion, goalState, extra, false, newPos)
		}

		previousNode = result.node
		didRecover = true
	}

	if didRecover {
		p.cachedTokenValid = false
	}

	return didRecover
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
	data.SetFlag(SubtreeFlagNamed, true)

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

// createMissingToken creates a MISSING leaf token (zero-width, for error recovery).
// Matches C's ts_subtree_new_missing_leaf. Padding accounts for included range
// snapping; lookaheadBytes captures the current token's total reach.
func (p *Parser) createMissingToken(symbol Symbol, padding Length, lookaheadBytes uint32) Subtree {
	st, data := p.arena.Alloc()
	meta := p.language.SymbolMetadata[symbol]
	*data = SubtreeHeapData{
		Symbol:         symbol,
		Padding:        padding,
		LookaheadBytes: lookaheadBytes,
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

			cmpResult := p.compareVersions(statusJ, statusI)
			posI := p.stack.Position(StackVersion(i))
			posJ := p.stack.Position(StackVersion(j))
			if (posI.Bytes >= 45 && posI.Bytes <= 65) || (posJ.Bytes >= 45 && posJ.Bytes <= 65) || (i <= 1 && j == 0) {
				fmt.Fprintf(os.Stderr, "[DEBUG CONDENSE] cmp i=%d(pos=%d,cost=%d,prec=%d,err=%v) vs j=%d(pos=%d,cost=%d,prec=%d,err=%v) → %d\n",
					i, posI.Bytes, statusI.cost, statusI.dynamicPrecedence, statusI.isInError,
					j, posJ.Bytes, statusJ.cost, statusJ.dynamicPrecedence, statusJ.isInError,
					cmpResult)
			}
			switch cmpResult {
			case errorComparisonTakeLeft:
				// j is decisively better — kill i.
				if (posI.Bytes >= 45 && posI.Bytes <= 65) || (posJ.Bytes >= 45 && posJ.Bytes <= 65) {
					fmt.Fprintf(os.Stderr, "[DEBUG CONDENSE] REMOVE i=%d (TakeLeft j=%d)\n", i, j)
				}
				p.stack.RemoveVersion(StackVersion(i))
				i--
				goto nextVersion

			case errorComparisonPreferLeft, errorComparisonNone:
				// j is better or equal — try merge (requires same state).
				if p.stack.Merge(StackVersion(j), StackVersion(i)) {
					if (posI.Bytes >= 45 && posI.Bytes <= 65) || (posJ.Bytes >= 45 && posJ.Bytes <= 65) {
						fmt.Fprintf(os.Stderr, "[DEBUG CONDENSE] MERGE i=%d into j=%d (PreferLeft/None)\n", i, j)
					}
					i--
					goto nextVersion
				}

			case errorComparisonPreferRight:
				// i is better — try merge, or swap positions.
				if p.stack.Merge(StackVersion(j), StackVersion(i)) {
					if (posI.Bytes >= 45 && posI.Bytes <= 65) || (posJ.Bytes >= 45 && posJ.Bytes <= 65) {
						fmt.Fprintf(os.Stderr, "[DEBUG CONDENSE] MERGE i=%d into j=%d (PreferRight)\n", i, j)
					}
					i--
					goto nextVersion
				}
				p.stack.SwapVersions(StackVersion(i), StackVersion(j))

			case errorComparisonTakeRight:
				// i is decisively better — kill j.
				if (posI.Bytes >= 45 && posI.Bytes <= 65) || (posJ.Bytes >= 45 && posJ.Bytes <= 65) {
					fmt.Fprintf(os.Stderr, "[DEBUG CONDENSE] REMOVE j=%d (TakeRight i=%d)\n", j, i)
				}
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
				if !hasUnpausedVersion && p.acceptCount < MaxVersionCount {
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
