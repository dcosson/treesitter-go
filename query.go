package treesitter

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Query constants matching C tree-sitter.
const (
	maxStepCaptureCount  = 3
	maxNegatedFieldCount = 8
	patternDoneMarker    = 0xFFFF
	noneValue            = 0xFFFF
	wildcardSymbol       = 0 // Symbol(0) matches any node ('_')
)

// QueryError represents an error that occurred during query compilation.
type QueryError struct {
	Offset  uint32
	Type    QueryErrorType
	Message string
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("query error at offset %d: %s: %s", e.Offset, e.Type, e.Message)
}

// QueryErrorType identifies the kind of query error.
type QueryErrorType int

const (
	QueryErrorNone      QueryErrorType = iota
	QueryErrorSyntax                   // malformed S-expression
	QueryErrorNodeType                 // unknown node type
	QueryErrorField                    // unknown field name
	QueryErrorCapture                  // bad capture syntax
	QueryErrorStructure                // structural constraint violation
	QueryErrorLanguage                 // language mismatch
)

func (t QueryErrorType) String() string {
	switch t {
	case QueryErrorNone:
		return "none"
	case QueryErrorSyntax:
		return "syntax"
	case QueryErrorNodeType:
		return "node type"
	case QueryErrorField:
		return "field"
	case QueryErrorCapture:
		return "capture"
	case QueryErrorStructure:
		return "structure"
	case QueryErrorLanguage:
		return "language"
	default:
		return "unknown"
	}
}

// PredicateStepType identifies the kind of predicate step.
type PredicateStepType uint8

const (
	PredicateStepDone    PredicateStepType = iota // end of predicate
	PredicateStepCapture                          // capture reference
	PredicateStepString                           // string literal
)

// PredicateStep is one element in a predicate expression.
type PredicateStep struct {
	Type    PredicateStepType
	ValueID uint32 // index into captureNames (for Capture) or stringValues (for String)
}

// QueryCapture represents a captured node in a query match.
type QueryCapture struct {
	Node  Node
	Index uint32 // capture index (into Query.captureNames)
}

// QueryMatch represents a completed pattern match.
type QueryMatch struct {
	ID           uint32
	PatternIndex uint16
	Captures     []QueryCapture
}

// queryStepFlags packs boolean flags into a uint16.
type queryStepFlags uint16

const (
	stepFlagIsNamed                queryStepFlags = 1 << iota
	stepFlagIsImmediate                           // anchor: preceded by '.'
	stepFlagIsLastChild                           // anchor: no subsequent siblings
	stepFlagIsPassThrough                         // split-only step
	stepFlagIsDeadEnd                             // redirect-only step
	stepFlagAlternativeIsImmediate                // alternative step's is_immediate
	stepFlagContainsCaptures                      // step or child has captures
	stepFlagRootPatternGuaranteed                 // whole pattern guaranteed if reached
	stepFlagParentPatternGuaranteed               // sibling steps guaranteed if reached
	stepFlagIsMissing                             // match MISSING nodes
)

// queryStep is a compiled pattern node — one step in the matching process.
type queryStep struct {
	symbol              Symbol
	supertypeSymbol     Symbol
	field               FieldID
	captureIDs          [maxStepCaptureCount]uint16
	depth               uint16
	alternativeIndex    uint16
	negatedFieldListID  uint16
	flags               queryStepFlags
}

func newQueryStep(symbol Symbol, depth uint16, isNamed bool) queryStep {
	s := queryStep{
		symbol:             symbol,
		depth:              depth,
		alternativeIndex:   noneValue,
		negatedFieldListID: noneValue,
	}
	for i := range s.captureIDs {
		s.captureIDs[i] = noneValue
	}
	if isNamed {
		s.flags |= stepFlagIsNamed
	}
	return s
}

func (s *queryStep) isNamed() bool      { return s.flags&stepFlagIsNamed != 0 }
func (s *queryStep) isImmediate() bool   { return s.flags&stepFlagIsImmediate != 0 }
func (s *queryStep) isLastChild() bool   { return s.flags&stepFlagIsLastChild != 0 }
func (s *queryStep) isPassThrough() bool { return s.flags&stepFlagIsPassThrough != 0 }
func (s *queryStep) isDeadEnd() bool     { return s.flags&stepFlagIsDeadEnd != 0 }
func (s *queryStep) containsCaptures() bool {
	return s.flags&stepFlagContainsCaptures != 0
}
func (s *queryStep) rootPatternGuaranteed() bool {
	return s.flags&stepFlagRootPatternGuaranteed != 0
}

// queryPattern holds metadata for one top-level pattern in the query.
type queryPattern struct {
	stepsOffset     uint32 // start index in Query.steps
	stepsLength     uint32
	predicateOffset uint32 // start index in Query.predicateSteps
	predicateLength uint32
	startByte       uint32 // source offset for error reporting
	endByte         uint32
	isNonLocal      bool
}

// patternEntry enables efficient lookup of patterns by their first step's symbol.
type patternEntry struct {
	stepIndex    uint16
	patternIndex uint16
	isRooted     bool
}

// Query is a compiled set of S-expression patterns for structural matching.
type Query struct {
	steps          []queryStep
	patterns       []queryPattern
	predicateSteps []PredicateStep
	captureNames   []string
	stringValues   []string
	negatedFields  [][]FieldID // negatedFields[listID] -> field IDs
	patternMap     []patternEntry
	language       *Language

	// wildcard pattern indices — patterns starting with '_' match any node
	wildcardRootPatternCount uint16
}

// NewQuery compiles a query string for the given language.
// Returns the Query and an error if compilation fails.
func NewQuery(language *Language, source string) (*Query, error) {
	if language == nil {
		return nil, &QueryError{Type: QueryErrorLanguage, Message: "language is nil"}
	}

	q := &Query{
		steps:    make([]queryStep, 0, 32),
		patterns: make([]queryPattern, 0, 4),
		language: language,
	}

	parser := &queryParser{
		query:  q,
		source: source,
		pos:    0,
	}

	if err := parser.parse(); err != nil {
		return nil, err
	}

	q.buildPatternMap()
	q.analyzePatterns()

	return q, nil
}

// PatternCount returns the number of patterns in the query.
func (q *Query) PatternCount() uint32 {
	return uint32(len(q.patterns))
}

// CaptureCount returns the number of capture names in the query.
func (q *Query) CaptureCount() uint32 {
	return uint32(len(q.captureNames))
}

// CaptureNameForID returns the capture name for the given capture index.
func (q *Query) CaptureNameForID(id uint32) string {
	if int(id) < len(q.captureNames) {
		return q.captureNames[id]
	}
	return ""
}

// PredicatesForPattern returns the predicates for the given pattern,
// grouped into individual predicate calls (each ending with PredicateStepDone).
func (q *Query) PredicatesForPattern(patternIndex uint32) [][]PredicateStep {
	if int(patternIndex) >= len(q.patterns) {
		return nil
	}
	pat := &q.patterns[patternIndex]
	if pat.predicateLength == 0 {
		return nil
	}

	steps := q.predicateSteps[pat.predicateOffset : pat.predicateOffset+pat.predicateLength]
	var result [][]PredicateStep
	var current []PredicateStep
	for _, step := range steps {
		if step.Type == PredicateStepDone {
			if len(current) > 0 {
				result = append(result, current)
				current = nil
			}
		} else {
			current = append(current, step)
		}
	}
	if len(current) > 0 {
		result = append(result, current)
	}
	return result
}

// StringValueForID returns the string value at the given index.
func (q *Query) StringValueForID(id uint32) string {
	if int(id) < len(q.stringValues) {
		return q.stringValues[id]
	}
	return ""
}

// buildPatternMap creates a sorted lookup table for pattern starts by symbol.
func (q *Query) buildPatternMap() {
	q.patternMap = q.patternMap[:0]
	q.wildcardRootPatternCount = 0

	for i, pat := range q.patterns {
		if pat.stepsLength == 0 {
			continue
		}
		step := &q.steps[pat.stepsOffset]

		isRooted := pat.stepsLength > 0 && step.depth == 0
		for j := uint32(1); j < pat.stepsLength; j++ {
			if q.steps[pat.stepsOffset+j].depth == 0 {
				isRooted = false
				break
			}
		}

		q.patternMap = append(q.patternMap, patternEntry{
			stepIndex:    uint16(pat.stepsOffset),
			patternIndex: uint16(i),
			isRooted:     isRooted,
		})

		if step.symbol == wildcardSymbol {
			q.wildcardRootPatternCount++
		}
	}

	// Sort by first step symbol for binary search lookup.
	sortPatternEntries(q.patternMap, q.steps)
}

// sortPatternEntries sorts pattern entries by their first step's symbol.
func sortPatternEntries(entries []patternEntry, steps []queryStep) {
	// Simple insertion sort — pattern counts are typically small.
	for i := 1; i < len(entries); i++ {
		key := entries[i]
		keySymbol := steps[key.stepIndex].symbol
		j := i - 1
		for j >= 0 && steps[entries[j].stepIndex].symbol > keySymbol {
			entries[j+1] = entries[j]
			j--
		}
		entries[j+1] = key
	}
}

// analyzePatterns sets derived flags on steps (contains_captures, etc.).
func (q *Query) analyzePatterns() {
	for i := range q.patterns {
		pat := &q.patterns[i]
		end := pat.stepsOffset + pat.stepsLength
		hasCapturesBelow := false
		for j := int(end) - 1; j >= int(pat.stepsOffset); j-- {
			step := &q.steps[j]
			if step.captureIDs[0] != noneValue {
				hasCapturesBelow = true
			}
			if hasCapturesBelow {
				step.flags |= stepFlagContainsCaptures
			}
		}
	}
}

// symbolForName looks up a symbol by its name in the language.
func (q *Query) symbolForName(name string, isNamed bool) (Symbol, bool) {
	for i := uint32(0); i < q.language.SymbolCount; i++ {
		sym := Symbol(i)
		if q.language.SymbolName(sym) == name {
			if isNamed && q.language.SymbolIsNamed(sym) {
				return sym, true
			}
			if !isNamed && !q.language.SymbolIsNamed(sym) {
				return sym, true
			}
		}
	}
	// Also check aliases.
	for i := q.language.SymbolCount; i < q.language.SymbolCount+q.language.AliasCount; i++ {
		sym := Symbol(i)
		if q.language.SymbolName(sym) == name {
			return sym, true
		}
	}
	return 0, false
}

// fieldForName looks up a field ID by its name.
func (q *Query) fieldForName(name string) (FieldID, bool) {
	for i, fn := range q.language.FieldNames {
		if fn == name {
			return FieldID(i), true
		}
	}
	return 0, false
}

// internCaptureName finds or creates a capture name, returning its index.
func (q *Query) internCaptureName(name string) uint32 {
	for i, n := range q.captureNames {
		if n == name {
			return uint32(i)
		}
	}
	id := uint32(len(q.captureNames))
	q.captureNames = append(q.captureNames, name)
	return id
}

// internStringValue finds or creates a string value, returning its index.
func (q *Query) internStringValue(value string) uint32 {
	for i, v := range q.stringValues {
		if v == value {
			return uint32(i)
		}
	}
	id := uint32(len(q.stringValues))
	q.stringValues = append(q.stringValues, value)
	return id
}

// internNegatedFieldList finds or creates a negated field list, returning its ID.
func (q *Query) internNegatedFieldList(fields []FieldID) uint16 {
	for i, existing := range q.negatedFields {
		if fieldListsEqual(existing, fields) {
			return uint16(i)
		}
	}
	id := uint16(len(q.negatedFields))
	cp := make([]FieldID, len(fields))
	copy(cp, fields)
	q.negatedFields = append(q.negatedFields, cp)
	return id
}

func fieldListsEqual(a, b []FieldID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Query Parser ---

// queryParser is the recursive descent parser for S-expression query patterns.
type queryParser struct {
	query  *Query
	source string
	pos    int
}

// parse parses all top-level patterns from the source.
func (p *queryParser) parse() error {
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.source) {
			break
		}

		startByte := uint32(p.pos)
		startStepIndex := uint32(len(p.query.steps))
		startPredicateIndex := uint32(len(p.query.predicateSteps))

		if err := p.parsePattern(0); err != nil {
			return err
		}

		// Handle captures after the top-level pattern (@name).
		p.skipWhitespaceAndComments()
		for p.pos < len(p.source) && p.source[p.pos] == '@' {
			p.pos++ // skip '@'
			name := p.readIdentifier()
			if name == "" {
				return p.syntaxError("expected capture name after '@'")
			}
			captureID := p.query.internCaptureName(name)
			step := &p.query.steps[startStepIndex]
			if err := addCaptureID(step, uint16(captureID)); err != nil {
				return &QueryError{
					Offset:  uint32(p.pos),
					Type:    QueryErrorCapture,
					Message: err.Error(),
				}
			}
			p.skipWhitespaceAndComments()
		}

		// Add DONE marker step.
		doneStep := newQueryStep(0, patternDoneMarker, false)
		doneStep.flags |= stepFlagIsDeadEnd
		p.query.steps = append(p.query.steps, doneStep)

		endByte := uint32(p.pos)
		stepsLen := uint32(len(p.query.steps)) - startStepIndex
		predLen := uint32(len(p.query.predicateSteps)) - startPredicateIndex

		p.query.patterns = append(p.query.patterns, queryPattern{
			stepsOffset:     startStepIndex,
			stepsLength:     stepsLen,
			predicateOffset: startPredicateIndex,
			predicateLength: predLen,
			startByte:       startByte,
			endByte:         endByte,
		})
	}

	if len(p.query.patterns) == 0 {
		return &QueryError{
			Offset:  0,
			Type:    QueryErrorSyntax,
			Message: "empty query",
		}
	}

	return nil
}

// parsePattern parses a single pattern or alternation group.
func (p *queryParser) parsePattern(depth uint16) error {
	p.skipWhitespaceAndComments()
	if p.pos >= len(p.source) {
		return p.syntaxError("unexpected end of query")
	}

	ch := p.source[p.pos]

	switch {
	case ch == '(':
		return p.parseNodePattern(depth)
	case ch == '[':
		return p.parseAlternation(depth)
	case ch == '"':
		return p.parseAnonymousNodePattern(depth)
	case ch == '_':
		return p.parseWildcard(depth)
	default:
		return p.syntaxError(fmt.Sprintf("unexpected character %q", string(ch)))
	}
}

// parseNodePattern parses (node_type field: child @capture ...)
func (p *queryParser) parseNodePattern(depth uint16) error {
	if p.pos >= len(p.source) || p.source[p.pos] != '(' {
		return p.syntaxError("expected '('")
	}
	p.pos++ // consume '('

	p.skipWhitespaceAndComments()

	// Check for anchor before node type.
	isImmediate := false
	if p.pos < len(p.source) && p.source[p.pos] == '.' {
		isImmediate = true
		p.pos++
		p.skipWhitespaceAndComments()
	}

	// Parse node type name.
	if p.pos >= len(p.source) {
		return p.syntaxError("expected node type after '('")
	}

	// Check for wildcard.
	if p.source[p.pos] == '_' {
		return p.parseWildcardInParens(depth, isImmediate)
	}

	name := p.readIdentifier()
	if name == "" {
		return p.syntaxError("expected node type name")
	}

	// Look up symbol.
	sym, found := p.query.symbolForName(name, true)
	if !found {
		return &QueryError{
			Offset:  uint32(p.pos - len(name)),
			Type:    QueryErrorNodeType,
			Message: fmt.Sprintf("unknown node type %q", name),
		}
	}

	stepIndex := len(p.query.steps)
	step := newQueryStep(sym, depth, true)
	if isImmediate {
		step.flags |= stepFlagIsImmediate
	}
	p.query.steps = append(p.query.steps, step)

	// Parse children, fields, captures, predicates, negated fields.
	if err := p.parseNodeBody(depth, stepIndex); err != nil {
		return err
	}

	return nil
}

// parseNodeBody parses the body inside a (...) node pattern.
func (p *queryParser) parseNodeBody(depth uint16, parentStepIndex int) error {
	var negatedFields []FieldID

	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.source) {
			return p.syntaxError("unexpected end inside pattern")
		}

		ch := p.source[p.pos]

		if ch == ')' {
			p.pos++ // consume ')'
			break
		}

		// Check for anchor at end.
		if ch == '.' {
			p.pos++
			p.skipWhitespaceAndComments()
			if p.pos < len(p.source) && p.source[p.pos] == ')' {
				// Trailing anchor — mark as last child.
				p.query.steps[parentStepIndex].flags |= stepFlagIsLastChild
				p.pos++
				break
			}
			// Anchor before next child — mark it as immediate.
			nextStepIndex := len(p.query.steps)
			if err := p.parseNodeChild(depth+1, 0); err != nil {
				return err
			}
			if nextStepIndex < len(p.query.steps) {
				p.query.steps[nextStepIndex].flags |= stepFlagIsImmediate
			}
			continue
		}

		// Check for capture (@name).
		if ch == '@' {
			p.pos++ // skip '@'
			name := p.readIdentifier()
			if name == "" {
				return p.syntaxError("expected capture name after '@'")
			}
			captureID := p.query.internCaptureName(name)
			step := &p.query.steps[parentStepIndex]
			if err := addCaptureID(step, uint16(captureID)); err != nil {
				return &QueryError{
					Offset:  uint32(p.pos),
					Type:    QueryErrorCapture,
					Message: err.Error(),
				}
			}
			continue
		}

		// Check for predicate (#pred).
		if ch == '#' {
			if err := p.parsePredicate(); err != nil {
				return err
			}
			continue
		}

		// Check for negated field (!field).
		if ch == '!' {
			p.pos++ // skip '!'
			fieldName := p.readIdentifier()
			if fieldName == "" {
				return p.syntaxError("expected field name after '!'")
			}
			fieldID, found := p.query.fieldForName(fieldName)
			if !found {
				return &QueryError{
					Offset:  uint32(p.pos - len(fieldName)),
					Type:    QueryErrorField,
					Message: fmt.Sprintf("unknown field %q", fieldName),
				}
			}
			negatedFields = append(negatedFields, fieldID)
			continue
		}

		// Check for field: pattern.
		if isIdentStart(p.peekRune()) {
			savedPos := p.pos
			name := p.readIdentifier()
			p.skipWhitespaceAndComments()
			if p.pos < len(p.source) && p.source[p.pos] == ':' {
				// Field constraint.
				p.pos++ // skip ':'
				p.skipWhitespaceAndComments()

				fieldID, found := p.query.fieldForName(name)
				if !found {
					return &QueryError{
						Offset:  uint32(savedPos),
						Type:    QueryErrorField,
						Message: fmt.Sprintf("unknown field %q", name),
					}
				}

				nextStepIndex := len(p.query.steps)
				if err := p.parseNodeChild(depth+1, fieldID); err != nil {
					return err
				}
				_ = nextStepIndex
				continue
			}
			// Not a field — restore position and fall through to child parsing.
			p.pos = savedPos
		}

		// Check for predicate with parens: (#pred? ...) — tree-sitter syntax.
		if ch == '(' && p.pos+1 < len(p.source) && p.source[p.pos+1] == '#' {
			p.pos++ // consume '('
			if err := p.parsePredicate(); err != nil {
				return err
			}
			// Consume the closing ')' of the predicate.
			p.skipWhitespaceAndComments()
			if p.pos < len(p.source) && p.source[p.pos] == ')' {
				p.pos++
			}
			continue
		}

		// Parse child pattern.
		if err := p.parseNodeChild(depth+1, 0); err != nil {
			return err
		}
	}

	// Store negated fields if any.
	if len(negatedFields) > 0 {
		listID := p.query.internNegatedFieldList(negatedFields)
		p.query.steps[parentStepIndex].negatedFieldListID = listID
	}

	return nil
}

// parseNodeChild parses a child pattern inside a node, with optional field and quantifier.
func (p *queryParser) parseNodeChild(depth uint16, fieldID FieldID) error {
	stepIndex := len(p.query.steps)

	if err := p.parsePattern(depth); err != nil {
		return err
	}

	// Set field on the step we just created.
	if fieldID != 0 && stepIndex < len(p.query.steps) {
		p.query.steps[stepIndex].field = fieldID
	}

	// Check for quantifier.
	p.skipWhitespaceAndComments()
	if p.pos < len(p.source) {
		ch := p.source[p.pos]
		switch ch {
		case '+': // one or more
			p.pos++
			p.addRepeatSteps(stepIndex)
		case '*': // zero or more
			p.pos++
			p.addOptionalAndRepeatSteps(stepIndex)
		case '?': // optional
			p.pos++
			p.addOptionalSteps(stepIndex)
		}
	}

	// Check for capture on this child.
	p.skipWhitespaceAndComments()
	for p.pos < len(p.source) && p.source[p.pos] == '@' {
		p.pos++ // skip '@'
		name := p.readIdentifier()
		if name == "" {
			return p.syntaxError("expected capture name after '@'")
		}
		captureID := p.query.internCaptureName(name)
		step := &p.query.steps[stepIndex]
		if err := addCaptureID(step, uint16(captureID)); err != nil {
			return &QueryError{
				Offset:  uint32(p.pos),
				Type:    QueryErrorCapture,
				Message: err.Error(),
			}
		}
		p.skipWhitespaceAndComments()
	}

	return nil
}

// parseAlternation parses [pattern1 pattern2 ...].
func (p *queryParser) parseAlternation(depth uint16) error {
	if p.pos >= len(p.source) || p.source[p.pos] != '[' {
		return p.syntaxError("expected '['")
	}
	p.pos++ // consume '['

	// We compile alternations as a chain of steps linked by alternativeIndex:
	// step_A (alt->step_B) step_A_body... | step_B (alt->step_C) step_B_body... | ...
	var altStepIndices []int

	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.source) {
			return p.syntaxError("unexpected end inside alternation")
		}
		if p.source[p.pos] == ']' {
			p.pos++ // consume ']'
			break
		}

		// For each alternative, insert a pass-through step that branches.
		branchIndex := len(p.query.steps)
		branchStep := newQueryStep(0, depth, false)
		branchStep.flags |= stepFlagIsPassThrough
		p.query.steps = append(p.query.steps, branchStep)
		altStepIndices = append(altStepIndices, branchIndex)

		// Parse the alternative pattern.
		if err := p.parsePattern(depth); err != nil {
			return err
		}
	}

	// Link alternatives: each pass-through step's alternativeIndex points to the next.
	for i := 0; i < len(altStepIndices)-1; i++ {
		p.query.steps[altStepIndices[i]].alternativeIndex = uint16(altStepIndices[i+1])
	}
	// Last alternative is a dead-end (no further alternatives).
	if len(altStepIndices) > 0 {
		lastIdx := altStepIndices[len(altStepIndices)-1]
		p.query.steps[lastIdx].flags &^= stepFlagIsPassThrough
		p.query.steps[lastIdx].flags |= stepFlagIsDeadEnd
		p.query.steps[lastIdx].alternativeIndex = noneValue
	}

	return nil
}

// parseAnonymousNodePattern parses "literal" anonymous node patterns.
func (p *queryParser) parseAnonymousNodePattern(depth uint16) error {
	if p.pos >= len(p.source) || p.source[p.pos] != '"' {
		return p.syntaxError("expected '\"'")
	}
	p.pos++ // consume opening '"'

	var buf strings.Builder
	for p.pos < len(p.source) {
		ch := p.source[p.pos]
		if ch == '"' {
			p.pos++ // consume closing '"'
			break
		}
		if ch == '\\' && p.pos+1 < len(p.source) {
			p.pos++
			ch = p.source[p.pos]
		}
		buf.WriteByte(ch)
		p.pos++
	}

	name := buf.String()
	sym, found := p.query.symbolForName(name, false)
	if !found {
		return &QueryError{
			Offset:  uint32(p.pos - len(name) - 2),
			Type:    QueryErrorNodeType,
			Message: fmt.Sprintf("unknown anonymous node %q", name),
		}
	}

	step := newQueryStep(sym, depth, false)
	p.query.steps = append(p.query.steps, step)
	return nil
}

// parseWildcard parses a standalone '_' wildcard.
func (p *queryParser) parseWildcard(depth uint16) error {
	if p.pos >= len(p.source) || p.source[p.pos] != '_' {
		return p.syntaxError("expected '_'")
	}
	p.pos++ // consume '_'

	step := newQueryStep(wildcardSymbol, depth, false)
	p.query.steps = append(p.query.steps, step)
	return nil
}

// parseWildcardInParens parses (_ ...) — wildcard with children.
func (p *queryParser) parseWildcardInParens(depth uint16, isImmediate bool) error {
	p.pos++ // consume '_'

	stepIndex := len(p.query.steps)
	step := newQueryStep(wildcardSymbol, depth, false)
	if isImmediate {
		step.flags |= stepFlagIsImmediate
	}
	p.query.steps = append(p.query.steps, step)

	if err := p.parseNodeBody(depth, stepIndex); err != nil {
		return err
	}

	return nil
}

// parsePredicate parses #predicate-name? @capture "string" ...
func (p *queryParser) parsePredicate() error {
	if p.pos >= len(p.source) || p.source[p.pos] != '#' {
		return p.syntaxError("expected '#'")
	}
	p.pos++ // skip '#'

	// Read predicate name (ends at whitespace or '?').
	name := p.readPredicateName()
	if name == "" {
		return p.syntaxError("expected predicate name after '#'")
	}

	// The predicate name is stored as a string value.
	nameID := p.query.internStringValue(name)
	p.query.predicateSteps = append(p.query.predicateSteps, PredicateStep{
		Type:    PredicateStepString,
		ValueID: nameID,
	})

	// Parse arguments until end of containing parens or next predicate.
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.source) {
			break
		}

		ch := p.source[p.pos]
		if ch == ')' || ch == '#' || ch == '!' || ch == '.' {
			break // end of predicate args
		}

		if ch == '@' {
			p.pos++
			capName := p.readIdentifier()
			if capName == "" {
				return p.syntaxError("expected capture name in predicate")
			}
			capID := p.query.internCaptureName(capName)
			p.query.predicateSteps = append(p.query.predicateSteps, PredicateStep{
				Type:    PredicateStepCapture,
				ValueID: capID,
			})
		} else if ch == '"' {
			str, err := p.readQuotedString()
			if err != nil {
				return err
			}
			strID := p.query.internStringValue(str)
			p.query.predicateSteps = append(p.query.predicateSteps, PredicateStep{
				Type:    PredicateStepString,
				ValueID: strID,
			})
		} else {
			break
		}
	}

	// Terminate predicate.
	p.query.predicateSteps = append(p.query.predicateSteps, PredicateStep{
		Type: PredicateStepDone,
	})

	return nil
}

// addRepeatSteps adds repeat logic for '+' quantifier (one or more).
func (p *queryParser) addRepeatSteps(startIndex int) {
	repeatIndex := len(p.query.steps)
	repeatStep := newQueryStep(0, p.query.steps[startIndex].depth, false)
	repeatStep.flags |= stepFlagIsPassThrough
	repeatStep.alternativeIndex = uint16(startIndex)
	p.query.steps = append(p.query.steps, repeatStep)
	_ = repeatIndex
}

// addOptionalSteps adds optional logic for '?' quantifier.
func (p *queryParser) addOptionalSteps(startIndex int) {
	endIndex := len(p.query.steps)
	p.query.steps[startIndex].alternativeIndex = uint16(endIndex)
}

// addOptionalAndRepeatSteps adds logic for '*' quantifier (zero or more).
func (p *queryParser) addOptionalAndRepeatSteps(startIndex int) {
	p.addRepeatSteps(startIndex)
	p.query.steps[startIndex].alternativeIndex = uint16(len(p.query.steps))
}

// --- Lexing helpers ---

func (p *queryParser) skipWhitespaceAndComments() {
	for p.pos < len(p.source) {
		ch := p.source[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			p.pos++
			continue
		}
		if ch == ';' {
			for p.pos < len(p.source) && p.source[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func (p *queryParser) readIdentifier() string {
	start := p.pos
	for p.pos < len(p.source) {
		r, size := utf8.DecodeRuneInString(p.source[p.pos:])
		if r == utf8.RuneError && size <= 1 {
			break
		}
		if p.pos == start {
			if !isIdentStart(r) {
				break
			}
		} else {
			if !isIdentContinue(r) {
				break
			}
		}
		p.pos += size
	}
	return p.source[start:p.pos]
}

func (p *queryParser) readPredicateName() string {
	start := p.pos
	for p.pos < len(p.source) {
		ch := p.source[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' ||
			ch == ')' || ch == '@' || ch == '"' {
			break
		}
		p.pos++
	}
	return p.source[start:p.pos]
}

func (p *queryParser) readQuotedString() (string, error) {
	if p.pos >= len(p.source) || p.source[p.pos] != '"' {
		return "", p.syntaxError("expected '\"'")
	}
	p.pos++ // consume opening '"'

	var buf strings.Builder
	for p.pos < len(p.source) {
		ch := p.source[p.pos]
		if ch == '"' {
			p.pos++
			return buf.String(), nil
		}
		if ch == '\\' && p.pos+1 < len(p.source) {
			p.pos++
			ch = p.source[p.pos]
			switch ch {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case 'r':
				buf.WriteByte('\r')
			case '\\':
				buf.WriteByte('\\')
			case '"':
				buf.WriteByte('"')
			default:
				buf.WriteByte('\\')
				buf.WriteByte(ch)
			}
			p.pos++
			continue
		}
		buf.WriteByte(ch)
		p.pos++
	}
	return "", p.syntaxError("unterminated string")
}

func (p *queryParser) peekRune() rune {
	if p.pos >= len(p.source) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(p.source[p.pos:])
	return r
}

func (p *queryParser) syntaxError(msg string) *QueryError {
	return &QueryError{
		Offset:  uint32(p.pos),
		Type:    QueryErrorSyntax,
		Message: msg,
	}
}

func isIdentStart(r rune) bool {
	return r == '_' || r == '-' || unicode.IsLetter(r)
}

func isIdentContinue(r rune) bool {
	return r == '_' || r == '-' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func addCaptureID(step *queryStep, captureID uint16) error {
	for i := 0; i < maxStepCaptureCount; i++ {
		if step.captureIDs[i] == noneValue {
			step.captureIDs[i] = captureID
			return nil
		}
	}
	return fmt.Errorf("too many captures on single node (max %d)", maxStepCaptureCount)
}
