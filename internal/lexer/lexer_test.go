package lexer

import (
	"testing"
)

// --- StringInput tests ---

func TestStringInputRead(t *testing.T) {
	data := []byte("hello world")
	input := NewStringInput(data)

	// Read from offset 0 returns full data.
	chunk := input.Read(0, Point{})
	if string(chunk) != "hello world" {
		t.Errorf("Read(0) = %q, want %q", chunk, "hello world")
	}

	// Read from offset 6 returns tail.
	chunk = input.Read(6, Point{})
	if string(chunk) != "world" {
		t.Errorf("Read(6) = %q, want %q", chunk, "world")
	}

	// Read past end returns nil.
	chunk = input.Read(11, Point{})
	if chunk != nil {
		t.Errorf("Read(11) = %v, want nil", chunk)
	}

	// Read well past end returns nil.
	chunk = input.Read(100, Point{})
	if chunk != nil {
		t.Errorf("Read(100) = %v, want nil", chunk)
	}
}

// --- Basic Lexer tests ---

func TestLexerNewAndReset(t *testing.T) {
	l := NewLexer()
	if l == nil {
		t.Fatal("NewLexer returned nil")
	}
	// Default included ranges should cover everything.
	if len(l.includedRanges) != 1 {
		t.Errorf("default included ranges count = %d, want 1", len(l.includedRanges))
	}
	if l.includedRanges[0].StartByte != 0 {
		t.Errorf("default range start = %d, want 0", l.includedRanges[0].StartByte)
	}
}

func TestLexerEOFOnEmptyInput(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte{}))
	l.Start(Length{})

	if !l.EOF() {
		t.Error("expected EOF on empty input")
	}
	if l.Lookahead != -1 {
		t.Errorf("Lookahead = %d, want -1 (EOF)", l.Lookahead)
	}
}

func TestLexerEOFOnNilInput(t *testing.T) {
	l := NewLexer()
	// No input set.
	l.Start(Length{})
	if !l.EOF() {
		t.Error("expected EOF with nil input")
	}
}

// --- ASCII character reading ---

func TestLexerASCIICharacters(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("abc")))
	l.Start(Length{})

	// First character.
	if l.Lookahead != 'a' {
		t.Errorf("Lookahead = %c, want 'a'", l.Lookahead)
	}
	if l.EOF() {
		t.Error("unexpected EOF after 'a'")
	}

	// Advance to 'b'.
	l.Advance(false)
	if l.Lookahead != 'b' {
		t.Errorf("Lookahead = %c, want 'b'", l.Lookahead)
	}

	// Advance to 'c'.
	l.Advance(false)
	if l.Lookahead != 'c' {
		t.Errorf("Lookahead = %c, want 'c'", l.Lookahead)
	}

	// Advance past 'c' — EOF.
	l.Advance(false)
	if !l.EOF() {
		t.Errorf("expected EOF, Lookahead = %d", l.Lookahead)
	}
}

// --- Position tracking ---

func TestLexerPositionTracking(t *testing.T) {
	// "ab\ncd" — should track rows and columns.
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("ab\ncd")))
	l.Start(Length{})

	// Position at 'a': byte 0, row 0, col 0.
	pos := l.CurrentPosition()
	if pos.Bytes != 0 || pos.Point.Row != 0 || pos.Point.Column != 0 {
		t.Errorf("pos at 'a' = {%d, %d, %d}, want {0, 0, 0}", pos.Bytes, pos.Point.Row, pos.Point.Column)
	}

	// Advance past 'a': byte 1, row 0, col 1.
	l.Advance(false)
	pos = l.CurrentPosition()
	if pos.Bytes != 1 || pos.Point.Row != 0 || pos.Point.Column != 1 {
		t.Errorf("pos at 'b' = {%d, %d, %d}, want {1, 0, 1}", pos.Bytes, pos.Point.Row, pos.Point.Column)
	}

	// Advance past 'b': byte 2, row 0, col 2.
	l.Advance(false)
	pos = l.CurrentPosition()
	if pos.Bytes != 2 || pos.Point.Row != 0 || pos.Point.Column != 2 {
		t.Errorf("pos at newline = {%d, %d, %d}, want {2, 0, 2}", pos.Bytes, pos.Point.Row, pos.Point.Column)
	}

	// Advance past '\n': byte 3, row 1, col 0.
	l.Advance(false)
	pos = l.CurrentPosition()
	if pos.Bytes != 3 || pos.Point.Row != 1 || pos.Point.Column != 0 {
		t.Errorf("pos at 'c' = {%d, %d, %d}, want {3, 1, 0}", pos.Bytes, pos.Point.Row, pos.Point.Column)
	}

	// Advance past 'c': byte 4, row 1, col 1.
	l.Advance(false)
	pos = l.CurrentPosition()
	if pos.Bytes != 4 || pos.Point.Row != 1 || pos.Point.Column != 1 {
		t.Errorf("pos at 'd' = {%d, %d, %d}, want {4, 1, 1}", pos.Bytes, pos.Point.Row, pos.Point.Column)
	}
}

// --- UTF-8 multi-byte character handling ---

func TestLexerUTF8MultiByte(t *testing.T) {
	// "aé" — 'a' is 1 byte, 'é' (U+00E9) is 2 bytes (0xC3, 0xA9).
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("aé")))
	l.Start(Length{})

	if l.Lookahead != 'a' {
		t.Errorf("Lookahead = %d, want 'a' (97)", l.Lookahead)
	}

	l.Advance(false)
	if l.Lookahead != 'é' {
		t.Errorf("Lookahead = %d (0x%X), want 'é' (0xE9)", l.Lookahead, l.Lookahead)
	}

	// Column should advance by the byte width of the character.
	pos := l.CurrentPosition()
	if pos.Bytes != 1 || pos.Point.Column != 1 {
		t.Errorf("pos at 'é' = {bytes:%d, col:%d}, want {1, 1}", pos.Bytes, pos.Point.Column)
	}

	l.Advance(false)
	pos = l.CurrentPosition()
	// After 'é' (2 bytes): byte offset 3, column 1 + 2 = 3.
	if pos.Bytes != 3 {
		t.Errorf("byte after 'é' = %d, want 3", pos.Bytes)
	}

	if !l.EOF() {
		t.Error("expected EOF")
	}
}

func TestLexerUTF8ThreeBytes(t *testing.T) {
	// "€" is U+20AC, encoded as 0xE2 0x82 0xAC (3 bytes).
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("€")))
	l.Start(Length{})

	if l.Lookahead != '€' {
		t.Errorf("Lookahead = 0x%X, want 0x20AC (€)", l.Lookahead)
	}

	l.Advance(false)
	if !l.EOF() {
		t.Error("expected EOF after single character")
	}

	pos := l.CurrentPosition()
	if pos.Bytes != 3 {
		t.Errorf("byte offset after '€' = %d, want 3", pos.Bytes)
	}
}

func TestLexerUTF8FourBytes(t *testing.T) {
	// "𝄞" is U+1D11E (musical symbol G clef), encoded as 4 bytes.
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("𝄞")))
	l.Start(Length{})

	if l.Lookahead != 0x1D11E {
		t.Errorf("Lookahead = 0x%X, want 0x1D11E (𝄞)", l.Lookahead)
	}

	l.Advance(false)
	if !l.EOF() {
		t.Error("expected EOF")
	}

	pos := l.CurrentPosition()
	if pos.Bytes != 4 {
		t.Errorf("byte offset after 4-byte char = %d, want 4", pos.Bytes)
	}
}

// --- Skip (whitespace handling) ---

func TestLexerSkipUpdatesTokenStart(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("  abc")))
	l.Start(Length{})

	// Skip two spaces.
	l.Skip() // skip ' '
	l.Skip() // skip ' '

	// Token should start at position 2.
	startPos := l.TokenStartPosition()
	if startPos.Bytes != 2 || startPos.Point.Column != 2 {
		t.Errorf("token start after skip = {bytes:%d, col:%d}, want {2, 2}",
			startPos.Bytes, startPos.Point.Column)
	}

	// Current position should also be 2.
	curPos := l.CurrentPosition()
	if curPos.Bytes != 2 || curPos.Point.Column != 2 {
		t.Errorf("current pos after skip = {bytes:%d, col:%d}, want {2, 2}",
			curPos.Bytes, curPos.Point.Column)
	}

	// Lookahead should be 'a'.
	if l.Lookahead != 'a' {
		t.Errorf("Lookahead = %c, want 'a'", l.Lookahead)
	}
}

func TestLexerAdvanceDoesNotUpdateTokenStart(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("abc")))
	l.Start(Length{})

	// Advance past 'a' (not skip — part of token).
	l.Advance(false)

	// Token start should remain at 0.
	startPos := l.TokenStartPosition()
	if startPos.Bytes != 0 {
		t.Errorf("token start after advance = %d, want 0", startPos.Bytes)
	}
}

// --- MarkEnd and AcceptToken ---

func TestLexerMarkEndAndAcceptToken(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("true rest")))
	l.Start(Length{})

	// Advance through "true".
	for i := 0; i < 4; i++ {
		l.Advance(false)
	}

	l.MarkEnd()
	l.AcceptToken(Symbol(11)) // SymTrue

	if l.ResultSymbol != Symbol(11) {
		t.Errorf("ResultSymbol = %d, want 11", l.ResultSymbol)
	}
	if l.TokenEndPosition.Bytes != 4 {
		t.Errorf("TokenEndPosition.Bytes = %d, want 4", l.TokenEndPosition.Bytes)
	}
}

// --- Chunk boundary crossing ---

// chunkedInput splits data into fixed-size chunks to test chunk boundary handling.
type chunkedInput struct {
	data      []byte
	chunkSize int
}

func (c *chunkedInput) Read(byteOffset uint32, _ Point) []byte {
	if int(byteOffset) >= len(c.data) {
		return nil
	}
	end := int(byteOffset) + c.chunkSize
	if end > len(c.data) {
		end = len(c.data)
	}
	return c.data[byteOffset:end]
}

func TestLexerChunkBoundaryCrossing(t *testing.T) {
	data := []byte("abcdefgh")

	// Test with various chunk sizes.
	for chunkSize := 1; chunkSize <= len(data); chunkSize++ {
		t.Run("", func(t *testing.T) {
			l := NewLexer()
			l.SetInput(&chunkedInput{data: data, chunkSize: chunkSize})
			l.Start(Length{})

			var collected []byte
			for !l.EOF() {
				collected = append(collected, byte(l.Lookahead))
				l.Advance(false)
			}

			if string(collected) != string(data) {
				t.Errorf("chunk size %d: collected %q, want %q", chunkSize, collected, data)
			}
		})
	}
}

func TestLexerChunkBoundaryUTF8(t *testing.T) {
	// "aéb" where 'é' spans a chunk boundary.
	data := []byte("aéb") // [0x61, 0xC3, 0xA9, 0x62]

	// Chunk size 2: chunk 1 = [0x61, 0xC3], chunk 2 = [0xA9, 0x62].
	// The 'é' (0xC3, 0xA9) spans chunks.
	l := NewLexer()
	l.SetInput(&chunkedInput{data: data, chunkSize: 2})
	l.Start(Length{})

	if l.Lookahead != 'a' {
		t.Errorf("first char = %c, want 'a'", l.Lookahead)
	}

	l.Advance(false)
	if l.Lookahead != 'é' {
		t.Errorf("second char = 0x%X, want 0xE9 (é)", l.Lookahead)
	}

	l.Advance(false)
	if l.Lookahead != 'b' {
		t.Errorf("third char = %c, want 'b'", l.Lookahead)
	}

	l.Advance(false)
	if !l.EOF() {
		t.Error("expected EOF")
	}
}

func TestLexerChunkBoundaryPositions(t *testing.T) {
	// "ab\ncd" with chunk size 3: chunks are "ab\n" and "cd".
	l := NewLexer()
	l.SetInput(&chunkedInput{data: []byte("ab\ncd"), chunkSize: 3})
	l.Start(Length{})

	// Read 'a', 'b', '\n' (crosses chunk at byte 3), 'c', 'd'.
	expected := []struct {
		ch   int32
		byte uint32
		row  uint32
		col  uint32
	}{
		{'a', 0, 0, 0},
		{'b', 1, 0, 1},
		{'\n', 2, 0, 2},
		{'c', 3, 1, 0},
		{'d', 4, 1, 1},
	}

	for i, exp := range expected {
		if l.Lookahead != exp.ch {
			t.Errorf("step %d: Lookahead = %c, want %c", i, l.Lookahead, exp.ch)
		}
		pos := l.CurrentPosition()
		if pos.Bytes != exp.byte || pos.Point.Row != exp.row || pos.Point.Column != exp.col {
			t.Errorf("step %d: pos = {%d, %d, %d}, want {%d, %d, %d}",
				i, pos.Bytes, pos.Point.Row, pos.Point.Column, exp.byte, exp.row, exp.col)
		}
		l.Advance(false)
	}

	if !l.EOF() {
		t.Error("expected EOF")
	}
}

// --- Included ranges ---

func TestLexerIncludedRangesSingle(t *testing.T) {
	// Parse only the middle portion of the input.
	// Input: "XXHELLOYY" — included range covers bytes 2-7 ("HELLO").
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("XXHELLOYY")))
	l.SetIncludedRanges([]Range{{
		StartByte:  2,
		EndByte:    7,
		StartPoint: Point{Row: 0, Column: 2},
		EndPoint:   Point{Row: 0, Column: 7},
	}})
	l.Start(Length{})

	var collected []byte
	for !l.EOF() {
		collected = append(collected, byte(l.Lookahead))
		l.Advance(false)
	}

	if string(collected) != "HELLO" {
		t.Errorf("collected %q, want %q", collected, "HELLO")
	}
}

func TestLexerIncludedRangesMultiple(t *testing.T) {
	// Input: "aaXXbbXXcc" — two included ranges: [0,2) and [4,6) and [8,10).
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("aaXXbbXXcc")))
	l.SetIncludedRanges([]Range{
		{StartByte: 0, EndByte: 2, StartPoint: Point{0, 0}, EndPoint: Point{0, 2}},
		{StartByte: 4, EndByte: 6, StartPoint: Point{0, 4}, EndPoint: Point{0, 6}},
		{StartByte: 8, EndByte: 10, StartPoint: Point{0, 8}, EndPoint: Point{0, 10}},
	})
	l.Start(Length{})

	var collected []byte
	for !l.EOF() {
		collected = append(collected, byte(l.Lookahead))
		l.Advance(false)
	}

	if string(collected) != "aabbcc" {
		t.Errorf("collected %q, want %q", collected, "aabbcc")
	}
}

func TestLexerIncludedRangeStartFlag(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("aaXXbb")))
	l.SetIncludedRanges([]Range{
		{StartByte: 0, EndByte: 2, StartPoint: Point{0, 0}, EndPoint: Point{0, 2}},
		{StartByte: 4, EndByte: 6, StartPoint: Point{0, 4}, EndPoint: Point{0, 6}},
	})
	l.Start(Length{})

	// At start of first range.
	if !l.IsAtIncludedRangeStart() {
		t.Error("should be at included range start at position 0")
	}

	// Advance through first range.
	l.Advance(false) // past 'a', no longer at range start
	if l.IsAtIncludedRangeStart() {
		t.Error("should not be at range start in middle of range")
	}
	l.Advance(false) // past second 'a', exits first range → jumps to second range

	// Now at start of second range.
	if !l.IsAtIncludedRangeStart() {
		t.Error("should be at included range start after jumping to second range")
	}
}

func TestLexerStartAtOffset(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("abcdef")))
	l.Start(Length{Bytes: 3, Point: Point{Row: 0, Column: 3}})

	if l.Lookahead != 'd' {
		t.Errorf("Lookahead = %c, want 'd'", l.Lookahead)
	}
}

// --- JSON token lexing using hand-compiled lex function ---

// jsonLexMain is a hand-written lex function for the JSON grammar.
// It recognizes: { } , : [ ] " number true false null and whitespace.
func jsonLexMain(lexer *Lexer, state StateID) bool {
	for {
		// Skip whitespace.
		for !lexer.EOF() && (lexer.Lookahead == ' ' || lexer.Lookahead == '\t' ||
			lexer.Lookahead == '\n' || lexer.Lookahead == '\r') {
			lexer.Skip()
		}

		if lexer.EOF() {
			lexer.MarkEnd()
			lexer.AcceptToken(SymbolEnd)
			return true
		}

		switch state {
		case 0: // Main lex state.
			switch lexer.Lookahead {
			case '{':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.AcceptToken(1) // SymLBrace
				return true
			case '}':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.AcceptToken(3) // SymRBrace
				return true
			case ',':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.AcceptToken(2) // SymComma
				return true
			case ':':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.AcceptToken(4) // SymColon
				return true
			case '[':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.AcceptToken(5) // SymLBrack
				return true
			case ']':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.AcceptToken(6) // SymRBrack
				return true
			case '"':
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.AcceptToken(7) // SymDQuote
				return true
			case 't': // "true"
				lexer.Advance(false)
				if !lexer.EOF() && lexer.Lookahead == 'r' {
					lexer.Advance(false)
					if !lexer.EOF() && lexer.Lookahead == 'u' {
						lexer.Advance(false)
						if !lexer.EOF() && lexer.Lookahead == 'e' {
							lexer.Advance(false)
							lexer.MarkEnd()
							lexer.AcceptToken(11) // SymTrue
							return true
						}
					}
				}
				return false
			case 'f': // "false"
				lexer.Advance(false)
				if !lexer.EOF() && lexer.Lookahead == 'a' {
					lexer.Advance(false)
					if !lexer.EOF() && lexer.Lookahead == 'l' {
						lexer.Advance(false)
						if !lexer.EOF() && lexer.Lookahead == 's' {
							lexer.Advance(false)
							if !lexer.EOF() && lexer.Lookahead == 'e' {
								lexer.Advance(false)
								lexer.MarkEnd()
								lexer.AcceptToken(12) // SymFalse
								return true
							}
						}
					}
				}
				return false
			case 'n': // "null"
				lexer.Advance(false)
				if !lexer.EOF() && lexer.Lookahead == 'u' {
					lexer.Advance(false)
					if !lexer.EOF() && lexer.Lookahead == 'l' {
						lexer.Advance(false)
						if !lexer.EOF() && lexer.Lookahead == 'l' {
							lexer.Advance(false)
							lexer.MarkEnd()
							lexer.AcceptToken(13) // SymNull
							return true
						}
					}
				}
				return false
			default:
				// Number: [-]?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]+)?
				if lexer.Lookahead == '-' || (lexer.Lookahead >= '0' && lexer.Lookahead <= '9') {
					if lexer.Lookahead == '-' {
						lexer.Advance(false)
					}
					if lexer.EOF() || lexer.Lookahead < '0' || lexer.Lookahead > '9' {
						return false
					}
					for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
						lexer.Advance(false)
					}
					if !lexer.EOF() && lexer.Lookahead == '.' {
						lexer.Advance(false)
						for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
							lexer.Advance(false)
						}
					}
					if !lexer.EOF() && (lexer.Lookahead == 'e' || lexer.Lookahead == 'E') {
						lexer.Advance(false)
						if !lexer.EOF() && (lexer.Lookahead == '+' || lexer.Lookahead == '-') {
							lexer.Advance(false)
						}
						for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
							lexer.Advance(false)
						}
					}
					lexer.MarkEnd()
					lexer.AcceptToken(10) // SymNumber
					return true
				}
				return false
			}

		case 1: // String content lex state.
			if lexer.Lookahead == '"' {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.AcceptToken(7) // SymDQuote (closing)
				return true
			}
			if lexer.Lookahead == '\\' {
				lexer.Advance(false)
				if !lexer.EOF() {
					lexer.Advance(false) // skip escaped char
				}
				lexer.MarkEnd()
				lexer.AcceptToken(9) // SymEscapeSequence
				return true
			}
			// String content: consume until " or \.
			if !lexer.EOF() && lexer.Lookahead != '"' && lexer.Lookahead != '\\' {
				for !lexer.EOF() && lexer.Lookahead != '"' && lexer.Lookahead != '\\' {
					lexer.Advance(false)
				}
				lexer.MarkEnd()
				lexer.AcceptToken(8) // SymStringContent
				return true
			}
			return false

		default:
			return false
		}
	}
}

// tokenInfo captures a scanned token for test assertions.
type tokenInfo struct {
	symbol    Symbol
	startByte uint32
	endByte   uint32
	startRow  uint32
	startCol  uint32
}

// lexAll runs the lex function repeatedly to tokenize the full input.
func lexAll(input []byte, lexFn func(*Lexer, StateID) bool) []tokenInfo {
	l := NewLexer()
	l.SetInput(NewStringInput(input))

	var tokens []tokenInfo
	position := Length{}

	for {
		l.Start(position)
		// Determine lex state. For our test JSON lex function,
		// state 0 = normal, state 1 = inside string.
		state := StateID(0)
		if len(tokens) > 0 {
			last := tokens[len(tokens)-1]
			// If the last token was an opening quote and wasn't immediately followed
			// by a closing quote, switch to string content mode.
			if last.symbol == 7 { // SymDQuote
				// Check if this is an opening quote (even number of quotes so far).
				quoteCount := 0
				for _, tok := range tokens {
					if tok.symbol == 7 {
						quoteCount++
					}
				}
				if quoteCount%2 == 1 { // odd count means we're inside a string
					state = 1
				}
			}
		}

		found := lexFn(l, state)
		if !found {
			break
		}

		sym := l.ResultSymbol
		if sym == SymbolEnd {
			break
		}

		startPos := l.TokenStartPosition()
		tokens = append(tokens, tokenInfo{
			symbol:    sym,
			startByte: startPos.Bytes,
			endByte:   l.TokenEndPosition.Bytes,
			startRow:  startPos.Point.Row,
			startCol:  startPos.Point.Column,
		})

		// Move position to end of this token.
		position = l.TokenEndPosition
	}

	return tokens
}

func TestLexerJSONPunctuation(t *testing.T) {
	tokens := lexAll([]byte(`{"key": [1, 2]}`), jsonLexMain)

	// Expected tokens: { " key " : [ 1 , 2 ] }
	if len(tokens) < 3 {
		t.Fatalf("got %d tokens, want at least 3", len(tokens))
	}

	// First token should be '{'.
	if tokens[0].symbol != 1 { // SymLBrace
		t.Errorf("token 0 symbol = %d, want 1 ({)", tokens[0].symbol)
	}
	if tokens[0].startByte != 0 || tokens[0].endByte != 1 {
		t.Errorf("token 0 range = [%d, %d), want [0, 1)", tokens[0].startByte, tokens[0].endByte)
	}
}

func TestLexerJSONNumber(t *testing.T) {
	tokens := lexAll([]byte("  42  "), jsonLexMain)

	if len(tokens) != 1 {
		t.Fatalf("got %d tokens, want 1", len(tokens))
	}

	if tokens[0].symbol != 10 { // SymNumber
		t.Errorf("symbol = %d, want 10 (number)", tokens[0].symbol)
	}
	if tokens[0].startByte != 2 || tokens[0].endByte != 4 {
		t.Errorf("range = [%d, %d), want [2, 4)", tokens[0].startByte, tokens[0].endByte)
	}
	if tokens[0].startCol != 2 {
		t.Errorf("start col = %d, want 2", tokens[0].startCol)
	}
}

func TestLexerJSONTrue(t *testing.T) {
	tokens := lexAll([]byte("true"), jsonLexMain)

	if len(tokens) != 1 {
		t.Fatalf("got %d tokens, want 1", len(tokens))
	}
	if tokens[0].symbol != 11 { // SymTrue
		t.Errorf("symbol = %d, want 11 (true)", tokens[0].symbol)
	}
	if tokens[0].startByte != 0 || tokens[0].endByte != 4 {
		t.Errorf("range = [%d, %d), want [0, 4)", tokens[0].startByte, tokens[0].endByte)
	}
}

func TestLexerJSONFalse(t *testing.T) {
	tokens := lexAll([]byte("false"), jsonLexMain)

	if len(tokens) != 1 {
		t.Fatalf("got %d tokens, want 1", len(tokens))
	}
	if tokens[0].symbol != 12 { // SymFalse
		t.Errorf("symbol = %d, want 12 (false)", tokens[0].symbol)
	}
}

func TestLexerJSONNull(t *testing.T) {
	tokens := lexAll([]byte("null"), jsonLexMain)

	if len(tokens) != 1 {
		t.Fatalf("got %d tokens, want 1", len(tokens))
	}
	if tokens[0].symbol != 13 { // SymNull
		t.Errorf("symbol = %d, want 13 (null)", tokens[0].symbol)
	}
}

func TestLexerJSONNegativeNumber(t *testing.T) {
	tokens := lexAll([]byte("-3.14e10"), jsonLexMain)

	if len(tokens) != 1 {
		t.Fatalf("got %d tokens, want 1", len(tokens))
	}
	if tokens[0].symbol != 10 {
		t.Errorf("symbol = %d, want 10 (number)", tokens[0].symbol)
	}
	if tokens[0].endByte != 8 {
		t.Errorf("endByte = %d, want 8", tokens[0].endByte)
	}
}

func TestLexerJSONMultilinePositions(t *testing.T) {
	// Test that positions track correctly across newlines.
	input := []byte("{\n  \"x\": 1\n}")
	tokens := lexAll(input, jsonLexMain)

	// First token: '{' at row 0, col 0.
	if tokens[0].startRow != 0 || tokens[0].startCol != 0 {
		t.Errorf("'{' pos = row %d col %d, want row 0 col 0",
			tokens[0].startRow, tokens[0].startCol)
	}

	// Find the number token (should be on row 1).
	for _, tok := range tokens {
		if tok.symbol == 10 { // SymNumber
			if tok.startRow != 1 {
				t.Errorf("number row = %d, want 1", tok.startRow)
			}
			break
		}
	}
}

// --- Edge cases ---

func TestLexerAdvancePastEOF(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("a")))
	l.Start(Length{})

	l.Advance(false) // past 'a'
	if !l.EOF() {
		t.Fatal("expected EOF")
	}

	// Advancing again past EOF should be a no-op, not crash.
	l.Advance(false)
	if !l.EOF() {
		t.Error("should still be EOF")
	}
}

func TestLexerMultipleStarts(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("abcdef")))

	// First scan: start at 0, read 'a'.
	l.Start(Length{})
	if l.Lookahead != 'a' {
		t.Errorf("first start: Lookahead = %c, want 'a'", l.Lookahead)
	}

	// Second scan: start at byte 3.
	l.Start(Length{Bytes: 3, Point: Point{Row: 0, Column: 3}})
	if l.Lookahead != 'd' {
		t.Errorf("second start: Lookahead = %c, want 'd'", l.Lookahead)
	}
}

func TestLexerSingleCharInput(t *testing.T) {
	l := NewLexer()
	l.SetInput(NewStringInput([]byte("x")))
	l.Start(Length{})

	if l.Lookahead != 'x' {
		t.Errorf("Lookahead = %c, want 'x'", l.Lookahead)
	}

	l.Advance(false)
	if !l.EOF() {
		t.Error("expected EOF after single char")
	}
}
