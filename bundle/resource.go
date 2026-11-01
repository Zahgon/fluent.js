// Ported from fluent.js (Apache-2.0), fluent-bundle/src/resource.ts.

package bundle

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/projectfluent/fluent-go"
	"github.com/projectfluent/fluent-go/internal/jsnum"
)

// FluentResource is a parsed FTL source. Its parser is not the reference parser of the
// syntax package: it minimizes false negatives at the expense of false positives, does
// not report syntax errors and skips directly to the next message or term instead.
type FluentResource struct {
	body []*Message
}

// NewFluentResource parses source. An escape above U+10FFFF in a string literal raises
// RangeError, which is the one error that is not skipped: it aborts the whole resource.
func NewFluentResource(source string) *FluentResource {
	p := &resourceParser{src: utf16.Encode([]rune(source))}
	res := &FluentResource{}

	// The search for message starts resumes right after the previous start rather than
	// after the parsed message, so a start inside a parsed message counts as well.
	lastIndex := 0
	for {
		id, end, ok := p.nextMessageStart(lastIndex)
		if !ok {
			return res
		}
		lastIndex, p.cursor = end, end
		if message := p.tryParseMessage(id); message != nil {
			res.body = append(res.body, message)
		}
	}
}

// resourceParser holds the source, as UTF-16 code units, and the cursor. The original
// uses sticky regexes; their ECMAScript semantics are written out by hand here.
type resourceParser struct {
	src    []uint16
	cursor int
}

// syntaxError is the panic value of the original's SyntaxError, which is never reported.
type syntaxError struct{}

// endOfSource is the original's undefined for an index past the end of the source.
const endOfSource rune = -1

// at returns the code unit at i, or endOfSource.
func (p *resourceParser) at(i int) rune {
	if i >= len(p.src) {
		return endOfSource
	}
	return rune(p.src[i])
}

// slice returns the source between two offsets as UTF-8.
func (p *resourceParser) slice(start, end int) string {
	return string(utf16.Decode(p.src[start:end]))
}

// nextMessageStart is one exec of /^(?<!\r)(-?[a-zA-Z][\w-]*) *= */gm from lastIndex. It
// returns the identifier and the end of the match. With /m, ^ matches after LF, CR,
// U+2028 and U+2029, and the lookbehind rules out CR.
func (p *resourceParser) nextMessageStart(lastIndex int) (id string, end int, ok bool) {
	for start := lastIndex; start < len(p.src); start++ {
		if start > 0 {
			switch p.src[start-1] {
			case '\n', 0x2028, 0x2029:
			default:
				continue
			}
		}

		idEnd := start
		if p.at(idEnd) == '-' {
			idEnd++
		}
		idEnd = p.identifierEnd(idEnd)
		if idEnd < 0 {
			continue
		}

		equals := p.spacesEnd(idEnd)
		if p.at(equals) == '=' {
			return p.slice(start, idEnd), p.spacesEnd(equals + 1), true
		}
	}
	return "", 0, false
}

func isLetter(ch rune) bool { return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') }

func isDigit(ch rune) bool { return '0' <= ch && ch <= '9' }

// identifierEnd matches [a-zA-Z][\w-]* at i and returns its end, or -1.
func (p *resourceParser) identifierEnd(i int) int {
	if !isLetter(p.at(i)) {
		return -1
	}
	for {
		i++
		if ch := p.at(i); !isLetter(ch) && !isDigit(ch) && ch != '_' && ch != '-' {
			return i
		}
	}
}

// spacesEnd matches " *" at i and returns its end.
func (p *resourceParser) spacesEnd(i int) int {
	for p.at(i) == ' ' {
		i++
	}
	return i
}

// blankEnd matches "(?: |\r?\n)*" at i and returns its end.
func (p *resourceParser) blankEnd(i int) int {
	for {
		switch {
		case p.at(i) == ' ' || p.at(i) == '\n':
			i++
		case p.at(i) == '\r' && p.at(i+1) == '\n':
			i += 2
		default:
			return i
		}
	}
}

// consumeChar advances over ch if it is next.
func (p *resourceParser) consumeChar(ch rune) bool {
	if p.at(p.cursor) == ch {
		p.cursor++
		return true
	}
	return false
}

// consumeBlankToken advances over a token with blanks around it, if blankBefore and
// blankAfter say so.
func (p *resourceParser) consumeBlankToken(blankBefore bool, token string, blankAfter bool) bool {
	i := p.cursor
	if blankBefore {
		i = p.blankEnd(i)
	}
	for _, ch := range token {
		if p.at(i) != ch {
			return false
		}
		i++
	}
	if blankAfter {
		i = p.blankEnd(i)
	}
	p.cursor = i
	return true
}

// tryParseMessage parses a message, returning nil if it is ill-formed.
func (p *resourceParser) tryParseMessage(id string) (message *Message) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(syntaxError); !ok {
				panic(r)
			}
			message = nil
		}
	}()
	return p.parseMessage(id)
}

func (p *resourceParser) parseMessage(id string) *Message {
	value := p.parsePattern()
	attributes := p.parseAttributes()

	if value == nil && len(attributes) == 0 {
		panic(syntaxError{})
	}
	return &Message{ID: id, Value: value, Attributes: attributes}
}

func (p *resourceParser) parseAttributes() map[string]Pattern {
	attrs := map[string]Pattern{}
	for {
		name, ok := p.consumeAttributeStart()
		if !ok {
			return attrs
		}
		value := p.parsePattern()
		if value == nil {
			panic(syntaxError{})
		}
		attrs[name] = value
	}
}

// consumeAttributeStart matches /(?<=\n *)\.([a-zA-Z][\w-]*) *= */y at the cursor.
func (p *resourceParser) consumeAttributeStart() (name string, ok bool) {
	if p.at(p.cursor) != '.' {
		return "", false
	}
	lineStart := p.cursor - 1
	for lineStart >= 0 && p.src[lineStart] == ' ' {
		lineStart--
	}
	if lineStart < 0 || p.src[lineStart] != '\n' {
		return "", false
	}

	idEnd := p.identifierEnd(p.cursor + 1)
	if idEnd < 0 {
		return "", false
	}
	equals := p.spacesEnd(idEnd)
	if p.at(equals) != '=' {
		return "", false
	}
	name = p.slice(p.cursor+1, idEnd)
	p.cursor = p.spacesEnd(equals + 1)
	return name, true
}

// consumeTextRun matches /((?:[^{}\n\r]|\r(?!\n))+)/y at the cursor and returns the text.
func (p *resourceParser) consumeTextRun() string {
	start := p.cursor
	for p.cursor < len(p.src) {
		switch p.src[p.cursor] {
		case '{', '}', '\n':
			return p.slice(start, p.cursor)
		case '\r':
			if p.at(p.cursor+1) == '\n' {
				return p.slice(start, p.cursor)
			}
		}
		p.cursor++
	}
	return p.slice(start, p.cursor)
}

// parsePattern returns a string, a *complexPattern, or nil if there is no pattern here.
func (p *resourceParser) parsePattern() Pattern {
	// First try to parse any simple text on the same line as the id.
	first := p.consumeTextRun()

	// If there's a placeable on the first line, parse a complex pattern.
	if ch := p.at(p.cursor); ch == '{' || ch == '}' {
		elements := []any{}
		if first != "" {
			elements = append(elements, first)
		}
		return p.parsePatternElements(elements, math.MaxInt)
	}

	// The text stops at a newline. Only continue if what comes after it is indented.
	if indent := p.parseIndent(); indent != nil {
		if first != "" {
			// With text on the first line, the blank block is part of the translation.
			return p.parsePatternElements([]any{first, indent}, indent.length)
		}
		// In a block pattern, discard the leading newlines but keep the inline indent.
		indent.value = strings.TrimLeft(indent.value, "\n")
		return p.parsePatternElements([]any{indent}, indent.length)
	}

	if first != "" {
		// It was just a simple inline text after all.
		return strings.TrimRight(first, " ")
	}
	return nil
}

// parsePatternElements parses the rest of a complex pattern and dedents it.
func (p *resourceParser) parsePatternElements(elements []any, commonIndent int) *complexPattern {
	for {
		if text := p.consumeTextRun(); text != "" {
			elements = append(elements, text)
			continue
		}
		switch p.at(p.cursor) {
		case '{':
			elements = append(elements, p.parsePlaceable())
			continue
		case '}':
			panic(syntaxError{})
		}
		indent := p.parseIndent()
		if indent == nil {
			break
		}
		elements = append(elements, indent)
		commonIndent = min(commonIndent, indent.length)
	}

	// Trim the trailing spaces of the last element if it is text.
	last := len(elements) - 1
	if text, ok := elements[last].(string); ok {
		elements[last] = strings.TrimRight(text, " ")
	}

	baked := []any{}
	for _, element := range elements {
		switch e := element.(type) {
		case *indent:
			// Dedent indented lines by the maximum common indent.
			if text := e.value[:len(e.value)-commonIndent]; text != "" {
				baked = append(baked, text)
			}
		case string:
			if e != "" {
				baked = append(baked, e)
			}
		default:
			baked = append(baked, e)
		}
	}
	return &complexPattern{elements: baked}
}

func (p *resourceParser) parsePlaceable() expression {
	p.consumeBlankToken(false, "{", true) // The caller has seen the brace.

	selector := p.parseInlineExpression()
	if p.consumeBlankToken(true, "}", false) {
		return selector
	}

	if p.consumeBlankToken(true, "->", true) {
		variants, star := p.parseVariants()
		if !p.consumeBlankToken(true, "}", false) {
			panic(syntaxError{})
		}
		return &selectExpression{selector: selector, variants: variants, star: star}
	}

	panic(syntaxError{})
}

func (p *resourceParser) parseInlineExpression() expression {
	if p.at(p.cursor) == '{' {
		// It's a nested placeable.
		return p.parsePlaceable()
	}

	sigil, name, attr, ok := p.consumeReference()
	if !ok {
		return p.parseLiteral()
	}

	if sigil == '$' {
		return &variableReference{name: name}
	}

	if p.consumeBlankToken(true, "(", true) {
		args := p.parseArguments()

		if sigil == '-' {
			// A parameterized term: -term(...).
			return &termReference{name: name, attr: attr, args: args}
		}
		if isFunctionName(name) {
			return &functionReference{name: name, args: args}
		}
		panic(syntaxError{})
	}

	if sigil == '-' {
		// A non-parameterized term: -term.
		return &termReference{name: name, attr: attr}
	}

	return &messageReference{name: name, attr: attr}
}

// consumeReference matches /([$-])?([a-zA-Z][\w-]*)(?:\.([a-zA-Z][\w-]*))?/y at the cursor.
func (p *resourceParser) consumeReference() (sigil rune, name, attr string, ok bool) {
	start := p.cursor
	if ch := p.at(start); ch == '$' || ch == '-' {
		sigil = ch
		start++
	}
	end := p.identifierEnd(start)
	if end < 0 {
		return 0, "", "", false
	}
	name = p.slice(start, end)
	if p.at(end) == '.' {
		if attrEnd := p.identifierEnd(end + 1); attrEnd >= 0 {
			attr = p.slice(end+1, attrEnd)
			end = attrEnd
		}
	}
	p.cursor = end
	return sigil, name, attr, true
}

// isFunctionName matches /^[A-Z][A-Z0-9_-]*$/. The name is an identifier, which starts
// with a letter, so it is enough to reject the other characters.
func isFunctionName(name string) bool {
	return !strings.ContainsFunc(name, func(ch rune) bool {
		return !('A' <= ch && ch <= 'Z') && !isDigit(ch) && ch != '_' && ch != '-'
	})
}

func (p *resourceParser) parseArguments() []argument {
	args := []argument{}
	for {
		switch p.at(p.cursor) {
		case ')': // The end of the argument list.
			p.cursor++
			return args
		case endOfSource:
			panic(syntaxError{})
		}

		args = append(args, p.parseArgument())
		// Commas between arguments are treated as whitespace: they are not enforced.
		p.cursor = p.blankEnd(p.cursor)
		p.consumeChar(',')
		p.cursor = p.blankEnd(p.cursor)
	}
}

func (p *resourceParser) parseArgument() argument {
	expr := p.parseInlineExpression()
	reference, ok := expr.(*messageReference)
	if !ok {
		return argument{value: expr}
	}

	if p.consumeBlankToken(true, ":", true) {
		// The reference is the beginning of a named argument; any attribute is dropped.
		return argument{name: reference.name, value: p.parseLiteral()}
	}

	// It's a regular message reference.
	return argument{value: expr}
}

// parseVariants returns nil if there are no variants. Of several default markers,
// the last one wins.
func (p *resourceParser) parseVariants() ([]variant, int) {
	var variants []variant
	star := -1

	for p.at(p.cursor) == '[' || (p.at(p.cursor) == '*' && p.at(p.cursor+1) == '[') {
		if p.consumeChar('*') {
			star = len(variants)
		}

		key := p.parseVariantKey()
		value := p.parsePattern()
		if value == nil {
			panic(syntaxError{})
		}
		variants = append(variants, variant{key: key, value: value})
	}

	if variants != nil && star == -1 {
		panic(syntaxError{})
	}
	return variants, star
}

func (p *resourceParser) parseVariantKey() expression {
	p.consumeBlankToken(false, "[", true) // The caller has seen the bracket.

	key := p.tryParseNumberLiteral()
	if key == nil {
		end := p.identifierEnd(p.cursor)
		if end < 0 {
			panic(syntaxError{})
		}
		key = &stringLiteral{value: p.slice(p.cursor, end)}
		p.cursor = end
	}

	// /(?: |\r?\n)*] */y
	if !p.consumeBlankToken(true, "]", false) {
		panic(syntaxError{})
	}
	p.cursor = p.spacesEnd(p.cursor)
	return key
}

func (p *resourceParser) parseLiteral() expression {
	if number := p.tryParseNumberLiteral(); number != nil {
		return number
	}
	if p.at(p.cursor) == '"' {
		return p.parseStringLiteral()
	}
	panic(syntaxError{})
}

// tryParseNumberLiteral matches /(-?[0-9]+(?:\.([0-9]+))?)/y at the cursor. It returns
// an untyped nil if there is no number.
func (p *resourceParser) tryParseNumberLiteral() expression {
	start := p.cursor
	i := start
	if p.at(i) == '-' {
		i++
	}
	digitsStart := i
	i = p.digitsEnd(i)
	if i == digitsStart {
		return nil
	}

	precision := 0
	if p.at(i) == '.' {
		if fractionEnd := p.digitsEnd(i + 1); fractionEnd > i+1 {
			precision = fractionEnd - i - 1
			i = fractionEnd
		}
	}

	p.cursor = i
	return &numberLiteral{value: jsnum.ParseFloat(p.slice(start, i)), precision: precision}
}

func (p *resourceParser) digitsEnd(i int) int {
	for isDigit(p.at(i)) {
		i++
	}
	return i
}

func (p *resourceParser) parseStringLiteral() expression {
	p.cursor++ // The caller has seen the quote.
	var value strings.Builder
	for {
		value.WriteString(p.consumeStringRun())

		if p.at(p.cursor) == '\\' {
			value.WriteString(p.parseEscapeSequence())
			continue
		}

		if p.consumeChar('"') {
			return &stringLiteral{value: value.String()}
		}

		// We've reached an EOL or EOF.
		panic(syntaxError{})
	}
}

// consumeStringRun matches /((?:[^\\"\n\r]|\r(?!\n))*)/y at the cursor and returns the text.
func (p *resourceParser) consumeStringRun() string {
	start := p.cursor
	for p.cursor < len(p.src) {
		switch p.src[p.cursor] {
		case '\\', '"', '\n':
			return p.slice(start, p.cursor)
		case '\r':
			if p.at(p.cursor+1) == '\n' {
				return p.slice(start, p.cursor)
			}
		}
		p.cursor++
	}
	return p.slice(start, p.cursor)
}

// parseEscapeSequence unescapes the sequence at the cursor.
func (p *resourceParser) parseEscapeSequence() string {
	switch p.at(p.cursor + 1) {
	case '\\':
		p.cursor += 2
		return `\`
	case '"':
		p.cursor += 2
		return `"`
	case 'u':
		return p.parseUnicodeEscape(4)
	case 'U':
		return p.parseUnicodeEscape(6)
	}
	panic(syntaxError{})
}

// parseUnicodeEscape decodes an escape with the given number of hex digits. A lone
// surrogate becomes U+FFFD; a code point above U+10FFFF raises the RangeError of
// String.fromCodePoint.
func (p *resourceParser) parseUnicodeEscape(digits int) string {
	start := p.cursor + 2
	end := start + digits
	if end > len(p.src) {
		panic(syntaxError{})
	}
	codepoint, err := strconv.ParseUint(p.slice(start, end), 16, 32)
	if err != nil {
		panic(syntaxError{})
	}
	p.cursor = end

	if codepoint > 0x10ffff {
		panic(&fluent.Error{
			Name:    "RangeError",
			Message: "Invalid code point " + strconv.FormatUint(codepoint, 10),
		})
	}
	if 0xd800 <= codepoint && codepoint <= 0xdfff {
		return "�"
	}
	return string(rune(codepoint))
}

// indent is the blank text before a pattern line: normalized, and the width of its
// last line.
type indent struct {
	value  string
	length int
}

// parseIndent parses blank space and returns it if it looks like the indent before a
// pattern line. The cursor stays after the blank space even when it returns nil.
func (p *resourceParser) parseIndent() *indent {
	start := p.cursor
	p.cursor = p.blankEnd(start)

	// Check the first non-blank character after the indent.
	switch p.at(p.cursor) {
	case '.', '[', '*', '}', endOfSource:
		// A special character ends the pattern.
		return nil
	case '{':
		// Placeables don't require indentation.
		return makeIndent(p.src[start:p.cursor])
	}

	// A regular text character continues the pattern only if it is indented; otherwise
	// it is likely the identifier of the next message.
	if p.at(p.cursor-1) == ' ' {
		return makeIndent(p.src[start:p.cursor])
	}
	return nil
}

// makeIndent normalizes a blank block: spaces before a line break go and CRLF becomes
// LF, which leaves the spaces of the last line.
func makeIndent(blank []uint16) *indent {
	var value strings.Builder
	spaces := 0
	for _, unit := range blank {
		switch unit {
		case ' ':
			spaces++
		case '\n':
			value.WriteByte('\n')
			spaces = 0
		}
	}
	value.WriteString(strings.Repeat(" ", spaces))
	return &indent{value: value.String(), length: spaces}
}
