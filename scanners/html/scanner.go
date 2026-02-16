package html

import (
	"encoding/binary"
	"unicode"

	ts "github.com/treesitter-go/treesitter"
)

// External token type indices (must match grammar.js externals array order).
const (
	StartTagName              = iota // 0
	ScriptStartTagName               // 1
	StyleStartTagName                 // 2
	EndTagName                        // 3
	ErroneousEndTagName               // 4
	SelfClosingTagDelimiter           // 5
	ImplicitEndTag                    // 6
	RawText                           // 7
	Comment                           // 8
)

// Scanner implements ts.ExternalScanner for HTML.
// It maintains a stack of open tags to determine implicit end tags.
type Scanner struct {
	tags []Tag
}

// New returns a new HTML external scanner.
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize writes the scanner's tag stack to buf.
// Format: [2 bytes: serialized_count] [2 bytes: total_count] [tag data...]
// Each tag: 1 byte TagType. If Custom: +1 byte name length, +N bytes name.
func (s *Scanner) Serialize(buf []byte) uint32 {
	tagCount := len(s.tags)
	if tagCount > 0xFFFF {
		tagCount = 0xFFFF
	}

	size := uint32(4) // 2 bytes serialized count + 2 bytes tag count
	serializedCount := uint16(0)

	for i := 0; i < tagCount; i++ {
		tag := &s.tags[i]
		if tag.Type == Custom {
			nameLen := len(tag.CustomTagName)
			if nameLen > 255 {
				nameLen = 255
			}
			if size+2+uint32(nameLen) >= 1024 { // TREE_SITTER_SERIALIZATION_BUFFER_SIZE
				break
			}
			buf[size] = byte(tag.Type)
			size++
			buf[size] = byte(nameLen)
			size++
			copy(buf[size:], tag.CustomTagName[:nameLen])
			size += uint32(nameLen)
		} else {
			if size+1 >= 1024 {
				break
			}
			buf[size] = byte(tag.Type)
			size++
		}
		serializedCount++
	}

	// Write the counts at the beginning.
	binary.LittleEndian.PutUint16(buf[0:2], serializedCount)
	binary.LittleEndian.PutUint16(buf[2:4], uint16(tagCount))
	return size
}

// Deserialize restores the scanner's tag stack from data.
func (s *Scanner) Deserialize(data []byte) {
	s.tags = s.tags[:0]

	if len(data) == 0 {
		return
	}

	if len(data) < 4 {
		return
	}

	serializedCount := binary.LittleEndian.Uint16(data[0:2])
	tagCount := binary.LittleEndian.Uint16(data[2:4])
	offset := uint32(4)

	for i := uint16(0); i < serializedCount; i++ {
		if offset >= uint32(len(data)) {
			break
		}
		tagType := TagType(data[offset])
		offset++

		if tagType == Custom {
			if offset >= uint32(len(data)) {
				break
			}
			nameLen := uint32(data[offset])
			offset++
			if offset+nameLen > uint32(len(data)) {
				break
			}
			name := string(data[offset : offset+nameLen])
			offset += nameLen
			s.tags = append(s.tags, Tag{Type: Custom, CustomTagName: name})
		} else {
			s.tags = append(s.tags, Tag{Type: tagType})
		}
	}

	// If we couldn't serialize all tags (buffer full), pad with empty tags.
	for uint16(len(s.tags)) < tagCount {
		s.tags = append(s.tags, Tag{Type: end_})
	}
}

// Scan attempts to recognize one of the 9 HTML external token types.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	if len(validSymbols) <= Comment {
		return false
	}

	// Raw text (script/style content): only scan when explicitly requested
	// and not when start/end tag names are also valid.
	if validSymbols[RawText] && !validSymbols[StartTagName] && !validSymbols[EndTagName] {
		return s.scanRawText(lexer)
	}

	// Skip whitespace.
	for lexer.Lookahead >= 0 && unicode.IsSpace(rune(lexer.Lookahead)) {
		lexer.Advance(true) // skip
	}

	switch {
	case lexer.Lookahead == '<':
		lexer.MarkEnd()
		lexer.Advance(false) // advance past '<'

		if lexer.Lookahead == '!' {
			lexer.Advance(false) // advance past '!'
			return s.scanComment(lexer)
		}

		if validSymbols[ImplicitEndTag] {
			return s.scanImplicitEndTag(lexer)
		}

	case lexer.EOF():
		if validSymbols[ImplicitEndTag] {
			return s.scanImplicitEndTag(lexer)
		}

	case lexer.Lookahead == '/':
		if validSymbols[SelfClosingTagDelimiter] {
			return s.scanSelfClosingTagDelimiter(lexer)
		}

	default:
		if (validSymbols[StartTagName] || validSymbols[EndTagName]) && !validSymbols[RawText] {
			if validSymbols[StartTagName] {
				return s.scanStartTagName(lexer)
			}
			return s.scanEndTagName(lexer)
		}
	}

	return false
}

// scanTagName reads alphanumeric/dash/colon characters and returns an uppercase tag name.
func scanTagName(lexer *ts.Lexer) string {
	var name []byte
	for isAlnumOrDash(lexer.Lookahead) {
		name = append(name, toUpper(lexer.Lookahead))
		lexer.Advance(false)
	}
	return string(name)
}

// scanComment recognizes an HTML comment: <!-- ... -->
// Called after '<!' has been consumed.
func (s *Scanner) scanComment(lexer *ts.Lexer) bool {
	if lexer.Lookahead != '-' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead != '-' {
		return false
	}
	lexer.Advance(false)

	dashes := 0
	for !lexer.EOF() {
		switch lexer.Lookahead {
		case '-':
			dashes++
		case '>':
			if dashes >= 2 {
				lexer.ResultSymbol = ts.Symbol(Comment)
				lexer.Advance(false)
				lexer.MarkEnd()
				return true
			}
			dashes = 0
		default:
			dashes = 0
		}
		lexer.Advance(false)
	}
	return false
}

// scanRawText scans raw text content inside <script> or <style> tags.
// It reads until the matching closing tag delimiter (</SCRIPT or </STYLE).
func (s *Scanner) scanRawText(lexer *ts.Lexer) bool {
	if len(s.tags) == 0 {
		return false
	}

	lexer.MarkEnd()

	endDelimiter := "</SCRIPT"
	if s.tags[len(s.tags)-1].Type == Style {
		endDelimiter = "</STYLE"
	}

	delimiterIndex := 0
	for !lexer.EOF() {
		if toUpper(lexer.Lookahead) == endDelimiter[delimiterIndex] {
			delimiterIndex++
			if delimiterIndex == len(endDelimiter) {
				break
			}
			lexer.Advance(false)
		} else {
			delimiterIndex = 0
			lexer.Advance(false)
			lexer.MarkEnd()
		}
	}

	lexer.ResultSymbol = ts.Symbol(RawText)
	return true
}

// popTag removes the top tag from the stack.
func (s *Scanner) popTag() {
	if len(s.tags) > 0 {
		s.tags = s.tags[:len(s.tags)-1]
	}
}

// parent returns a pointer to the top tag, or nil if the stack is empty.
func (s *Scanner) parent() *Tag {
	if len(s.tags) == 0 {
		return nil
	}
	return &s.tags[len(s.tags)-1]
}

// scanImplicitEndTag handles implicit end tag insertion.
// Called after '<' has been consumed (or at EOF).
func (s *Scanner) scanImplicitEndTag(lexer *ts.Lexer) bool {
	parent := s.parent()

	isClosingTag := false
	if lexer.Lookahead == '/' {
		isClosingTag = true
		lexer.Advance(false)
	} else {
		if parent != nil && parent.isVoid() {
			s.popTag()
			lexer.ResultSymbol = ts.Symbol(ImplicitEndTag)
			return true
		}
	}

	tagName := scanTagName(lexer)
	if len(tagName) == 0 && !lexer.EOF() {
		return false
	}

	nextTag := tagForName(tagName)

	if isClosingTag {
		// The tag correctly closes the topmost element on the stack.
		if len(s.tags) > 0 && s.tags[len(s.tags)-1].eq(&nextTag) {
			return false
		}

		// Otherwise, dig deeper and queue implicit end tags.
		for i := len(s.tags); i > 0; i-- {
			if s.tags[i-1].Type == nextTag.Type {
				s.popTag()
				lexer.ResultSymbol = ts.Symbol(ImplicitEndTag)
				return true
			}
		}
	} else if parent != nil &&
		(!parent.canContain(&nextTag) ||
			((parent.Type == HTML || parent.Type == Head || parent.Type == Body) && lexer.EOF())) {
		s.popTag()
		lexer.ResultSymbol = ts.Symbol(ImplicitEndTag)
		return true
	}

	return false
}

// scanStartTagName scans a start tag name and pushes it onto the tag stack.
func (s *Scanner) scanStartTagName(lexer *ts.Lexer) bool {
	tagName := scanTagName(lexer)
	if len(tagName) == 0 {
		return false
	}

	tag := tagForName(tagName)
	s.tags = append(s.tags, tag)

	switch tag.Type {
	case Script:
		lexer.ResultSymbol = ts.Symbol(ScriptStartTagName)
	case Style:
		lexer.ResultSymbol = ts.Symbol(StyleStartTagName)
	default:
		lexer.ResultSymbol = ts.Symbol(StartTagName)
	}
	return true
}

// scanEndTagName scans an end tag name and pops from the tag stack if it matches.
func (s *Scanner) scanEndTagName(lexer *ts.Lexer) bool {
	tagName := scanTagName(lexer)
	if len(tagName) == 0 {
		return false
	}

	tag := tagForName(tagName)
	if len(s.tags) > 0 && s.tags[len(s.tags)-1].eq(&tag) {
		s.popTag()
		lexer.ResultSymbol = ts.Symbol(EndTagName)
	} else {
		lexer.ResultSymbol = ts.Symbol(ErroneousEndTagName)
	}
	return true
}

// scanSelfClosingTagDelimiter recognizes "/>" and pops the tag stack.
func (s *Scanner) scanSelfClosingTagDelimiter(lexer *ts.Lexer) bool {
	lexer.Advance(false) // consume '/'
	if lexer.Lookahead == '>' {
		lexer.Advance(false) // consume '>'
		if len(s.tags) > 0 {
			s.popTag()
			lexer.ResultSymbol = ts.Symbol(SelfClosingTagDelimiter)
		}
		return true
	}
	return false
}
