// Ported from fluent.js (Apache-2.0), fluent-syntax/src/stream.ts.

package syntax

import "unicode/utf16"

const (
	// eol is what charAt reports for both LF and CRLF.
	eol rune = '\n'
	// noChar is the original's undefined at the end of the source, and also what
	// takeChar returns when the predicate rejects the character.
	noChar rune = -1
)

// stream is the original's FluentParserStream: a cursor over UTF-16 code units with
// a separate look-ahead offset. The peek state deliberately survives parse errors,
// as it does in the original.
type stream struct {
	units      []uint16
	index      int
	peekOffset int
}

func newStream(source string) *stream {
	return &stream{units: utf16.Encode([]rune(source))}
}

// unit returns the code unit at offset, or noChar past the end.
func (s *stream) unit(offset int) rune {
	if offset >= len(s.units) {
		return noChar
	}
	return rune(s.units[offset])
}

// slice returns the source between two offsets as UTF-8; the end is clamped.
func (s *stream) slice(start, end int) string {
	end = min(end, len(s.units))
	if start >= end {
		return ""
	}
	return string(utf16.Decode(s.units[start:end]))
}

// charAt returns the code unit at offset, reporting CRLF as a single LF. The cursor
// still points at the CR, so slices of [inclusive, exclusive) keep working.
func (s *stream) charAt(offset int) rune {
	if s.unit(offset) == '\r' && s.unit(offset+1) == '\n' {
		return eol
	}
	return s.unit(offset)
}

func (s *stream) currentChar() rune { return s.charAt(s.index) }

func (s *stream) currentPeek() rune { return s.charAt(s.index + s.peekOffset) }

// next advances the cursor past one character, CRLF being one, and returns the raw
// code unit there.
func (s *stream) next() rune {
	s.peekOffset = 0
	if s.unit(s.index) == '\r' && s.unit(s.index+1) == '\n' {
		s.index++
	}
	s.index++
	return s.unit(s.index)
}

// peek advances the look-ahead past one character, CRLF being one, and returns the
// raw code unit there. The raw CR of a CRLF is why isNextLineComment fails on a
// comment line of "#\r\n".
func (s *stream) peek() rune {
	if s.unit(s.index+s.peekOffset) == '\r' && s.unit(s.index+s.peekOffset+1) == '\n' {
		s.peekOffset++
	}
	s.peekOffset++
	return s.unit(s.index + s.peekOffset)
}

func (s *stream) resetPeek(offset int) { s.peekOffset = offset }

func (s *stream) skipToPeek() {
	s.index += s.peekOffset
	s.peekOffset = 0
}

// peekBlankInline peeks over spaces and returns them.
func (s *stream) peekBlankInline() string {
	start := s.index + s.peekOffset
	for s.currentPeek() == ' ' {
		s.peek()
	}
	return s.slice(start, s.index+s.peekOffset)
}

func (s *stream) skipBlankInline() string {
	blank := s.peekBlankInline()
	s.skipToPeek()
	return blank
}

// peekBlankBlock peeks over blank lines and returns one LF for each of them.
func (s *stream) peekBlankBlock() string {
	blank := ""
	for {
		lineStart := s.peekOffset
		s.peekBlankInline()
		if s.currentPeek() == eol {
			blank += "\n"
			s.peek()
			continue
		}
		if s.currentPeek() == noChar {
			// A blank line at the end of the source is a blank block.
			return blank
		}
		// Any other character: back to column 1 of this line.
		s.resetPeek(lineStart)
		return blank
	}
}

func (s *stream) skipBlankBlock() string {
	blank := s.peekBlankBlock()
	s.skipToPeek()
	return blank
}

func (s *stream) peekBlank() {
	for s.currentPeek() == ' ' || s.currentPeek() == eol {
		s.peek()
	}
}

func (s *stream) skipBlank() {
	s.peekBlank()
	s.skipToPeek()
}

func (s *stream) expectChar(ch rune) {
	if s.currentChar() == ch {
		s.next()
		return
	}
	panic(newParseError("E0003", string(ch)))
}

func (s *stream) expectLineEnd() {
	switch s.currentChar() {
	case noChar:
		// The end of the source is a valid line end.
	case eol:
		s.next()
	default:
		panic(newParseError("E0003", "␤"))
	}
}

// takeChar advances over the current character and returns it if accepted is true
// for it; otherwise it returns noChar.
func (s *stream) takeChar(accepted func(rune) bool) rune {
	ch := s.currentChar()
	if ch == noChar || !accepted(ch) {
		return noChar
	}
	s.next()
	return ch
}

func isCharIDStart(ch rune) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z')
}

func isDigit(ch rune) bool { return '0' <= ch && ch <= '9' }

func isIDChar(ch rune) bool {
	return isCharIDStart(ch) || isDigit(ch) || ch == '_' || ch == '-'
}

func isHexDigit(ch rune) bool {
	return isDigit(ch) || ('A' <= ch && ch <= 'F') || ('a' <= ch && ch <= 'f')
}

func (s *stream) isIdentifierStart() bool { return isCharIDStart(s.currentPeek()) }

func (s *stream) isNumberStart() bool {
	ch := s.currentChar()
	if ch == '-' {
		ch = s.peek()
	}
	s.resetPeek(0)
	return isDigit(ch)
}

func (s *stream) isValueStart() bool {
	ch := s.currentPeek()
	return ch != eol && ch != noChar
}

// isValueContinuation reports whether the peeked line continues a pattern: it starts
// with a placeable, or it is indented and does not start with a special character.
func (s *stream) isValueContinuation() bool {
	column1 := s.peekOffset
	s.peekBlankInline()

	if s.currentPeek() == '{' {
		s.resetPeek(column1)
		return true
	}
	if s.peekOffset-column1 == 0 {
		return false
	}
	if isCharPatternContinuation(s.currentPeek()) {
		s.resetPeek(column1)
		return true
	}
	return false
}

func isCharPatternContinuation(ch rune) bool {
	switch ch {
	case noChar, '}', '.', '[', '*':
		return false
	}
	return true
}

// isNextLineComment reports whether the next line continues a comment of the given
// level: 0 is a comment, 1 a group comment and 2 a resource comment.
func (s *stream) isNextLineComment(level int) bool {
	if s.currentChar() != eol {
		return false
	}
	for i := 0; i <= level; i++ {
		if s.peek() != '#' {
			s.resetPeek(0)
			return false
		}
	}
	ch := s.peek()
	s.resetPeek(0)
	return ch == ' ' || ch == eol
}

func (s *stream) isVariantStart() bool {
	offset := s.peekOffset
	if s.currentPeek() == '*' {
		s.peek()
	}
	isStart := s.currentPeek() == '['
	s.resetPeek(offset)
	return isStart
}

func (s *stream) isAttributeStart() bool { return s.currentPeek() == '.' }

// skipToNextEntryStart moves to the next line that looks like an entry start.
func (s *stream) skipToNextEntryStart(junkStart int) {
	if lastNewline := s.lastNewlineFrom(s.index); junkStart < lastNewline {
		// The last newline is after the junk start, so it is safe to rewind without
		// the risk of resuming at the same broken entry.
		s.index = lastNewline
	}
	for s.currentChar() != noChar {
		if s.currentChar() != eol {
			s.next()
			continue
		}
		first := s.next()
		if isCharIDStart(first) || first == '-' || first == '#' {
			break
		}
	}
}

// lastNewlineFrom returns the offset of the last LF at or before from, or -1.
func (s *stream) lastNewlineFrom(from int) int {
	for i := min(from, len(s.units)-1); i >= 0; i-- {
		if s.units[i] == '\n' {
			return i
		}
	}
	return -1
}

func (s *stream) takeIDStart() rune {
	if isCharIDStart(s.currentChar()) {
		ch := s.currentChar()
		s.next()
		return ch
	}
	panic(newParseError("E0004", "a-zA-Z"))
}
