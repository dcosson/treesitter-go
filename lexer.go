package treesitter

import (
	"unicode/utf8"
)

// Input provides chunked access to source text. The lexer calls Read()
// when it needs more data. For a contiguous []byte source, use StringInput.
//
// Read returns a slice of source bytes starting at the given byte offset.
// Returning nil or an empty slice signals EOF at that position.
// The returned slice must remain valid until the next Read call.
type Input interface {
	Read(byteOffset uint32, position Point) []byte
}

// StringInput wraps a contiguous []byte as an Input. On first Read it
// returns the full slice (or the tail from byteOffset). On subsequent
// reads at or past the end, it returns nil.
type StringInput struct {
	data []byte
}

// NewStringInput creates a StringInput from a byte slice.
func NewStringInput(data []byte) *StringInput {
	return &StringInput{data: data}
}

// Read returns the source bytes starting at byteOffset.
func (s *StringInput) Read(byteOffset uint32, _ Point) []byte {
	if int(byteOffset) >= len(s.data) {
		return nil
	}
	return s.data[byteOffset:]
}

// Lexer reads input text and produces tokens for the parser. It manages
// chunked input, UTF-8 decoding, position tracking (byte offset + row/column),
// and included range boundaries.
//
// The generated lex function calls Advance/Skip to move through input and
// AcceptToken/MarkEnd to record token boundaries. The parser drives the
// lexer by calling its methods to start/finish lexing at each position.
//
// Lexer is a concrete struct (not an interface) for hot-path performance.
type Lexer struct {
	// Current lookahead character (Unicode code point). -1 means EOF.
	Lookahead int32

	// TokenEndPosition is the byte position where the currently recognized
	// token ends. Set by MarkEnd and used by the parser to determine the
	// token's byte range.
	TokenEndPosition Length

	// ResultSymbol is the symbol of the accepted token.
	ResultSymbol Symbol

	// --- Internal state ---

	input            Input
	currentPosition  Length // current read position (byte offset + row/col)
	tokenStartPosition Length // where the current token started

	// Chunk management: the lexer reads input in chunks.
	chunk      []byte // current chunk of input data
	chunkStart uint32 // byte offset where chunk begins
	chunkSize  uint32 // length of current chunk

	lookaheadSize uint32 // byte length of current lookahead character (1-4 for UTF-8)

	// Included ranges support for language injection.
	includedRanges           []Range
	currentIncludedRangeIndex int
	atIncludedRangeStart     bool

	// markEndCalled tracks whether MarkEnd was called during the current
	// lex invocation. Used by AcceptToken to decide whether to default
	// TokenEndPosition to currentPosition.
	markEndCalled bool

	// logger is reserved for future debug logging support.
	debugEnabled bool
}

// NewLexer creates a new Lexer. If includedRanges is nil or empty,
// a single range covering the entire uint32 space is used.
func NewLexer() *Lexer {
	l := &Lexer{}
	l.SetIncludedRanges(nil)
	return l
}

// SetInput sets the input source for the lexer.
func (l *Lexer) SetInput(input Input) {
	l.input = input
	l.Reset()
}

// Reset clears the lexer's internal state, preparing it for a new parse
// or a reset. The input source is retained.
func (l *Lexer) Reset() {
	l.Lookahead = 0
	l.TokenEndPosition = Length{}
	l.ResultSymbol = 0
	l.currentPosition = Length{}
	l.tokenStartPosition = Length{}
	l.chunk = nil
	l.chunkStart = 0
	l.chunkSize = 0
	l.lookaheadSize = 0
	l.currentIncludedRangeIndex = 0
	l.atIncludedRangeStart = false
	l.markEndCalled = false
}

// SetIncludedRanges sets the byte ranges the lexer should scan.
// If ranges is nil or empty, a single range covering [0, MaxUint32) is used.
func (l *Lexer) SetIncludedRanges(ranges []Range) {
	if len(ranges) == 0 {
		l.includedRanges = []Range{{
			StartPoint: Point{Row: 0, Column: 0},
			EndPoint:   Point{Row: ^uint32(0), Column: ^uint32(0)},
			StartByte:  0,
			EndByte:    ^uint32(0),
		}}
	} else {
		l.includedRanges = make([]Range, len(ranges))
		copy(l.includedRanges, ranges)
	}
}

// EOF returns true if the lexer is at end of input.
func (l *Lexer) EOF() bool {
	return l.Lookahead < 0
}

// Start prepares the lexer to scan a new token starting at the given position.
// This is called by the parser before invoking the lex function.
func (l *Lexer) Start(position Length) {
	l.tokenStartPosition = position
	l.TokenEndPosition = Length{}
	l.ResultSymbol = 0
	l.markEndCalled = false
	l.currentPosition = position

	// Find the included range that contains or follows this position.
	l.currentIncludedRangeIndex = 0
	for i := range l.includedRanges {
		r := &l.includedRanges[i]
		if r.EndByte > position.Bytes {
			l.currentIncludedRangeIndex = i
			// If position is before this range's start, jump to it.
			if position.Bytes < r.StartByte {
				l.currentPosition = Length{
					Bytes: r.StartByte,
					Point: r.StartPoint,
				}
				l.atIncludedRangeStart = true
			} else {
				l.atIncludedRangeStart = (position.Bytes == r.StartByte)
			}
			break
		}
	}

	// Invalidate current chunk to force a fresh read at the new position.
	l.chunk = nil
	l.chunkStart = 0
	l.chunkSize = 0

	// Read the first chunk and decode the first lookahead character.
	l.getChunk()
	l.getLookahead()
}

// Advance moves past the current lookahead character. If skip is true, the
// skipped character is treated as whitespace/ignored content (its size
// contributes to token padding, not token content). If skip is false, the
// character is part of the token being scanned.
func (l *Lexer) Advance(skip bool) {
	if l.lookaheadSize == 0 {
		return
	}

	// Clear the included range start flag — it will be set again if we
	// cross into a new range boundary below.
	l.atIncludedRangeStart = false

	// Update position based on the current character.
	if l.Lookahead == '\n' {
		l.currentPosition.Point.Row++
		l.currentPosition.Point.Column = 0
	} else {
		l.currentPosition.Point.Column += l.lookaheadSize
	}
	l.currentPosition.Bytes += l.lookaheadSize

	// If skipping (whitespace before token), update the token start position.
	if skip {
		l.tokenStartPosition = l.currentPosition
	}

	// Check if we've exited the current included range.
	currentRange := &l.includedRanges[l.currentIncludedRangeIndex]
	if l.currentPosition.Bytes >= currentRange.EndByte {
		// Move to the next included range.
		l.currentIncludedRangeIndex++
		if l.currentIncludedRangeIndex < len(l.includedRanges) {
			nextRange := &l.includedRanges[l.currentIncludedRangeIndex]
			l.currentPosition = Length{
				Bytes: nextRange.StartByte,
				Point: nextRange.StartPoint,
			}
			l.atIncludedRangeStart = true
		} else {
			// Past all included ranges — signal EOF.
			l.Lookahead = -1
			l.lookaheadSize = 0
			return
		}
	}

	l.getLookahead()
}

// Skip is a convenience shorthand for Advance(true).
func (l *Lexer) Skip() {
	l.Advance(true)
}

// MarkEnd records the current position as the end of the recognized token.
// The lex function calls this when it has recognized a valid token prefix
// and wants to remember this position in case longer matches fail.
func (l *Lexer) MarkEnd() {
	l.TokenEndPosition = l.currentPosition
	l.markEndCalled = true
}

// AcceptToken records that a token with the given symbol has been recognized.
// The lex function should call MarkEnd before AcceptToken unless it wants
// the token end to coincide with the current position.
func (l *Lexer) AcceptToken(symbol Symbol) {
	l.ResultSymbol = symbol
	// If MarkEnd wasn't called, default the end position to current.
	if !l.markEndCalled {
		l.TokenEndPosition = l.currentPosition
	}
}

// IsAtIncludedRangeStart returns true if the lexer just jumped to the
// start of a new included range (for language injection boundaries).
func (l *Lexer) IsAtIncludedRangeStart() bool {
	return l.atIncludedRangeStart
}

// CurrentPosition returns the current read position.
func (l *Lexer) CurrentPosition() Length {
	return l.currentPosition
}

// TokenStartPosition returns the position where the current token started.
func (l *Lexer) TokenStartPosition() Length {
	return l.tokenStartPosition
}

// --- Internal methods ---

// getChunk fetches a new chunk of input data at the current position.
func (l *Lexer) getChunk() {
	if l.input == nil {
		l.chunk = nil
		l.chunkStart = 0
		l.chunkSize = 0
		return
	}

	l.chunkStart = l.currentPosition.Bytes
	l.chunk = l.input.Read(l.chunkStart, l.currentPosition.Point)
	if l.chunk == nil {
		l.chunkSize = 0
	} else {
		l.chunkSize = uint32(len(l.chunk))
	}
}

// getLookahead decodes the next UTF-8 character from the current position
// in the current chunk. Handles chunk boundaries and multi-byte characters
// that may span chunk boundaries.
func (l *Lexer) getLookahead() {
	posInChunk := l.currentPosition.Bytes - l.chunkStart

	// If we've consumed the current chunk, fetch a new one.
	if posInChunk >= l.chunkSize {
		l.getChunk()
		posInChunk = l.currentPosition.Bytes - l.chunkStart
		if l.chunkSize == 0 || posInChunk >= l.chunkSize {
			// No more data — EOF.
			l.Lookahead = -1
			l.lookaheadSize = 0
			return
		}
	}

	remaining := l.chunk[posInChunk:]

	// ASCII fast path: byte < 0x80 means single-byte character.
	// This covers >99% of source code characters.
	if remaining[0] < 0x80 {
		l.Lookahead = int32(remaining[0])
		l.lookaheadSize = 1
		return
	}

	// Multi-byte UTF-8 decoding.
	// Check if we have enough bytes in this chunk for a full character.
	needed := utf8ByteLength(remaining[0])
	if uint32(len(remaining)) >= uint32(needed) {
		r, size := utf8.DecodeRune(remaining)
		if r == utf8.RuneError && size <= 1 {
			// Invalid UTF-8 — treat as single byte.
			l.Lookahead = int32(remaining[0])
			l.lookaheadSize = 1
			return
		}
		l.Lookahead = int32(r)
		l.lookaheadSize = uint32(size)
		return
	}

	// The multi-byte character spans a chunk boundary.
	// Buffer the partial bytes and request the next chunk.
	var buf [4]byte
	n := copy(buf[:], remaining)

	// Fetch next chunk for the remaining bytes.
	nextOffset := l.chunkStart + l.chunkSize
	nextChunk := l.input.Read(nextOffset, l.currentPosition.Point)
	if len(nextChunk) > 0 {
		copy(buf[n:], nextChunk[:min(len(nextChunk), 4-n)])
		r, size := utf8.DecodeRune(buf[:min(n+len(nextChunk), 4)])
		if r == utf8.RuneError && size <= 1 {
			l.Lookahead = int32(remaining[0])
			l.lookaheadSize = 1
			return
		}
		l.Lookahead = int32(r)
		l.lookaheadSize = uint32(size)
		// Update chunk to the new one since we'll need it for subsequent reads.
		l.chunk = nextChunk
		l.chunkStart = nextOffset
		l.chunkSize = uint32(len(nextChunk))
	} else {
		// No more data — decode what we have.
		r, size := utf8.DecodeRune(buf[:n])
		if r == utf8.RuneError && size <= 1 {
			l.Lookahead = int32(remaining[0])
			l.lookaheadSize = 1
			return
		}
		l.Lookahead = int32(r)
		l.lookaheadSize = uint32(size)
	}
}

// utf8ByteLength returns the expected byte length of a UTF-8 character
// from its leading byte.
func utf8ByteLength(lead byte) int {
	if lead < 0xC0 {
		return 1 // ASCII or continuation byte (invalid as lead)
	}
	if lead < 0xE0 {
		return 2
	}
	if lead < 0xF0 {
		return 3
	}
	return 4
}

