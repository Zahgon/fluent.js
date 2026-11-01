// Ported from fluent.js (Apache-2.0), fluent-syntax/src/ast.ts.

// Package syntax is the Fluent reference parser, serializer and AST: a port of
// @fluent/syntax. Strings are UTF-8, but every span, position and length counts
// UTF-16 code units, as in the original.
package syntax

import (
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/projectfluent/fluent-go"
	"github.com/projectfluent/fluent-go/internal/jsnum"
)

// Node is every AST node, each a pointer to one of the structs of this package.
// NodeType returns the original's type string ("Message", "Span", ...).
//
// The fields of every node struct are declared in the order the original's
// Object.keys reports them at run time; the reflective walks in walk.go rely on it.
type Node interface{ NodeType() string }

// Entry is a *Message, *Term, *Comment, *GroupComment, *ResourceComment or *Junk.
type Entry interface{ Node }

// PatternElement is a *TextElement or a *Placeable.
type PatternElement interface{ Node }

// InlineExpression is a *StringLiteral, *NumberLiteral, *FunctionReference,
// *MessageReference, *TermReference, *VariableReference or *Placeable.
type InlineExpression interface{ Node }

// Expression is an InlineExpression or a *SelectExpression.
type Expression interface{ Node }

// Literal is a *StringLiteral or a *NumberLiteral.
type Literal interface{ Node }

// VariantKey is an *Identifier or a *NumberLiteral.
type VariantKey interface{ Node }

// Span is a half-open range of UTF-16 code units in the source.
type Span struct{ Start, End int }

// Resource is a whole FTL document.
type Resource struct {
	Body []Entry
	Span *Span
}

// Message is a message entry. Value is nil for a message with attributes only.
type Message struct {
	ID         *Identifier
	Value      *Pattern
	Attributes []*Attribute
	Comment    *Comment
	Span       *Span
}

// Term is a term entry; ID does not include the leading dash.
type Term struct {
	ID         *Identifier
	Value      *Pattern
	Attributes []*Attribute
	Comment    *Comment
	Span       *Span
}

// Pattern is the value of a message, term, attribute or variant.
type Pattern struct {
	Elements []PatternElement
	Span     *Span
}

// TextElement is a run of literal text in a pattern.
type TextElement struct {
	Value string
	Span  *Span
}

// Placeable is an expression in braces.
type Placeable struct {
	Expression Expression
	Span       *Span
}

// StringLiteral holds the exact source text between the quotes, escapes included.
type StringLiteral struct {
	Value string
	Span  *Span
}

// NumberLiteral holds the exact source text of a number.
type NumberLiteral struct {
	Value string
	Span  *Span
}

// MessageReference refers to a message or to one of its attributes.
type MessageReference struct {
	ID        *Identifier
	Attribute *Identifier
	Span      *Span
}

// TermReference refers to a term or to one of its attributes, optionally with arguments.
type TermReference struct {
	ID        *Identifier
	Attribute *Identifier
	Arguments *CallArguments
	Span      *Span
}

// VariableReference refers to a variable supplied at format time.
type VariableReference struct {
	ID   *Identifier
	Span *Span
}

// FunctionReference calls a function.
type FunctionReference struct {
	ID        *Identifier
	Arguments *CallArguments
	Span      *Span
}

// SelectExpression picks one variant by the value of its selector.
type SelectExpression struct {
	Selector InlineExpression
	Variants []*Variant
	Span     *Span
}

// CallArguments holds the arguments of a call.
type CallArguments struct {
	Positional []InlineExpression
	Named      []*NamedArgument
	Span       *Span
}

// Attribute is a named extra pattern of a message or term.
type Attribute struct {
	ID    *Identifier
	Value *Pattern
	Span  *Span
}

// Variant is one branch of a select expression.
type Variant struct {
	Key     VariantKey
	Value   *Pattern
	Default bool
	Span    *Span
}

// NamedArgument is a name and a literal value in a call.
type NamedArgument struct {
	Name  *Identifier
	Value Literal
	Span  *Span
}

// Identifier is a name.
type Identifier struct {
	Name string
	Span *Span
}

// Comment is a standalone or attached comment (#).
type Comment struct {
	Content string
	Span    *Span
}

// GroupComment is a group comment (##).
type GroupComment struct {
	Content string
	Span    *Span
}

// ResourceComment is a resource comment (###).
type ResourceComment struct {
	Content string
	Span    *Span
}

// Junk is source text that failed to parse, with the reason in Annotations.
type Junk struct {
	Annotations []*Annotation
	Content     string
	Span        *Span
}

// Annotation describes a parse error. Arguments holds the original's strings and
// numbers; nil stands for undefined. Span is set even when the parser has spans off.
type Annotation struct {
	Code      string
	Arguments []any
	Message   string
	Span      *Span
}

// NodeType returns "Span".
func (*Span) NodeType() string { return "Span" }

// NodeType returns "Resource".
func (*Resource) NodeType() string { return "Resource" }

// NodeType returns "Message".
func (*Message) NodeType() string { return "Message" }

// NodeType returns "Term".
func (*Term) NodeType() string { return "Term" }

// NodeType returns "Pattern".
func (*Pattern) NodeType() string { return "Pattern" }

// NodeType returns "TextElement".
func (*TextElement) NodeType() string { return "TextElement" }

// NodeType returns "Placeable".
func (*Placeable) NodeType() string { return "Placeable" }

// NodeType returns "StringLiteral".
func (*StringLiteral) NodeType() string { return "StringLiteral" }

// NodeType returns "NumberLiteral".
func (*NumberLiteral) NodeType() string { return "NumberLiteral" }

// NodeType returns "MessageReference".
func (*MessageReference) NodeType() string { return "MessageReference" }

// NodeType returns "TermReference".
func (*TermReference) NodeType() string { return "TermReference" }

// NodeType returns "VariableReference".
func (*VariableReference) NodeType() string { return "VariableReference" }

// NodeType returns "FunctionReference".
func (*FunctionReference) NodeType() string { return "FunctionReference" }

// NodeType returns "SelectExpression".
func (*SelectExpression) NodeType() string { return "SelectExpression" }

// NodeType returns "CallArguments".
func (*CallArguments) NodeType() string { return "CallArguments" }

// NodeType returns "Attribute".
func (*Attribute) NodeType() string { return "Attribute" }

// NodeType returns "Variant".
func (*Variant) NodeType() string { return "Variant" }

// NodeType returns "NamedArgument".
func (*NamedArgument) NodeType() string { return "NamedArgument" }

// NodeType returns "Identifier".
func (*Identifier) NodeType() string { return "Identifier" }

// NodeType returns "Comment".
func (*Comment) NodeType() string { return "Comment" }

// NodeType returns "GroupComment".
func (*GroupComment) NodeType() string { return "GroupComment" }

// NodeType returns "ResourceComment".
func (*ResourceComment) NodeType() string { return "ResourceComment" }

// NodeType returns "Junk".
func (*Junk) NodeType() string { return "Junk" }

// NodeType returns "Annotation".
func (*Annotation) NodeType() string { return "Annotation" }

// Parse returns the literal's value with its escape sequences decoded. An escape
// above U+10FFFF raises RangeError, as String.fromCodePoint does; an escaped
// surrogate decodes to U+FFFD.
func (l *StringLiteral) Parse() string {
	var out strings.Builder
	value := l.Value
	for i := 0; i < len(value); {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			i++
			continue
		}
		decoded, size := decodeEscape(value[i:])
		if size == 0 {
			out.WriteByte('\\')
			i++
			continue
		}
		out.WriteString(decoded)
		i += size
	}
	return out.String()
}

// decodeEscape decodes the escape sequence at the start of s and returns its
// value and its length. The length is 0 when s does not start with a known escape.
func decodeEscape(s string) (string, int) {
	if len(s) < 2 {
		return "", 0
	}
	switch s[1] {
	case '\\':
		return "\\", 2
	case '"':
		return `"`, 2
	case 'u':
		return decodeUnicodeEscape(s, 4)
	case 'U':
		return decodeUnicodeEscape(s, 6)
	}
	return "", 0
}

// decodeUnicodeEscape decodes \uHHHH or \UHHHHHH with the given number of digits.
func decodeUnicodeEscape(s string, digits int) (string, int) {
	if len(s) < 2+digits {
		return "", 0
	}
	codepoint, err := strconv.ParseUint(s[2:2+digits], 16, 32)
	if err != nil {
		return "", 0
	}
	if codepoint > 0x10ffff {
		panic(&fluent.Error{
			Name:    "RangeError",
			Message: "Invalid code point " + strconv.FormatUint(codepoint, 10),
		})
	}
	if 0xd800 <= codepoint && codepoint <= 0xdfff {
		return "�", 2 + digits
	}
	return string(rune(codepoint)), 2 + digits
}

// Parse returns the literal's numeric value and the number of its fractional digits.
func (l *NumberLiteral) Parse() (float64, int) {
	value := jsnum.ParseFloat(l.Value)
	decimalPos := strings.IndexByte(l.Value, '.')
	if decimalPos <= 0 {
		return value, 0
	}
	return value, utf16Len(l.Value[decimalPos+1:])
}

// utf16Len returns the length of s in UTF-16 code units.
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}
