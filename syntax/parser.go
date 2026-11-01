// Ported from fluent.js (Apache-2.0), fluent-syntax/src/parser.ts.

package syntax

import "strings"

// ParserOptions configures a FluentParser. A nil WithSpans means the original's
// default, true.
type ParserOptions struct{ WithSpans *bool }

// FluentParser is the reference parser of FTL.
type FluentParser struct{ withSpans bool }

// NewFluentParser returns a parser with the given options.
func NewFluentParser(opts ParserOptions) *FluentParser {
	return &FluentParser{withSpans: opts.WithSpans == nil || *opts.WithSpans}
}

// addSpan sets a new node's span from start to the cursor, if spans are on.
func (p *FluentParser) addSpan(span **Span, start int, ps *stream) {
	if p.withSpans {
		*span = &Span{Start: start, End: ps.index}
	}
}

// Parse parses a whole document. Malformed entries become Junk.
func (p *FluentParser) Parse(source string) *Resource {
	ps := newStream(source)
	ps.skipBlankBlock()

	entries := []Entry{}
	var lastComment *Comment

	for ps.currentChar() != noChar {
		entry := p.getEntryOrJunk(ps)
		blankLines := ps.skipBlankBlock()

		// A comment attaches to a following message or term, but it stays standalone
		// when junk follows it, so it can only be attached once the next entry is known.
		if comment, ok := entry.(*Comment); ok && blankLines == "" && ps.currentChar() != noChar {
			lastComment = comment
			continue
		}

		if lastComment != nil {
			switch e := entry.(type) {
			case *Message:
				e.Comment = lastComment
				p.extendSpan(e.Span, lastComment)
			case *Term:
				e.Comment = lastComment
				p.extendSpan(e.Span, lastComment)
			default:
				entries = append(entries, lastComment)
			}
			lastComment = nil
		}

		entries = append(entries, entry)
	}

	res := &Resource{Body: entries}
	p.addSpan(&res.Span, 0, ps)
	return res
}

// extendSpan moves the start of an entry's span to that of its attached comment.
func (p *FluentParser) extendSpan(span *Span, comment *Comment) {
	if p.withSpans {
		span.Start = comment.Span.Start
	}
}

// ParseEntry parses the first message or term of source. It skips comments unless
// they are malformed, in which case it returns their Junk.
func (p *FluentParser) ParseEntry(source string) Entry {
	ps := newStream(source)
	ps.skipBlankBlock()

	for ps.currentChar() == '#' {
		if junk, ok := p.getEntryOrJunk(ps).(*Junk); ok {
			return junk
		}
		ps.skipBlankBlock()
	}

	return p.getEntryOrJunk(ps)
}

// getEntryOrJunk parses one entry and its line end, or turns the source up to the
// next entry start into Junk.
func (p *FluentParser) getEntryOrJunk(ps *stream) Entry {
	entryStart := ps.index

	entry, err := p.tryGetEntry(ps)
	if err == nil {
		return entry
	}

	errorIndex := ps.index
	ps.skipToNextEntryStart(entryStart)
	nextEntryStart := ps.index
	// The position of the error must be inside of the Junk's span.
	errorIndex = min(errorIndex, nextEntryStart)

	junk := &Junk{Content: ps.slice(entryStart, nextEntryStart)}
	p.addSpan(&junk.Span, entryStart, ps)
	junk.Annotations = []*Annotation{{
		Code:      err.code,
		Arguments: err.args,
		Message:   err.message,
		Span:      &Span{Start: errorIndex, End: errorIndex},
	}}
	return junk
}

// tryGetEntry parses one entry and its line end, catching parse errors only.
func (p *FluentParser) tryGetEntry(ps *stream) (entry Entry, err *parseError) {
	defer func() {
		if r := recover(); r != nil {
			parseErr, ok := r.(*parseError)
			if !ok {
				panic(r)
			}
			err = parseErr
		}
	}()
	entry = p.getEntry(ps)
	ps.expectLineEnd()
	return entry, nil
}

func (p *FluentParser) getEntry(ps *stream) Entry {
	if ps.currentChar() == '#' {
		return p.getComment(ps)
	}
	if ps.currentChar() == '-' {
		return p.getTerm(ps)
	}
	if ps.isIdentifierStart() {
		return p.getMessage(ps)
	}
	panic(newParseError("E0002"))
}

func isNotEOL(ch rune) bool { return ch != eol }

// getComment parses a comment of any level, joining its lines with LF.
func (p *FluentParser) getComment(ps *stream) Entry {
	start := ps.index
	level := -1 // 0: comment, 1: group comment, 2: resource comment.
	var content strings.Builder

	for {
		limit := level
		if level == -1 {
			limit = 2
		}
		i := -1
		for ps.currentChar() == '#' && i < limit {
			ps.next()
			i++
		}
		if level == -1 {
			level = i
		}

		if ps.currentChar() != eol {
			ps.expectChar(' ')
			lineStart := ps.index
			for ps.takeChar(isNotEOL) != noChar {
			}
			content.WriteString(ps.slice(lineStart, ps.index))
		}

		if !ps.isNextLineComment(level) {
			break
		}
		content.WriteRune(eol)
		ps.next()
	}

	switch level {
	case 0:
		comment := &Comment{Content: content.String()}
		p.addSpan(&comment.Span, start, ps)
		return comment
	case 1:
		comment := &GroupComment{Content: content.String()}
		p.addSpan(&comment.Span, start, ps)
		return comment
	default:
		comment := &ResourceComment{Content: content.String()}
		p.addSpan(&comment.Span, start, ps)
		return comment
	}
}

func (p *FluentParser) getMessage(ps *stream) *Message {
	start := ps.index
	id := p.getIdentifier(ps)

	ps.skipBlankInline()
	ps.expectChar('=')

	value := p.maybeGetPattern(ps)
	attrs := p.getAttributes(ps)

	if value == nil && len(attrs) == 0 {
		panic(newParseError("E0005", id.Name))
	}

	message := &Message{ID: id, Value: value, Attributes: attrs}
	p.addSpan(&message.Span, start, ps)
	return message
}

func (p *FluentParser) getTerm(ps *stream) *Term {
	start := ps.index
	ps.expectChar('-')
	id := p.getIdentifier(ps)

	ps.skipBlankInline()
	ps.expectChar('=')

	value := p.maybeGetPattern(ps)
	if value == nil {
		panic(newParseError("E0006", id.Name))
	}

	attrs := p.getAttributes(ps)
	term := &Term{ID: id, Value: value, Attributes: attrs}
	p.addSpan(&term.Span, start, ps)
	return term
}

func (p *FluentParser) getAttribute(ps *stream) *Attribute {
	start := ps.index
	ps.expectChar('.')

	key := p.getIdentifier(ps)

	ps.skipBlankInline()
	ps.expectChar('=')

	value := p.maybeGetPattern(ps)
	if value == nil {
		panic(newParseError("E0012"))
	}

	attribute := &Attribute{ID: key, Value: value}
	p.addSpan(&attribute.Span, start, ps)
	return attribute
}

func (p *FluentParser) getAttributes(ps *stream) []*Attribute {
	attrs := []*Attribute{}
	ps.peekBlank()
	for ps.isAttributeStart() {
		ps.skipToPeek()
		attrs = append(attrs, p.getAttribute(ps))
		ps.peekBlank()
	}
	return attrs
}

func (p *FluentParser) getIdentifier(ps *stream) *Identifier {
	start := ps.index
	ps.takeIDStart()
	for ps.takeChar(isIDChar) != noChar {
	}

	id := &Identifier{Name: ps.slice(start, ps.index)}
	p.addSpan(&id.Span, start, ps)
	return id
}

func (p *FluentParser) getVariantKey(ps *stream) VariantKey {
	ch := ps.currentChar()
	if ch == noChar {
		panic(newParseError("E0013"))
	}
	if isDigit(ch) || ch == '-' {
		return p.getNumber(ps)
	}
	return p.getIdentifier(ps)
}

func (p *FluentParser) getVariant(ps *stream, hasDefault bool) *Variant {
	start := ps.index
	isDefault := false

	if ps.currentChar() == '*' {
		if hasDefault {
			panic(newParseError("E0015"))
		}
		ps.next()
		isDefault = true
	}

	ps.expectChar('[')
	ps.skipBlank()

	key := p.getVariantKey(ps)

	ps.skipBlank()
	ps.expectChar(']')

	value := p.maybeGetPattern(ps)
	if value == nil {
		panic(newParseError("E0012"))
	}

	variant := &Variant{Key: key, Value: value, Default: isDefault}
	p.addSpan(&variant.Span, start, ps)
	return variant
}

func (p *FluentParser) getVariants(ps *stream) []*Variant {
	variants := []*Variant{}
	hasDefault := false

	ps.skipBlank()
	for ps.isVariantStart() {
		variant := p.getVariant(ps, hasDefault)
		hasDefault = hasDefault || variant.Default

		variants = append(variants, variant)
		ps.expectLineEnd()
		ps.skipBlank()
	}

	if len(variants) == 0 {
		panic(newParseError("E0011"))
	}
	if !hasDefault {
		panic(newParseError("E0010"))
	}
	return variants
}

func (p *FluentParser) getDigits(ps *stream) string {
	start := ps.index
	for ps.takeChar(isDigit) != noChar {
	}
	if ps.index == start {
		panic(newParseError("E0004", "0-9"))
	}
	return ps.slice(start, ps.index)
}

func (p *FluentParser) getNumber(ps *stream) *NumberLiteral {
	start := ps.index

	value := ""
	if ps.currentChar() == '-' {
		ps.next()
		value = "-"
	}
	value += p.getDigits(ps)

	if ps.currentChar() == '.' {
		ps.next()
		value += "." + p.getDigits(ps)
	}

	number := &NumberLiteral{Value: value}
	p.addSpan(&number.Span, start, ps)
	return number
}

// maybeGetPattern distinguishes patterns that start on the identifier's line from
// block patterns that start on a new line: the indent of the first line of a block
// pattern counts towards the common indent.
func (p *FluentParser) maybeGetPattern(ps *stream) *Pattern {
	ps.peekBlankInline()
	if ps.isValueStart() {
		ps.skipToPeek()
		return p.getPattern(ps, false)
	}

	ps.peekBlankBlock()
	if ps.isValueContinuation() {
		ps.skipToPeek()
		return p.getPattern(ps, true)
	}

	return nil
}

func (p *FluentParser) getPattern(ps *stream, isBlock bool) *Pattern {
	start := ps.index
	elements := []PatternElement{}
	commonIndent := -1 // -1: no indented line yet.
	if isBlock {
		// Store and measure the indent of the first line of a block pattern.
		blankStart := ps.index
		firstIndent := ps.skipBlankInline()
		elements = append(elements, newIndent(firstIndent, blankStart, ps.index))
		commonIndent = len(firstIndent)
	}

loop:
	for {
		switch ps.currentChar() {
		case noChar:
			break loop
		case eol:
			blankStart := ps.index
			blankLines := ps.peekBlankBlock()
			if !ps.isValueContinuation() {
				// A newline not followed by a continuation ends the pattern.
				ps.resetPeek(0)
				break loop
			}
			ps.skipToPeek()
			indent := ps.skipBlankInline()
			if commonIndent == -1 || len(indent) < commonIndent {
				commonIndent = len(indent)
			}
			elements = append(elements, newIndent(blankLines+indent, blankStart, ps.index))
		case '{':
			elements = append(elements, p.getPlaceable(ps))
		case '}':
			panic(newParseError("E0027"))
		default:
			elements = append(elements, p.getTextElement(ps))
		}
	}

	pattern := &Pattern{Elements: p.dedent(elements, commonIndent)}
	p.addSpan(&pattern.Span, start, ps)
	return pattern
}

// indent is a token for the blank text before a pattern line. It is not part of the
// AST: dedent trims it and merges it into a neighbouring TextElement, or turns it
// into a new one.
type indent struct {
	value      string
	start, end int
}

func newIndent(value string, start, end int) *indent {
	return &indent{value: value, start: start, end: end}
}

// NodeType makes the token storable among the pattern's elements until dedent runs.
func (*indent) NodeType() string { return "Indent" }

// dedent removes the common indent from the beginning of the text lines, joins
// adjacent text and trims the trailing whitespace of the pattern.
func (p *FluentParser) dedent(elements []PatternElement, commonIndent int) []PatternElement {
	trimmed := []PatternElement{}

	for _, element := range elements {
		var value string
		var end int
		switch e := element.(type) {
		case *Placeable:
			trimmed = append(trimmed, e)
			continue
		case *indent:
			value = e.value[:len(e.value)-commonIndent]
			if value == "" {
				continue
			}
			end = e.end
		case *TextElement:
			value = e.Value
			if p.withSpans {
				end = e.Span.End
			}
		}

		if len(trimmed) > 0 {
			if prev, ok := trimmed[len(trimmed)-1].(*TextElement); ok {
				sum := &TextElement{Value: prev.Value + value}
				if p.withSpans {
					sum.Span = &Span{Start: prev.Span.Start, End: end}
				}
				trimmed[len(trimmed)-1] = sum
				continue
			}
		}

		if e, ok := element.(*indent); ok {
			text := &TextElement{Value: value}
			if p.withSpans {
				text.Span = &Span{Start: e.start, End: e.end}
			}
			element = text
		}
		trimmed = append(trimmed, element)
	}

	if len(trimmed) > 0 {
		if last, ok := trimmed[len(trimmed)-1].(*TextElement); ok {
			last.Value = strings.TrimRight(last.Value, " \n\r")
			if last.Value == "" {
				trimmed = trimmed[:len(trimmed)-1]
			}
		}
	}
	return trimmed
}

func (p *FluentParser) getTextElement(ps *stream) *TextElement {
	start := ps.index
	for {
		ch := ps.currentChar()
		if ch == noChar || ch == '{' || ch == '}' || ch == eol {
			break
		}
		ps.next()
	}

	text := &TextElement{Value: ps.slice(start, ps.index)}
	p.addSpan(&text.Span, start, ps)
	return text
}

// getEscapeSequence validates the escape after a backslash.
func (p *FluentParser) getEscapeSequence(ps *stream) {
	switch next := ps.currentChar(); next {
	case '\\', '"':
		ps.next()
	case 'u':
		p.getUnicodeEscapeSequence(ps, next, 4)
	case 'U':
		p.getUnicodeEscapeSequence(ps, next, 6)
	case noChar:
		panic(newParseError("E0025", nil))
	default:
		panic(newParseError("E0025", string(next)))
	}
}

func (p *FluentParser) getUnicodeEscapeSequence(ps *stream, u rune, digits int) {
	ps.expectChar(u)

	start := ps.index
	for range digits {
		if ps.takeChar(isHexDigit) == noChar {
			sequence := `\` + string(u) + ps.slice(start, ps.index)
			if ch := ps.currentChar(); ch != noChar {
				panic(newParseError("E0026", sequence+string(ch)))
			}
			panic(newParseError("E0026", sequence+"undefined"))
		}
	}
}

func (p *FluentParser) getPlaceable(ps *stream) *Placeable {
	start := ps.index
	ps.expectChar('{')
	ps.skipBlank()
	expression := p.getExpression(ps)
	ps.expectChar('}')

	placeable := &Placeable{Expression: expression}
	p.addSpan(&placeable.Span, start, ps)
	return placeable
}

func (p *FluentParser) getExpression(ps *stream) Expression {
	start := ps.index
	selector := p.getInlineExpression(ps)
	ps.skipBlank()

	if ps.currentChar() == '-' {
		if ps.peek() != '>' {
			ps.resetPeek(0)
			return selector
		}

		switch s := selector.(type) {
		case *MessageReference:
			if s.Attribute == nil {
				panic(newParseError("E0016"))
			}
			panic(newParseError("E0018"))
		case *TermReference:
			if s.Attribute == nil {
				panic(newParseError("E0017"))
			}
		case *Placeable:
			panic(newParseError("E0029"))
		}

		ps.next()
		ps.next()

		ps.skipBlankInline()
		ps.expectLineEnd()

		variants := p.getVariants(ps)
		expression := &SelectExpression{Selector: selector, Variants: variants}
		p.addSpan(&expression.Span, start, ps)
		return expression
	}

	if term, ok := selector.(*TermReference); ok && term.Attribute != nil {
		panic(newParseError("E0019"))
	}

	return selector
}

func (p *FluentParser) getInlineExpression(ps *stream) InlineExpression {
	start := ps.index

	if ps.currentChar() == '{' {
		return p.getPlaceable(ps)
	}

	if ps.isNumberStart() {
		return p.getNumber(ps)
	}

	if ps.currentChar() == '"' {
		return p.getString(ps)
	}

	if ps.currentChar() == '$' {
		ps.next()
		reference := &VariableReference{ID: p.getIdentifier(ps)}
		p.addSpan(&reference.Span, start, ps)
		return reference
	}

	if ps.currentChar() == '-' {
		ps.next()
		id := p.getIdentifier(ps)

		var attr *Identifier
		if ps.currentChar() == '.' {
			ps.next()
			attr = p.getIdentifier(ps)
		}

		var args *CallArguments
		ps.peekBlank()
		if ps.currentPeek() == '(' {
			ps.skipToPeek()
			args = p.getCallArguments(ps)
		}

		reference := &TermReference{ID: id, Attribute: attr, Arguments: args}
		p.addSpan(&reference.Span, start, ps)
		return reference
	}

	if ps.isIdentifierStart() {
		id := p.getIdentifier(ps)
		ps.peekBlank()

		if ps.currentPeek() == '(' {
			if !isFunctionName(id.Name) {
				panic(newParseError("E0008"))
			}

			ps.skipToPeek()
			reference := &FunctionReference{ID: id, Arguments: p.getCallArguments(ps)}
			p.addSpan(&reference.Span, start, ps)
			return reference
		}

		var attr *Identifier
		if ps.currentChar() == '.' {
			ps.next()
			attr = p.getIdentifier(ps)
		}

		reference := &MessageReference{ID: id, Attribute: attr}
		p.addSpan(&reference.Span, start, ps)
		return reference
	}

	panic(newParseError("E0028"))
}

// isFunctionName matches /^[A-Z][A-Z0-9_-]*$/. The name is an identifier, which
// starts with a letter, so it is enough to reject the other characters.
func isFunctionName(name string) bool {
	return !strings.ContainsFunc(name, func(ch rune) bool {
		return !('A' <= ch && ch <= 'Z') && !isDigit(ch) && ch != '_' && ch != '-'
	})
}

// getCallArgument returns an InlineExpression or a *NamedArgument.
func (p *FluentParser) getCallArgument(ps *stream) Node {
	start := ps.index
	exp := p.getInlineExpression(ps)

	ps.skipBlank()

	if ps.currentChar() != ':' {
		return exp
	}

	if reference, ok := exp.(*MessageReference); ok && reference.Attribute == nil {
		ps.next()
		ps.skipBlank()

		argument := &NamedArgument{Name: reference.ID, Value: p.getLiteral(ps)}
		p.addSpan(&argument.Span, start, ps)
		return argument
	}

	panic(newParseError("E0009"))
}

func (p *FluentParser) getCallArguments(ps *stream) *CallArguments {
	start := ps.index
	positional := []InlineExpression{}
	named := []*NamedArgument{}
	argumentNames := map[string]bool{}

	ps.expectChar('(')
	ps.skipBlank()

	for ps.currentChar() != ')' {
		switch arg := p.getCallArgument(ps).(type) {
		case *NamedArgument:
			if argumentNames[arg.Name.Name] {
				panic(newParseError("E0022"))
			}
			named = append(named, arg)
			argumentNames[arg.Name.Name] = true
		default:
			if len(argumentNames) > 0 {
				panic(newParseError("E0021"))
			}
			positional = append(positional, arg)
		}

		ps.skipBlank()

		if ps.currentChar() != ',' {
			break
		}
		ps.next()
		ps.skipBlank()
	}

	ps.expectChar(')')

	arguments := &CallArguments{Positional: positional, Named: named}
	p.addSpan(&arguments.Span, start, ps)
	return arguments
}

func isStringChar(ch rune) bool { return ch != '"' && ch != eol }

func (p *FluentParser) getString(ps *stream) *StringLiteral {
	start := ps.index
	ps.expectChar('"')

	valueStart := ps.index
	for {
		ch := ps.takeChar(isStringChar)
		if ch == noChar {
			break
		}
		if ch == '\\' {
			p.getEscapeSequence(ps)
		}
	}
	valueEnd := ps.index

	if ps.currentChar() == eol {
		panic(newParseError("E0020"))
	}

	ps.expectChar('"')

	literal := &StringLiteral{Value: ps.slice(valueStart, valueEnd)}
	p.addSpan(&literal.Span, start, ps)
	return literal
}

func (p *FluentParser) getLiteral(ps *stream) Literal {
	if ps.isNumberStart() {
		return p.getNumber(ps)
	}
	if ps.currentChar() == '"' {
		return p.getString(ps)
	}
	panic(newParseError("E0014"))
}
